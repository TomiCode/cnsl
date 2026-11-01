// Package secret encrypts small values at rest (connector tokens, API headers) with AES-256-GCM.
//
// The key comes from CNSL_SECRET_KEY (base64, 32 bytes). Without it, a key is generated once and
// kept in <data>/secret.key — convenient, but anyone with the data directory then has the key
// too, so for real protection keep the key outside it (env var, Docker secret).
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

// prefix marks encrypted values, so plaintext from before encryption can be told apart.
const prefix = "enc:v1:"

// ErrDecrypt means a value could not be decrypted, usually because the key changed.
var ErrDecrypt = errors.New("cannot decrypt secret (was CNSL_SECRET_KEY changed?)")

type Box struct {
	aead cipher.AEAD
}

// New creates a Box from a 32-byte key.
func New(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("secret key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Load returns the Box for the configured key: envKey (base64) if set, otherwise the key file in
// dataDir, which is created on first use. The second result says where the key came from.
func Load(envKey, dataDir string) (*Box, string, error) {
	if envKey != "" {
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(envKey))
		if err != nil {
			return nil, "", fmt.Errorf("CNSL_SECRET_KEY must be base64: %w", err)
		}
		b, err := New(key)
		return b, "CNSL_SECRET_KEY", err
	}

	path := filepath.Join(dataDir, "secret.key")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, "", err
		}
		data = []byte(base64.StdEncoding.EncodeToString(key) + "\n")
		// O_EXCL: never overwrite a key that appeared meanwhile.
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return nil, "", fmt.Errorf("create secret key: %w", err)
		}
		if _, err := f.Write(data); err != nil {
			f.Close()
			return nil, "", err
		}
		if err := f.Close(); err != nil {
			return nil, "", err
		}
	} else if err != nil {
		return nil, "", fmt.Errorf("read secret key: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, "", fmt.Errorf("%s is not valid base64: %w", path, err)
	}
	b, err := New(key)
	return b, path, err
}

// Seal encrypts a value. Empty values and values that are already sealed are returned as is.
func (b *Box) Seal(plain string) (string, error) {
	if plain == "" || IsSealed(plain) {
		return plain, nil
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := b.aead.Seal(nonce, nonce, []byte(plain), []byte(prefix))
	return prefix + base64.StdEncoding.EncodeToString(ct), nil
}

// Open decrypts a sealed value. Values without the prefix (stored before encryption) are
// returned unchanged.
func (b *Box) Open(value string) (string, error) {
	if !IsSealed(value) {
		return value, nil
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil || len(data) < b.aead.NonceSize() {
		return "", ErrDecrypt
	}
	nonce, ct := data[:b.aead.NonceSize()], data[b.aead.NonceSize():]
	plain, err := b.aead.Open(nil, nonce, ct, []byte(prefix))
	if err != nil {
		return "", ErrDecrypt
	}
	return string(plain), nil
}

func IsSealed(value string) bool {
	return strings.HasPrefix(value, prefix)
}
