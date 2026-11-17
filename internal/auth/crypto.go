package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
)

// sealer encrypts Apple refresh tokens at rest with AES-256-GCM. The key is
// derived from SESSION_SECRET, so rotating that secret makes stored tokens
// unreadable (accounts can still be deleted, just without the Apple revoke).
type sealer struct {
	aead cipher.AEAD
}

func newSealer(secret string) (*sealer, error) {
	if secret == "" {
		return nil, errors.New("session secret is empty")
	}
	key, err := hkdf.Key(sha256.New, []byte(secret), nil, "campus-study-group/apple-refresh-token/v1", 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &sealer{aead: aead}, nil
}

// seal returns nonce || ciphertext.
func (s *sealer) seal(plaintext string) []byte {
	nonce := make([]byte, s.aead.NonceSize())
	_, _ = rand.Read(nonce) // never fails
	return s.aead.Seal(nonce, nonce, []byte(plaintext), nil)
}

func (s *sealer) open(sealed []byte) (string, error) {
	n := s.aead.NonceSize()
	if len(sealed) < n {
		return "", errors.New("ciphertext too short")
	}
	plain, err := s.aead.Open(nil, sealed[:n], sealed[n:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
