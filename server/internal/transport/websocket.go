package transport

import (
	"context"
	"errors"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
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

	// Health, when set, supplies the body of /healthz. A bare "ok" answers
	// "the process is up", which is the one question that never distinguished
	// a real server from one running on an empty generated arena with no items
	// and no spells — the failure that reached production once already. Let
	// the caller say what actually loaded so one curl settles it.
	Health func() string

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
		body := "ok"
		if s.Health != nil {
			body = s.Health()
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})

	if s.StaticDir != "" {
		// Windows registry entries routinely override the built-in MIME table
		// and hand back the wrong type for .wasm, which browsers refuse to
		// stream-compile. Setting it explicitly costs nothing.
		_ = mime.AddExtensionType(".wasm", "application/wasm")
		mux.Handle("/", crossOriginIsolated(precompressed(s.StaticDir, http.FileServer(http.Dir(s.StaticDir)))))
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

// precompressed serves "<file>.gz" in place of "<file>" when one exists and
// the client accepts gzip, falling through to next otherwise.
//
// The web export is 46MB on disk and 38MB of that is one wasm. http.FileServer
// does not compress, so without this every single first load pulls the whole
// 46MB even though the browser asked for gzip — against ~17MB compressed. The
// files are compressed once at build time rather than per request, because the
// machine serving them is a 256MB shared CPU and gzipping 38MB per visitor
// would cost far more than the bandwidth saved.
//
// Content-Type has to be set from the *uncompressed* name: Go would otherwise
// infer "application/gzip" from the .gz suffix and the browser would refuse to
// stream-compile the wasm.
func precompressed(dir string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		// path.Clean via http.ServeFile's own rules is not enough here because
		// we build a filesystem path by hand; reject anything with a traversal
		// rather than trying to normalise it.
		name := path.Clean("/" + r.URL.Path)
		if strings.Contains(name, "..") {
			next.ServeHTTP(w, r)
			return
		}
		gz := filepath.Join(dir, filepath.FromSlash(name)+".gz")
		info, err := os.Stat(gz)
		if err != nil || info.IsDir() {
			next.ServeHTTP(w, r)
			return
		}

		// Always set it, never leave it to ServeFile. Given a .gz path Go
		// infers "application/x-gzip", which is both wrong and self-
		// contradictory next to Content-Encoding: gzip — the body is a wasm
		// or a pack that happens to be transfer-compressed, not a gzip file.
		// Godot's own .pck has no registered MIME type, hence the fallback.
		ctype := mime.TypeByExtension(filepath.Ext(name))
		if ctype == "" {
			ctype = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		http.ServeFile(w, r, gz)
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
