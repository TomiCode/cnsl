package secret

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSealOpen(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	b, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := b.Seal("my-token")
	if err != nil || !IsSealed(sealed) || strings.Contains(sealed, "my-token") {
		t.Fatalf("sealed = %q, %v", sealed, err)
	}
	again, _ := b.Seal("my-token")
	if again == sealed {
		t.Fatal("sealing twice must use a fresh nonce")
	}
	if twice, _ := b.Seal(sealed); twice != sealed {
		t.Fatal("sealing a sealed value must be a no-op")
	}
	if plain, err := b.Open(sealed); err != nil || plain != "my-token" {
		t.Fatalf("open = %q, %v", plain, err)
	}
	if plain, _ := b.Open("legacy plaintext"); plain != "legacy plaintext" {
		t.Fatal("plaintext from before encryption must pass through")
	}

	// Tampering or a different key fails.
	tampered := sealed[:len(sealed)-4] + "AAAA"
	if _, err := b.Open(tampered); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("tampered: %v", err)
	}
	other := make([]byte, 32)
	rand.Read(other)
	b2, _ := New(other)
	if _, err := b2.Open(sealed); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("other key: %v", err)
	}
}

func TestLoadKeyFile(t *testing.T) {
	dir := t.TempDir()
	b1, src, err := Load("", dir)
	if err != nil || !strings.HasSuffix(src, "secret.key") {
		t.Fatalf("first load: %v %q", err, src)
	}
	sealed, _ := b1.Seal("x")
	b2, _, err := Load("", dir) // second start reuses the file
	if err != nil {
		t.Fatal(err)
	}
	if plain, err := b2.Open(sealed); err != nil || plain != "x" {
		t.Fatalf("reload: %q %v", plain, err)
	}
	if info, err := os.Stat(filepath.Join(dir, "secret.key")); err != nil || info.Size() == 0 {
		t.Fatalf("key file: %v", err)
	}

	key := make([]byte, 32)
	if _, src, err := Load(base64.StdEncoding.EncodeToString(key), dir); err != nil || src != "CNSL_SECRET_KEY" {
		t.Fatalf("env key: %v %q", err, src)
	}
	if _, _, err := Load("not base64!", dir); err == nil {
		t.Fatal("invalid env key must fail")
	}
}
