package ginboot

import (
	"crypto/rand"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

// DefaultPBKDF2Iterations is OWASP's recommendation for PBKDF2-HMAC-SHA512,
// used when PBKDF2_ENCODER_ITERATION is unset.
const DefaultPBKDF2Iterations = 210000

// DefaultPBKDF2KeyLength is used when PBKDF2_ENCODER_KEY_LENGTH is unset.
const DefaultPBKDF2KeyLength = 32

const (
	pbkdf2Prefix   = "pbkdf2-sha512$"
	pbkdf2SaltSize = 16
)

// PBKDF2Encoder hashes passwords with PBKDF2-HMAC-SHA512 and a random salt per
// password. A hash records its own iteration count and salt:
//
//	pbkdf2-sha512$<iterations>$<salt>$<key>
//
// so changing Iteration or KeyLength never invalidates a stored hash; it only
// makes NeedsRehash report true for the older ones.
//
// Secret is kept only to verify hashes written before salts were random, which
// used it as one salt shared by every password. New hashes never use it.
type PBKDF2Encoder struct {
	Secret    string
	Iteration int
	KeyLength int
}

func NewPBKDF2Encoder() *PBKDF2Encoder {
	return &PBKDF2Encoder{
		Secret:    os.Getenv("PBKDF2_ENCODER_SECRET"),
		Iteration: envPositiveInt("PBKDF2_ENCODER_ITERATION", DefaultPBKDF2Iterations),
		KeyLength: envPositiveInt("PBKDF2_ENCODER_KEY_LENGTH", DefaultPBKDF2KeyLength),
	}
}

// envPositiveInt reads name, falling back to def when it is unset. A value
// that is set but unusable panics, as before: silently hashing with a default
// the operator did not choose is worse than refusing to start.
func envPositiveInt(name string, def int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		panic(fmt.Sprintf("%s must be a positive integer, got %q", name, raw))
	}
	return n
}

func (P PBKDF2Encoder) GetPasswordHash(password string) (string, error) {
	salt := make([]byte, pbkdf2SaltSize)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}
	key := pbkdf2.Key([]byte(password), salt, P.Iteration, P.KeyLength, sha512.New)
	return fmt.Sprintf("%s%d$%s$%s", pbkdf2Prefix, P.Iteration,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

func (P PBKDF2Encoder) IsMatching(hash, password string) bool {
	if !strings.HasPrefix(hash, pbkdf2Prefix) {
		return P.isMatchingLegacy(hash, password)
	}
	iterations, salt, key, ok := parsePBKDF2Hash(hash)
	if !ok {
		return false
	}
	derived := pbkdf2.Key([]byte(password), salt, iterations, len(key), sha512.New)
	return subtle.ConstantTimeCompare(derived, key) == 1
}

// NeedsRehash reports whether hash should be replaced with a fresh
// GetPasswordHash the next time its password is known, typically right after
// a successful login. That is how stored hashes move off the legacy shared
// salt, and up to a raised Iteration or KeyLength, without a password reset.
func (P PBKDF2Encoder) NeedsRehash(hash string) bool {
	if !strings.HasPrefix(hash, pbkdf2Prefix) {
		return true
	}
	iterations, _, key, ok := parsePBKDF2Hash(hash)
	return !ok || iterations < P.Iteration || len(key) != P.KeyLength
}

// isMatchingLegacy verifies a hash written before per-password salts: the
// bare base64 key, salted with Secret. An encoder with no Secret cannot have
// written one, so it matches nothing rather than an empty salt.
func (P PBKDF2Encoder) isMatchingLegacy(hash, password string) bool {
	if P.Secret == "" {
		return false
	}
	key := pbkdf2.Key([]byte(password), []byte(P.Secret), P.Iteration, P.KeyLength, sha512.New)
	encoded := base64.StdEncoding.EncodeToString(key)
	return subtle.ConstantTimeCompare([]byte(encoded), []byte(hash)) == 1
}

func parsePBKDF2Hash(hash string) (iterations int, salt, key []byte, ok bool) {
	parts := strings.Split(strings.TrimPrefix(hash, pbkdf2Prefix), "$")
	if len(parts) != 3 {
		return 0, nil, nil, false
	}
	iterations, err := strconv.Atoi(parts[0])
	if err != nil || iterations <= 0 {
		return 0, nil, nil, false
	}
	if salt, err = base64.RawStdEncoding.DecodeString(parts[1]); err != nil || len(salt) == 0 {
		return 0, nil, nil, false
	}
	if key, err = base64.RawStdEncoding.DecodeString(parts[2]); err != nil || len(key) == 0 {
		return 0, nil, nil, false
	}
	return iterations, salt, key, true
}
