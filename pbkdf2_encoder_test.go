package ginboot

import (
	"crypto/sha512"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/pbkdf2"
)

// Low iteration count keeps the suite fast; the cost is not what is under test.
func testEncoder() PBKDF2Encoder {
	return PBKDF2Encoder{Secret: "shared-secret", Iteration: 1000, KeyLength: 32}
}

// legacyHash is exactly what GetPasswordHash produced before per-password
// salts, so the tests prove those stored hashes still verify.
func legacyHash(e PBKDF2Encoder, password string) string {
	key := pbkdf2.Key([]byte(password), []byte(e.Secret), e.Iteration, e.KeyLength, sha512.New)
	return base64.StdEncoding.EncodeToString(key)
}

func TestPBKDF2Encoder_RoundTrip(t *testing.T) {
	e := testEncoder()
	hash, err := e.GetPasswordHash("correct horse")
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(hash, "pbkdf2-sha512$1000$"))
	assert.True(t, e.IsMatching(hash, "correct horse"))
	assert.False(t, e.IsMatching(hash, "wrong horse"))
	assert.False(t, e.NeedsRehash(hash))
}

func TestPBKDF2Encoder_SamePasswordGetsDifferentHashes(t *testing.T) {
	e := testEncoder()
	a, err := e.GetPasswordHash("Summer2026!")
	require.NoError(t, err)
	b, err := e.GetPasswordHash("Summer2026!")
	require.NoError(t, err)

	assert.NotEqual(t, a, b, "a shared salt would make equal passwords visible in the table")
	assert.True(t, e.IsMatching(a, "Summer2026!"))
	assert.True(t, e.IsMatching(b, "Summer2026!"))
}

func TestPBKDF2Encoder_DoesNotUseSecret(t *testing.T) {
	e := testEncoder()
	hash, err := e.GetPasswordHash("pw")
	require.NoError(t, err)

	rotated := e
	rotated.Secret = "a different secret"
	assert.True(t, rotated.IsMatching(hash, "pw"))
}

func TestPBKDF2Encoder_VerifiesLegacyHashes(t *testing.T) {
	e := testEncoder()
	old := legacyHash(e, "pw")

	assert.True(t, e.IsMatching(old, "pw"))
	assert.False(t, e.IsMatching(old, "nope"))
	assert.True(t, e.NeedsRehash(old))
}

func TestPBKDF2Encoder_LegacyNeedsSecret(t *testing.T) {
	e := testEncoder()
	old := legacyHash(PBKDF2Encoder{Secret: "", Iteration: e.Iteration, KeyLength: e.KeyLength}, "pw")

	e.Secret = ""
	assert.False(t, e.IsMatching(old, "pw"))
}

func TestPBKDF2Encoder_HashOutlivesConfigChange(t *testing.T) {
	e := testEncoder()
	hash, err := e.GetPasswordHash("pw")
	require.NoError(t, err)

	raised := e
	raised.Iteration = 2000
	raised.KeyLength = 64
	assert.True(t, raised.IsMatching(hash, "pw"), "the hash records its own parameters")
	assert.True(t, raised.NeedsRehash(hash))
}

func TestPBKDF2Encoder_RejectsMalformedHashes(t *testing.T) {
	e := testEncoder()
	for _, h := range []string{
		"pbkdf2-sha512$",
		"pbkdf2-sha512$1000$salt",
		"pbkdf2-sha512$abc$c2FsdA$a2V5",
		"pbkdf2-sha512$0$c2FsdA$a2V5",
		"pbkdf2-sha512$1000$!!!$a2V5",
		"pbkdf2-sha512$1000$$a2V5",
		"pbkdf2-sha512$1000$c2FsdA$",
	} {
		assert.False(t, e.IsMatching(h, "pw"), h)
		assert.True(t, e.NeedsRehash(h), h)
	}
}

func TestNewPBKDF2Encoder_Defaults(t *testing.T) {
	t.Setenv("PBKDF2_ENCODER_SECRET", "")
	t.Setenv("PBKDF2_ENCODER_ITERATION", "")
	t.Setenv("PBKDF2_ENCODER_KEY_LENGTH", "")

	e := NewPBKDF2Encoder()
	assert.Equal(t, DefaultPBKDF2Iterations, e.Iteration)
	assert.Equal(t, DefaultPBKDF2KeyLength, e.KeyLength)
}

func TestNewPBKDF2Encoder_ReadsEnv(t *testing.T) {
	t.Setenv("PBKDF2_ENCODER_SECRET", "s")
	t.Setenv("PBKDF2_ENCODER_ITERATION", "300000")
	t.Setenv("PBKDF2_ENCODER_KEY_LENGTH", "64")

	e := NewPBKDF2Encoder()
	assert.Equal(t, "s", e.Secret)
	assert.Equal(t, 300000, e.Iteration)
	assert.Equal(t, 64, e.KeyLength)
}

func TestNewPBKDF2Encoder_PanicsOnInvalidEnv(t *testing.T) {
	t.Setenv("PBKDF2_ENCODER_ITERATION", "ten thousand")
	assert.PanicsWithValue(t,
		`PBKDF2_ENCODER_ITERATION must be a positive integer, got "ten thousand"`,
		func() { NewPBKDF2Encoder() })

	t.Setenv("PBKDF2_ENCODER_ITERATION", "1000")
	t.Setenv("PBKDF2_ENCODER_KEY_LENGTH", "-1")
	assert.Panics(t, func() { NewPBKDF2Encoder() })
}
