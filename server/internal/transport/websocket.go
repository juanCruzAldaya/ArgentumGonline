package transport

import (
	"context"
	"errors"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	// sendQueueDepth is how many frames may be in flight to one peer before
	// Send starts reporting backpressure. At 20 Hz this is about three seconds
	// of snapshots — well past the point where the client is unplayable anyway.
	sendQueueDepth = 64

	// readLimitBytes caps a single inbound frame. Client commands are tiny;
	// anything near this is a bug or an attack.
	readLimitBytes = 32 * 1024

	writeTimeout    = 5 * time.Second
	shutdownTimeout = 5 * time.Second
)

// WSServer accepts game clients over WebSocket.
type WSServer struct {
	Addr    string
	Handler Handler
	Logger  *slog.Logger

	// StaticDir, when set, serves the exported Godot web client at /. One
	// process then serves both the page and the protocol, which is why the
	// browser client needs no configuration: its own origin is the server.
	StaticDir string

	mu       sync.Mutex
	listener net.Listener
}

// ListenAndServe serves until ctx is cancelled.
func (s *WSServer) ListenAndServe(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)
	// Plain HTTP health check so a platform like Fly.io can tell whether the
	// machine is up without speaking the game protocol.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	if s.StaticDir != "" {
		// Windows registry entries routinely override the built-in MIME table
		// and hand back the wrong type for .wasm, which browsers refuse to
		// stream-compile. Setting it explicitly costs nothing.
		_ = mime.AddExtensionType(".wasm", "application/wasm")
		mux.Handle("/", crossOriginIsolated(http.FileServer(http.Dir(s.StaticDir))))
		s.Logger.Info("serving web client", "dir", s.StaticDir)
	}

	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	srv := &http.Server{Handler: mux}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	s.Logger.Info("listening", "addr", ln.Addr().String())
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// BoundAddr reports the address actually bound, which is how callers discover
// the port when Addr asked for :0.
func (s *WSServer) BoundAddr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// crossOriginIsolated grants the page SharedArrayBuffer, which Godot's threaded
// web export needs. WebSocket connections are exempt from COEP, so this affects
// the client page only and never the game protocol.
func crossOriginIsolated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		next.ServeHTTP(w, r)
	})
}

func (s *WSServer) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// The Godot client sends no Origin header, and browser-based debug
		// clients may come from anywhere during development.
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		s.Logger.Debug("websocket accept failed", "err", err, "addr", r.RemoteAddr)
		return
	}
	c.SetReadLimit(readLimitBytes)

	ctx, cancel := context.WithCancel(context.Background())
	conn := &wsConn{
		c:      c,
		ctx:    ctx,
		cancel: cancel,
		out:    make(chan []byte, sendQueueDepth),
		addr:   r.RemoteAddr,
	}
	go conn.writePump()

	s.Handler(conn)
	_ = conn.Close()
}

type wsConn struct {
	c      *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc
	out    chan []byte
	addr   string

	closeOnce sync.Once
}

// Send queues frame for delivery and never blocks. If the peer is not draining
// fast enough the frame is dropped and ErrBackpressure is returned, so one slow
// client can never stall the world tick.
func (w *wsConn) Send(frame []byte) error {
	select {
	case <-w.ctx.Done():
		return net.ErrClosed
	default:
	}

	select {
	case w.out <- frame:
		return nil
	default:
		return ErrBackpressure
	}
}

func (w *wsConn) Recv() ([]byte, error) {
	_, data, err := w.c.Read(w.ctx)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (w *wsConn) Close() error {
	w.closeOnce.Do(func() {
		w.cancel()
		_ = w.c.Close(websocket.StatusNormalClosure, "")
	})
	return nil
}

func (w *wsConn) RemoteAddr() string { return w.addr }

// writePump is the only goroutine that writes to the socket, which is what the
// websocket package requires and what lets Send stay non-blocking.
func (w *wsConn) writePump() {
	for {
		select {
		case <-w.ctx.Done():
			return
		case frame := <-w.out:
			ctx, cancel := context.WithTimeout(w.ctx, writeTimeout)
			err := w.c.Write(ctx, websocket.MessageText, frame)
			cancel()
			if err != nil {
				_ = w.Close()
				return
			}
		}
	}
}

// DialWS connects to a server as a client. The bot load generator uses this to
// speak exactly the same protocol a real client does.
func DialWS(ctx context.Context, url string) (Conn, error) {
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	c.SetReadLimit(readLimitBytes)

	connCtx, cancel := context.WithCancel(context.Background())
	conn := &wsConn{
		c:      c,
		ctx:    connCtx,
		cancel: cancel,
		out:    make(chan []byte, sendQueueDepth),
		addr:   url,
	}
	go conn.writePump()
	return conn, nil
}
