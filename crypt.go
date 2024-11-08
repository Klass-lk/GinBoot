package ginboot

import "golang.org/x/crypto/bcrypt"

type Crypt struct {
}

func NewCrypt() *Crypt {
	return &Crypt{}
}

func (Crypt) GetPasswordHash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 5)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (Crypt) IsMatching(hash, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
