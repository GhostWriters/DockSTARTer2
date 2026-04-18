package serve

import (
	"bufio"
	"os"

	"github.com/charmbracelet/ssh"
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

// authorizedKeysContains reports whether the given public key appears in the
// OpenSSH authorized_keys file at path. Lines that fail to parse are skipped.
func authorizedKeysContains(path string, key ssh.PublicKey) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		parsed, _, _, _, err := ssh.ParseAuthorizedKey(line)
		if err != nil {
			continue
		}
		if ssh.KeysEqual(parsed, key) {
			return true
		}
	}
	return false
}
