package secret

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func testKey() []byte {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}

func withEnv(t *testing.T) {
	t.Helper()
	t.Setenv(EnvKey, base64.StdEncoding.EncodeToString(testKey()))
}

func TestRoundTrip(t *testing.T) {
	withEnv(t)
	s, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	cases := []string{"sk-abc", "very-long-secret-value-" + strings.Repeat("x", 300), ""}
	for _, in := range cases {
		enc, err := s.Encrypt(in)
		if err != nil {
			t.Fatalf("encrypt %q: %v", in, err)
		}
		if !strings.HasPrefix(enc, Prefix) {
			t.Fatalf("missing prefix: %s", enc)
		}
		out, err := s.Decrypt(enc)
		if err != nil {
			t.Fatalf("decrypt %q: %v", in, err)
		}
		if !bytes.Equal(out, []byte(in)) {
			t.Fatalf("roundtrip mismatch for %q", in)
		}
		Zero(out)
	}
}

func TestUniqueCiphertext(t *testing.T) {
	withEnv(t)
	s, _ := FromEnv()
	a, _ := s.Encrypt("sk-same")
	b, _ := s.Encrypt("sk-same")
	if a == b {
		t.Fatal("ciphertexts must differ due to random nonce")
	}
}

func TestPlaintextRejected(t *testing.T) {
	withEnv(t)
	s, _ := FromEnv()
	if _, err := s.Decrypt("sk-plain-leak"); err != ErrPlaintext {
		t.Fatalf("expected ErrPlaintext, got %v", err)
	}
}

func TestMissingKey(t *testing.T) {
	t.Setenv(EnvKey, "")
	if _, err := FromEnv(); err != ErrNotConfigured {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestMalformed(t *testing.T) {
	withEnv(t)
	s, _ := FromEnv()
	if _, err := s.Decrypt(Prefix + "!!not-base64"); err != ErrMalformed {
		t.Fatalf("expected ErrMalformed, got %v", err)
	}
	if _, err := s.Decrypt(Prefix + base64.StdEncoding.EncodeToString([]byte("short"))); err != ErrMalformed {
		t.Fatalf("expected ErrMalformed for short payload, got %v", err)
	}
}
