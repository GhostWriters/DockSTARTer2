package serve

import (
	"golang.org/x/crypto/bcrypt"
)

// checkPassword verifies a plaintext password against a bcrypt hash stored
// in dockstarter2.toml. Returns true if they match.
func checkPassword(plaintext, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext))
	return err == nil
}

// HashPassword returns a bcrypt hash of the plaintext password for storage
// in dockstarter2.toml. Used by future "set server password" TUI flow.
func HashPassword(plaintext string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
