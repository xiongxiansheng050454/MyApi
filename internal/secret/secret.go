package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	EnvKey = "MYAPI_APIKEY_ENC_KEY"
	Prefix = "enc:v1:"
)

var (
	ErrNotConfigured = errors.New("api key encryption key not configured")
	ErrPlaintext     = errors.New("stored api key is plaintext, run migrate-enc first")
	ErrMalformed     = errors.New("stored api key ciphertext is malformed")
)

type Secret struct {
	aead cipher.AEAD
}

func FromEnv() (*Secret, error) {
	raw := os.Getenv(EnvKey)
	if raw == "" {
		return nil, ErrNotConfigured
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("%s must be base64: %w", EnvKey, err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("%s must decode to exactly 32 bytes", EnvKey)
	}
	defer Zero(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Secret{aead: aead}, nil
}

func (s *Secret) Encrypt(plain string) (string, error) {
	if s == nil {
		return "", ErrNotConfigured
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := s.aead.Seal(nil, nonce, []byte(plain), nil)
	raw := make([]byte, 0, len(nonce)+len(ct))
	raw = append(raw, nonce...)
	raw = append(raw, ct...)
	return Prefix + base64.StdEncoding.EncodeToString(raw), nil
}

func (s *Secret) Decrypt(stored string) ([]byte, error) {
	if s == nil {
		return nil, ErrNotConfigured
	}
	if !strings.HasPrefix(stored, Prefix) {
		return nil, ErrPlaintext
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, Prefix))
	if err != nil {
		return nil, ErrMalformed
	}
	ns := s.aead.NonceSize()
	if len(raw) < ns {
		return nil, ErrMalformed
	}
	pt, err := s.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return nil, ErrMalformed
	}
	return pt, nil
}

func Zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
