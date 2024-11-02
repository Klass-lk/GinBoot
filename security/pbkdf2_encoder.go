package security

import (
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"golang.org/x/crypto/pbkdf2"
	"os"
	"strconv"
)

type PBKDF2Encoder struct {
	Secret    string
	Iteration int
	KeyLength int
}

func NewPBKDF2Encoder() *PBKDF2Encoder {
	secret := os.Getenv("PBKDF2_ENCODER_SECRET")
	iteration, err := strconv.ParseInt(os.Getenv("PBKDF2_ENCODER_ITERATION"), 10, 64)
	if err != nil {
		panic("ENCODER_ITERATIONS_MISSING")
	}
	keyLength, err := strconv.ParseInt(os.Getenv("PBKDF2_ENCODER_KEY_LENGTH"), 10, 64)
	if err != nil {
		panic("ENCODER_ITERATIONS_MISSING")
	}
	return &PBKDF2Encoder{secret, int(iteration), int(keyLength)}
}

func (P PBKDF2Encoder) GetPasswordHash(password string) (string, error) {
	hash := pbkdf2.Key([]byte(password), []byte(P.Secret), P.Iteration, P.KeyLength, sha512.New)
	encoded := base64.StdEncoding.EncodeToString(hash)
	return encoded, nil
}

func (P PBKDF2Encoder) IsMatching(hash, password string) bool {
	encodedCharSeq, _ := P.GetPasswordHash(password)
	return subtle.ConstantTimeCompare([]byte(encodedCharSeq), []byte(hash)) == 1
}
