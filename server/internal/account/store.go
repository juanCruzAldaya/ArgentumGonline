package account

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Store is every account and every match ever played on this server.
//
// The file is one JSON object per line, each tagged with what it is. Loading is
// replaying it; writing is appending one line and flushing it to disk. There is
// no rewrite path and no compaction, because nothing here is ever edited: a
// registration happened, a match happened, and neither un-happens.
//
// The in-memory side is the whole state, so a read never touches the disk. That
// is what lets a profile be answered inside a websocket handler without either
// blocking on IO or handing the world goroutine a database.
type Store struct {
	// mu guards everything below. This is the one place in the server that has
	// a mutex, and it is here for a reason the world does not share: the world
	// is a single goroutine that owns its state, while this is read from every
	// connection's own goroutine and written from the recorder.
	mu       sync.RWMutex
	path     string
	file     *os.File
	closed   bool
	accounts map[string]*record // by folded name
}

type record struct {
	Name string
	// Email is collected at registration and never shown to anybody, including
	// its owner. Nothing in the server sends mail; it is kept so there is a way
	// to reach a player later, which makes it the only personal data this
	// project holds — see Register for what that costs.
	Email     string
	Hash      string
	CreatedAt time.Time

	matches int
	wins    int
	kills   int
	best    int
	seconds float64
	recent  []Match
}

// The line formats. Each carries its own type tag so the log can grow a third
// kind of entry later without any older file becoming unreadable.
type line struct {
	Type string `json:"t"`

	Name      string    `json:"name,omitempty"`
	Email     string    `json:"email,omitempty"`
	Hash      string    `json:"hash,omitempty"`
	CreatedAt time.Time `json:"at,omitempty"`

	Match *Match `json:"m,omitempty"`
}

const (
	lineAccount = "account"
	lineMatch   = "match"
)

// Open loads the log at path, creating it if it is not there.
//
// A line that will not parse is skipped rather than fatal, and that is
// deliberate: the last write before a power cut is the one that can be half
// written, and refusing to start over a torn final line would lose every
// account to protect the newest one.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("account: no se pudo crear %s: %w", dir, err)
		}
	}

	s := &Store{path: path, accounts: make(map[string]*record)}
	if err := s.load(); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("account: no se pudo abrir %s: %w", path, err)
	}
	s.file = file
	return s, nil
}

func (s *Store) load() error {
	file, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("account: no se pudo leer %s: %w", s.path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Accounts and matches are small, but a name plus a hash plus timestamps
	// can pass the default 64 KB ceiling if the format ever grows; this makes
	// the limit an explicit decision rather than a surprise at load.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		var ln line
		if err := json.Unmarshal(scanner.Bytes(), &ln); err != nil {
			continue // torn or unknown line: skip it, keep the rest
		}
		switch ln.Type {
		case lineAccount:
			if ln.Name == "" {
				continue
			}
			s.accounts[foldName(ln.Name)] = &record{
				Name: ln.Name, Email: ln.Email, Hash: ln.Hash, CreatedAt: ln.CreatedAt,
			}
		case lineMatch:
			rec, ok := s.accounts[foldName(ln.Name)]
			if !ok || ln.Match == nil {
				continue
			}
			rec.apply(*ln.Match)
		}
	}
	return scanner.Err()
}

// apply folds one match into the running totals. Aggregates are kept as the log
// replays rather than recomputed per request, so a profile is a struct copy.
func (r *record) apply(m Match) {
	r.matches++
	r.kills += m.Kills
	r.seconds += m.Seconds
	if m.Won {
		r.wins++
	}
	if m.Placement > 0 && (r.best == 0 || m.Placement < r.best) {
		r.best = m.Placement
	}
	r.recent = append(r.recent, m)
	if len(r.recent) > recentKept {
		r.recent = r.recent[len(r.recent)-recentKept:]
	}
}

// append writes one line and makes sure it is on the disk before returning.
//
// Sync on every write because these are rare — a registration, and one row per
// player per match — and because the thing being protected is somebody's
// account. Buffering them would trade a real guarantee for a saving nobody
// would ever measure.
func (s *Store) append(ln line) error {
	if s.closed {
		return ErrStoreClosed
	}
	blob, err := json.Marshal(ln)
	if err != nil {
		return err
	}
	if _, err := s.file.Write(append(blob, '\n')); err != nil {
		return err
	}
	return s.file.Sync()
}

// Register creates an account. The name is taken case-insensitively, so nobody
// can register the visually identical name of somebody already playing here.
//
// The email is required and stored in the clear, which is a deliberate decision
// and the one thing in this package worth arguing about. The log is append-only
// by design: there is no rewrite path, so an address written here cannot be
// edited or deleted without rewriting the file by hand, and it is not
// encrypted. That is fine for a prototype whose account file lives on one Fly
// volume; it is the first thing to revisit if this ever holds real players.
// Nothing sends mail, so the address is only ever written, never read back out.
//
// Two addresses can register the same email on purpose. Uniqueness would only
// matter for a recovery flow, which does not exist, and enforcing it now would
// turn "is that address already here" into something an outsider could probe.
func (s *Store) Register(name, email, password string) error {
	if !validName(name) {
		return ErrBadName
	}
	email = foldEmail(email)
	if !validEmail(email) {
		return ErrBadEmail
	}
	if len(password) < MinPasswordLen {
		return ErrShortPass
	}

	hash, err := hashPassword(password)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	key := foldName(name)
	if _, taken := s.accounts[key]; taken {
		return ErrNameTaken
	}

	created := time.Now().UTC().Truncate(time.Second)
	if err := s.append(line{Type: lineAccount, Name: name, Email: email, Hash: hash, CreatedAt: created}); err != nil {
		return err
	}
	s.accounts[key] = &record{Name: name, Email: email, Hash: hash, CreatedAt: created}
	return nil
}

// Authenticate checks a name and password and returns the stored spelling of
// the name, which is the one the player registered rather than however they
// typed it this time.
func (s *Store) Authenticate(name, password string) (string, error) {
	s.mu.RLock()
	rec, ok := s.accounts[foldName(name)]
	s.mu.RUnlock()
	if !ok {
		// The password is still verified against a dummy hash so that a
		// missing account and a wrong password take the same time to answer.
		// Without it, the time to say no is a way to enumerate who plays here.
		verifyPassword(dummyHash, password)
		return "", ErrNoSuchUser
	}
	if !verifyPassword(rec.Hash, password) {
		return "", ErrBadPassword
	}
	return rec.Name, nil
}

// dummyHash is a real record of a password nobody has, so the failure path for
// an unknown account does the same work as the one for a wrong password.
var dummyHash = func() string {
	h, err := hashPassword("cuenta que no existe")
	if err != nil {
		return ""
	}
	return h
}()

// Record stores one finished match for one account.
func (s *Store) Record(name string, m Match) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.accounts[foldName(name)]
	if !ok {
		return ErrNoSuchUser
	}
	if m.PlayedAt.IsZero() {
		m.PlayedAt = time.Now().UTC().Truncate(time.Second)
	}
	if err := s.append(line{Type: lineMatch, Name: rec.Name, Match: &m}); err != nil {
		return err
	}
	rec.apply(m)
	return nil
}

// Profile is one account's career.
func (s *Store) Profile(name string) (Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.accounts[foldName(name)]
	if !ok {
		return Profile{}, ErrNoSuchUser
	}

	// Newest first, and a copy: the caller must not be handed the slice the
	// store keeps appending to.
	recent := make([]Match, len(rec.recent))
	for i, m := range rec.recent {
		recent[len(rec.recent)-1-i] = m
	}

	return Profile{
		Name:      rec.Name,
		CreatedAt: rec.CreatedAt,
		Matches:   rec.matches,
		Wins:      rec.wins,
		Kills:     rec.kills,
		Best:      rec.best,
		Seconds:   rec.seconds,
		Recent:    recent,
	}, nil
}

// Leaderboard is the accounts with the most wins, best first. Ties break on
// kills, then on name, so the order is stable rather than whatever the map
// felt like.
func (s *Store) Leaderboard(limit int) []Profile {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Profile, 0, len(s.accounts))
	for _, rec := range s.accounts {
		if rec.matches == 0 {
			continue
		}
		out = append(out, Profile{
			Name: rec.Name, CreatedAt: rec.CreatedAt,
			Matches: rec.matches, Wins: rec.wins, Kills: rec.kills,
			Best: rec.best, Seconds: rec.seconds,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Wins != out[j].Wins {
			return out[i].Wins > out[j].Wins
		}
		if out[i].Kills != out[j].Kills {
			return out[i].Kills > out[j].Kills
		}
		return out[i].Name < out[j].Name
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Count is how many accounts exist, for the startup log.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.accounts)
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.file.Close()
}
