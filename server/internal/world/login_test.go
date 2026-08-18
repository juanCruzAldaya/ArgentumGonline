package world

import (
	"errors"
	"io"
	"sync"
	"testing"

	"juegito/server/internal/protocol"
)

// scriptedConn plays a fixed list of client frames and records what came back,
// so the handshake can be driven the way a real client drives it: one message
// at a time, with the server's answer deciding what happens next.
type scriptedConn struct {
	mu     sync.Mutex
	inbox  [][]byte
	sent   [][]byte
	closed bool
}

func (c *scriptedConn) push(t *testing.T, typ protocol.MsgType, payload any) {
	t.Helper()
	codec := protocol.JSONCodec{}
	frame, err := codec.Encode(typ, payload)
	if err != nil {
		t.Fatalf("encode %s: %v", typ, err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inbox = append(c.inbox, frame)
}

func (c *scriptedConn) Recv() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.inbox) == 0 {
		return nil, io.EOF
	}
	frame := c.inbox[0]
	c.inbox = c.inbox[1:]
	return frame, nil
}

func (c *scriptedConn) Send(frame []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, append([]byte(nil), frame...))
	return nil
}

func (c *scriptedConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *scriptedConn) RemoteAddr() string { return "scripted" }

func (c *scriptedConn) types() []protocol.MsgType {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []protocol.MsgType
	codec := protocol.JSONCodec{}
	for _, frame := range c.sent {
		typ, _, err := codec.DecodeEnvelope(frame)
		if err == nil {
			out = append(out, typ)
		}
	}
	return out
}

func (c *scriptedConn) payloadOf(t *testing.T, typ protocol.MsgType, into any) bool {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	codec := protocol.JSONCodec{}
	for _, frame := range c.sent {
		got, payload, err := codec.DecodeEnvelope(frame)
		if err != nil || got != typ {
			continue
		}
		if err := codec.DecodePayload(payload, into); err != nil {
			t.Fatalf("decode %s: %v", typ, err)
		}
		return true
	}
	return false
}

// fakeAccounts is an in-memory stand-in: the handshake is what is under test,
// not the store, which has its own suite.
type fakeAccounts struct {
	mu       sync.Mutex
	byName   map[string]string // name -> password
	byEmail  map[string]string // name -> email, so a test can check it travelled
	profiles map[string]protocol.Account
	recorded []struct {
		name string
		out  protocol.Outcome
		mapa string
	}
}

func newFakeAccounts() *fakeAccounts {
	return &fakeAccounts{
		byName:   map[string]string{},
		byEmail:  map[string]string{},
		profiles: map[string]protocol.Account{},
	}
}

func (f *fakeAccounts) Register(name, email, password string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, taken := f.byName[name]; taken {
		return errors.New("ese nombre ya está tomado")
	}
	if email == "" {
		return errors.New("falta el correo")
	}
	f.byName[name] = password
	f.byEmail[name] = email
	f.profiles[name] = protocol.Account{Name: name}
	return nil
}

func (f *fakeAccounts) Authenticate(name, password string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	want, ok := f.byName[name]
	if !ok {
		return "", errors.New("no existe esa cuenta")
	}
	if want != password {
		return "", errors.New("contraseña incorrecta")
	}
	return name, nil
}

func (f *fakeAccounts) Profile(name string) (protocol.Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.profiles[name]
	if !ok {
		return protocol.Account{}, errors.New("no existe esa cuenta")
	}
	return p, nil
}

func (f *fakeAccounts) Record(name string, out protocol.Outcome, mapa string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recorded = append(f.recorded, struct {
		name string
		out  protocol.Outcome
		mapa string
	}{name, out, mapa})
}

// The happy path: register, get the career back, then pick a character.
func TestLoginThenJoin(t *testing.T) {
	w := newTestWorld(t, 40, 40)
	accounts := newFakeAccounts()
	w.SetAccounts(accounts)
	go w.Run(t.Context())

	conn := &scriptedConn{}
	conn.push(t, protocol.TypeLogin, protocol.Login{Name: "wachin", Email: "wachin@ejemplo.com", Password: "seiscaracteres", Register: true})
	conn.push(t, protocol.TypeJoin, protocol.Join{Name: "otro", Class: 0, Race: 0})
	w.HandleConn(conn)

	var got protocol.Account
	if !conn.payloadOf(t, protocol.TypeAccount, &got) {
		t.Fatalf("no llegó la ficha de la cuenta; se mandó %v", conn.types())
	}
	if got.Name != "wachin" {
		t.Errorf("ficha de %q", got.Name)
	}
	var welcome protocol.Welcome
	if !conn.payloadOf(t, protocol.TypeWelcome, &welcome) {
		t.Fatalf("no se entró al mundo; se mandó %v", conn.types())
	}

	// El correo del registro tiene que haber llegado hasta el store. El mundo
	// no lo valida ni lo guarda — solo lo acarrea — así que si se pierde, se
	// pierde en silencio y la cuenta queda creada igual.
	if got := accounts.byEmail["wachin"]; got != "wachin@ejemplo.com" {
		t.Errorf("el correo que llegó al store es %q", got)
	}
}

// Signing in does not carry an address, and must not start asking for one: the
// email is what registration collects, not a second credential.
func TestSignInNeedsNoEmail(t *testing.T) {
	w := newTestWorld(t, 40, 40)
	accounts := newFakeAccounts()
	if err := accounts.Register("wachin", "wachin@ejemplo.com", "seiscaracteres"); err != nil {
		t.Fatal(err)
	}
	w.SetAccounts(accounts)
	go w.Run(t.Context())

	conn := &scriptedConn{}
	conn.push(t, protocol.TypeLogin, protocol.Login{Name: "wachin", Password: "seiscaracteres"})
	conn.push(t, protocol.TypeJoin, protocol.Join{Class: 0, Race: 0})
	w.HandleConn(conn)

	var welcome protocol.Welcome
	if !conn.payloadOf(t, protocol.TypeWelcome, &welcome) {
		t.Fatalf("entrar sin correo no dejó entrar; se mandó %v", conn.types())
	}
}

// The name in the join is ignored once there is an account behind the
// connection. Without this the whole record is of whoever the client felt like
// claiming to be that match.
func TestTheJoinCannotRenameAnAccount(t *testing.T) {
	w := newTestWorld(t, 40, 40)
	accounts := newFakeAccounts()
	if err := accounts.Register("wachin", "wachin@ejemplo.com", "seiscaracteres"); err != nil {
		t.Fatal(err)
	}
	w.SetAccounts(accounts)
	go w.Run(t.Context())

	conn := &scriptedConn{}
	conn.push(t, protocol.TypeLogin, protocol.Login{Name: "wachin", Password: "seiscaracteres"})
	conn.push(t, protocol.TypeJoin, protocol.Join{Name: "ElVerdaderoCampeon"})
	w.HandleConn(conn)

	var welcome protocol.Welcome
	if !conn.payloadOf(t, protocol.TypeWelcome, &welcome) {
		t.Fatal("no se entró al mundo")
	}
	// The snapshot names entities, so the claimed name would show up there if
	// the join had been believed.
	var snap protocol.Snapshot
	if conn.payloadOf(t, protocol.TypeSnapshot, &snap) {
		for _, e := range snap.Entities {
			if e.ID == welcome.EntityID && e.Name != "wachin" {
				t.Errorf("el jugador entró como %q: el join le cambió el nombre a la cuenta", e.Name)
			}
		}
	}
}

// A wrong password answers and waits for another attempt rather than dropping
// the connection: the map and the collision bitset are already downloaded, and
// a typo should not cost them.
func TestAWrongPasswordCanBeRetried(t *testing.T) {
	w := newTestWorld(t, 40, 40)
	accounts := newFakeAccounts()
	if err := accounts.Register("wachin", "wachin@ejemplo.com", "seiscaracteres"); err != nil {
		t.Fatal(err)
	}
	w.SetAccounts(accounts)
	go w.Run(t.Context())

	conn := &scriptedConn{}
	conn.push(t, protocol.TypeLogin, protocol.Login{Name: "wachin", Password: "equivocada"})
	conn.push(t, protocol.TypeLogin, protocol.Login{Name: "wachin", Password: "seiscaracteres"})
	conn.push(t, protocol.TypeJoin, protocol.Join{})
	w.HandleConn(conn)

	types := conn.types()
	// The hello comes first on every connection now, so what matters is that an
	// error was answered at all, not that it was the very first frame.
	if !contains(types, protocol.TypeError) {
		t.Fatalf("la contraseña equivocada tenía que contestar un error: %v", types)
	}
	var welcome protocol.Welcome
	if !conn.payloadOf(t, protocol.TypeWelcome, &welcome) {
		t.Errorf("el segundo intento tenía que entrar: %v", types)
	}
}

// Registering a taken name fails as a registration rather than falling through
// to a sign-in, so a typo in somebody else's name never reads as "contraseña
// incorrecta".
func TestRegisteringATakenNameFails(t *testing.T) {
	w := newTestWorld(t, 40, 40)
	accounts := newFakeAccounts()
	if err := accounts.Register("wachin", "wachin@ejemplo.com", "seiscaracteres"); err != nil {
		t.Fatal(err)
	}
	w.SetAccounts(accounts)
	go w.Run(t.Context())

	conn := &scriptedConn{}
	conn.push(t, protocol.TypeLogin, protocol.Login{Name: "wachin", Email: "otro@ejemplo.com", Password: "otraclave", Register: true})
	w.HandleConn(conn)

	if types := conn.types(); !contains(types, protocol.TypeError) {
		t.Fatalf("se esperaba un error de nombre tomado: %v", types)
	}
	for _, typ := range conn.types() {
		if typ == protocol.TypeWelcome {
			t.Fatal("entró al mundo con una cuenta que no es suya")
		}
	}
}

// A server with no accounts is the server it always was.
func TestWithoutAccountsTheJoinStillCarriesTheName(t *testing.T) {
	w := newTestWorld(t, 40, 40)
	go w.Run(t.Context())

	conn := &scriptedConn{}
	conn.push(t, protocol.TypeJoin, protocol.Join{Name: "wachin"})
	w.HandleConn(conn)

	var welcome protocol.Welcome
	if !conn.payloadOf(t, protocol.TypeWelcome, &welcome) {
		t.Fatalf("no se entró al mundo sin cuentas: %v", conn.types())
	}
	if conn.payloadOf(t, protocol.TypeAccount, &protocol.Account{}) {
		t.Error("mandó una ficha de cuenta en un servidor sin cuentas")
	}
}

// Every player gets exactly one row per match: the eliminated when they die,
// the survivor when the match is called. Filing both places would double the
// dead, and filing only at the end would lose anybody who closed the client on
// their own corpse.
func TestEachPlayerIsRecordedOncePerMatch(t *testing.T) {
	w := matchWorld(t)
	accounts := newFakeAccounts()
	w.SetAccounts(accounts)

	a, _ := place(t, w, "a", 5, 5)
	b, _ := place(t, w, "b", 6, 6)
	c, _ := place(t, w, "c", 7, 7)
	w.setAccount(a.ID, "a")
	w.setAccount(b.ID, "b")
	w.setAccount(c.ID, "c")

	w.kill(b, a)
	w.kill(c, a)
	w.matchTick()

	accounts.mu.Lock()
	defer accounts.mu.Unlock()
	if len(accounts.recorded) != 3 {
		t.Fatalf("se registraron %d filas para 3 jugadores", len(accounts.recorded))
	}
	seen := map[string]protocol.Outcome{}
	for _, r := range accounts.recorded {
		if _, dup := seen[r.name]; dup {
			t.Errorf("%s quedó registrado dos veces en una partida", r.name)
		}
		seen[r.name] = r.out
	}
	if !seen["a"].Won {
		t.Error("el ganador no quedó registrado como ganador")
	}
	if seen["b"].Placement != 3 || seen["c"].Placement != 2 {
		t.Errorf("puestos mal registrados: b=%d c=%d", seen["b"].Placement, seen["c"].Placement)
	}
}

func contains(types []protocol.MsgType, want protocol.MsgType) bool {
	for _, typ := range types {
		if typ == want {
			return true
		}
	}
	return false
}
