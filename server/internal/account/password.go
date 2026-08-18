package account

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Password hashing.
//
// PBKDF2-HMAC-SHA256, from the standard library — crypto/pbkdf2 has been in it
// since Go 1.24, so the whole of this costs no dependency at all. argon2id
// would resist a GPU better and lives one module away in x/crypto; PBKDF2 at a
// high iteration count is the trade this project makes, in a repo that has
// exactly one dependency on purpose.
//
// What matters more than the choice of function is that the parameters travel
// with the hash. Every stored value names its own algorithm, iteration count
// and salt, so raising the cost later applies to new passwords without
// invalidating a single old one — the same record tells verify how to check it.

const (
	// pbkdf2Iterations is the work factor. OWASP's floor for PBKDF2-HMAC-SHA256
	// is 600.000, which is deliberately slow: a login takes a fraction of a
	// second and a stolen file takes centuries.
	pbkdf2Iterations = 600_000
	pbkdf2KeyLen     = 32
	saltLen          = 16
)

var errBadHash = errors.New("account: hash ilegible")

// hashPassword returns a self-describing record: algorithm, cost, salt, key.
func hashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("account: sin entropía para el salt: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iterations, pbkdf2KeyLen)
	if err != nil {
		return "", fmt.Errorf("account: pbkdf2: %w", err)
	}
	return strings.Join([]string{
		"pbkdf2-sha256",
		strconv.Itoa(pbkdf2Iterations),
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	}, "$"), nil
}

// verifyPassword checks a password against a stored record.
//
// The comparison is constant-time. A byte-by-byte one leaks how much of the key
// matched through how long it took to say no, which is enough to recover a hash
// one byte at a time.
func verifyPassword(stored, password string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}
