package account

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cuentas.log")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func TestRegisterAndAuthenticate(t *testing.T) {
	s, _ := openTemp(t)

	if err := s.Register("wachin", "seiscaracteres"); err != nil {
		t.Fatalf("register: %v", err)
	}
	name, err := s.Authenticate("wachin", "seiscaracteres")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if name != "wachin" {
		t.Errorf("nombre = %q", name)
	}

	if _, err := s.Authenticate("wachin", "otra cosa"); err != ErrBadPassword {
		t.Errorf("con contraseña incorrecta dio %v, se esperaba ErrBadPassword", err)
	}
	if _, err := s.Authenticate("nadie", "seiscaracteres"); err != ErrNoSuchUser {
		t.Errorf("con cuenta inexistente dio %v, se esperaba ErrNoSuchUser", err)
	}
}

// The password must never be recoverable from the file, which is the entire
// point of hashing it. A store that wrote it in the clear would pass every
// other test in here.
func TestThePasswordNeverReachesTheDisk(t *testing.T) {
	s, path := openTemp(t)
	const secret = "manzanaroja2026"

	if err := s.Register("wachin", secret); err != nil {
		t.Fatalf("register: %v", err)
	}
	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(blob), secret) {
		t.Fatal("la contraseña quedó escrita en texto plano en el log")
	}
	if !strings.Contains(string(blob), "pbkdf2-sha256") {
		t.Error("el registro no dice con qué se hasheó, así que no se puede migrar")
	}
}

// Two accounts with the same password must not share a hash, or the file tells
// an attacker which players to crack together.
func TestSamePasswordHashesDifferently(t *testing.T) {
	s, _ := openTemp(t)
	if err := s.Register("uno", "lamismaclave"); err != nil {
		t.Fatal(err)
	}
	if err := s.Register("dos", "lamismaclave"); err != nil {
		t.Fatal(err)
	}

	one, _ := s.Profile("uno")
	two, _ := s.Profile("dos")
	_ = one
	_ = two

	s.mu.RLock()
	h1 := s.accounts["uno"].Hash
	h2 := s.accounts["dos"].Hash
	s.mu.RUnlock()
	if h1 == h2 {
		t.Error("misma contraseña, mismo hash: falta el salt")
	}
}

// Names are an identity now, not a label, so the same name in different case is
// the same account -- otherwise anybody can register "Wachin" next to "wachin".
func TestNamesAreCaseInsensitive(t *testing.T) {
	s, _ := openTemp(t)
	if err := s.Register("Wachin", "seiscaracteres"); err != nil {
		t.Fatal(err)
	}
	if err := s.Register("wachin", "otraclave"); err != ErrNameTaken {
		t.Errorf("dio %v, se esperaba ErrNameTaken", err)
	}
	// And logging in with any casing lands on the registered spelling.
	name, err := s.Authenticate("WACHIN", "seiscaracteres")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if name != "Wachin" {
		t.Errorf("nombre = %q, se esperaba la grafía registrada", name)
	}
}

func TestRejectsBadNamesAndShortPasswords(t *testing.T) {
	s, _ := openTemp(t)
	for _, name := range []string{"ab", "con espacio", "diecisieteletrass", "acentó", ""} {
		if err := s.Register(name, "seiscaracteres"); err != ErrBadName {
			t.Errorf("nombre %q dio %v, se esperaba ErrBadName", name, err)
		}
	}
	if err := s.Register("valido", "corta"); err != ErrShortPass {
		t.Errorf("contraseña corta dio %v, se esperaba ErrShortPass", err)
	}
}

// The career is the point: it has to survive the process.
func TestProfileSurvivesAReopen(t *testing.T) {
	s, path := openTemp(t)
	if err := s.Register("wachin", "seiscaracteres"); err != nil {
		t.Fatal(err)
	}
	matches := []Match{
		{Placement: 1, Players: 1002, Kills: 23, Seconds: 622, Won: true, Map: "Confín"},
		{Placement: 37, Players: 100, Kills: 2, Seconds: 180},
		{Placement: 4, Players: 50, Kills: 5, Seconds: 300},
	}
	for _, m := range matches {
		if err := s.Record("wachin", m); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	p, err := reopened.Profile("wachin")
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	if p.Matches != 3 || p.Wins != 1 || p.Kills != 30 || p.Best != 1 {
		t.Errorf("perfil = %d partidas, %d victorias, %d bajas, mejor %d",
			p.Matches, p.Wins, p.Kills, p.Best)
	}
	if p.Seconds != 1102 {
		t.Errorf("tiempo total = %.0f, se esperaban 1102", p.Seconds)
	}
	// Newest first, so the account screen reads top-down.
	if len(p.Recent) != 3 || p.Recent[0].Placement != 4 {
		t.Errorf("historial mal ordenado: %+v", p.Recent)
	}
	// And the password still works after the reload.
	if _, err := reopened.Authenticate("wachin", "seiscaracteres"); err != nil {
		t.Errorf("no se pudo entrar tras reabrir: %v", err)
	}
}

// A half-written last line is what a power cut leaves behind. Losing it is
// fine; losing every account before it is not.
func TestATornLastLineDoesNotLoseTheFile(t *testing.T) {
	s, path := openTemp(t)
	if err := s.Register("wachin", "seiscaracteres"); err != nil {
		t.Fatal(err)
	}
	if err := s.Record("wachin", Match{Placement: 1, Players: 10, Kills: 3, Won: true}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Cut the file mid-way through a third line that was never finished.
	torn := append(blob, []byte(`{"t":"match","name":"wach`)...)
	if err := os.WriteFile(path, torn, 0o600); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("una linea cortada no puede impedir abrir el archivo: %v", err)
	}
	defer reopened.Close()

	p, err := reopened.Profile("wachin")
	if err != nil {
		t.Fatalf("se perdió la cuenta por una línea cortada: %v", err)
	}
	if p.Matches != 1 {
		t.Errorf("partidas = %d, se esperaba 1: la cortada se descarta, la buena no", p.Matches)
	}
}

// Recording for somebody who never registered must fail rather than invent an
// account, or a typo in a name silently creates a second career.
func TestRecordingForAnUnknownAccountFails(t *testing.T) {
	s, _ := openTemp(t)
	if err := s.Record("fantasma", Match{Placement: 1}); err != ErrNoSuchUser {
		t.Errorf("dio %v, se esperaba ErrNoSuchUser", err)
	}
}

// Only the last few matches are kept, so a profile stays one message.
func TestRecentHistoryIsBounded(t *testing.T) {
	s, _ := openTemp(t)
	if err := s.Register("wachin", "seiscaracteres"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < recentKept+5; i++ {
		if err := s.Record("wachin", Match{Placement: i + 1, Players: 20, PlayedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	p, _ := s.Profile("wachin")
	if len(p.Recent) != recentKept {
		t.Errorf("historial de %d, se esperaban %d", len(p.Recent), recentKept)
	}
	if p.Matches != recentKept+5 {
		t.Errorf("el total tiene que contar todas: %d", p.Matches)
	}
	// The newest is the one with the highest placement number here.
	if p.Recent[0].Placement != recentKept+5 {
		t.Errorf("el primero del historial no es el más nuevo: %+v", p.Recent[0])
	}
}

func TestLeaderboardRanksByWinsThenKills(t *testing.T) {
	s, _ := openTemp(t)
	for _, name := range []string{"uno", "dos", "tres", "cuatro"} {
		if err := s.Register(name, "seiscaracteres"); err != nil {
			t.Fatal(err)
		}
	}
	s.Record("uno", Match{Placement: 1, Won: true, Kills: 5})
	s.Record("dos", Match{Placement: 1, Won: true, Kills: 9})
	s.Record("tres", Match{Placement: 3, Kills: 40})
	// "cuatro" registered and never played: it must not appear at all.

	board := s.Leaderboard(10)
	if len(board) != 3 {
		t.Fatalf("la tabla tiene %d, se esperaban 3 (el que nunca jugó no entra)", len(board))
	}
	if board[0].Name != "dos" || board[1].Name != "uno" || board[2].Name != "tres" {
		t.Errorf("orden = %s, %s, %s", board[0].Name, board[1].Name, board[2].Name)
	}
}
