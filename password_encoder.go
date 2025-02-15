package ginboot

type PasswordEncoder interface {
	GetPasswordHash(password string) (string, error)
	IsMatching(hash, password string) bool
}
