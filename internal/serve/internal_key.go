package serve

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"

	gossh "golang.org/x/crypto/ssh"
)

// internalKeyPair holds the ephemeral key pair used by the web proxy to
// authenticate with the local wish SSH server. Generated once at server
// startup and never written to disk.
type internalKeyPair struct {
	Signer    gossh.Signer
	PublicKey gossh.PublicKey
}

// generateInternalKey creates a fresh ed25519 key pair for the internal
// web-proxy client. The private key is held only in memory.
func generateInternalKey() (*internalKeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating internal key: %w", err)
	}

	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		return nil, fmt.Errorf("creating signer: %w", err)
	}

	cryptoPub, err := gossh.NewPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("creating public key: %w", err)
	}

	return &internalKeyPair{
		Signer:    signer,
		PublicKey: cryptoPub,
	}, nil
}
