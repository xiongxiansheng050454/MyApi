package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	return fromBase64(raw)
}

// FromEnvOrFile 优先使用环境变量 MYAPI_APIKEY_ENC_KEY；否则读取 keyFile；
// 若 keyFile 不存在则生成随机密钥并写入该文件（generated=true），保证重启后仍可解密。
func FromEnvOrFile(keyFile string) (s *Secret, generated bool, err error) {
	if sec, e := FromEnv(); e == nil {
		return sec, false, nil
	} else if !errors.Is(e, ErrNotConfigured) {
		return nil, false, e
	}

	if keyFile == "" {
		return nil, false, ErrNotConfigured
	}

	if raw, e := os.ReadFile(keyFile); e == nil {
		sec, e2 := fromBase64(strings.TrimSpace(string(raw)))
		return sec, false, e2
	} else if !os.IsNotExist(e) {
		return nil, false, e
	}

	key := make([]byte, 32)
	if _, e := rand.Read(key); e != nil {
		return nil, false, e
	}
	encoded := base64.StdEncoding.EncodeToString(key)
	Zero(key)

	if dir := filepath.Dir(keyFile); dir != "" && dir != "." {
		if e := os.MkdirAll(dir, 0o700); e != nil {
			return nil, false, e
		}
	}
	if e := os.WriteFile(keyFile, []byte(encoded), 0o600); e != nil {
		return nil, false, fmt.Errorf("generate encryption key: %w", e)
	}
	sec, e := fromBase64(encoded)
	return sec, true, e
}

func fromBase64(raw string) (*Secret, error) {
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
