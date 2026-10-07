// Package crypto provides token encryption, password hashing and secure tokens.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const cipherVersion = "v1"

// ErrDecrypt is returned for any malformed or tampered ciphertext.
var ErrDecrypt = errors.New("crypto: decryption failed")

// Cipher encrypts secrets with AES-256-GCM. Output format: "v1:" + base64(nonce|ciphertext).
type Cipher struct {
	aead cipher.AEAD
}

// NewCipherFromBase64 builds a Cipher from a base64-encoded 32-byte key.
func NewCipherFromBase64(b64 string) (*Cipher, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, fmt.Errorf("crypto: ENCRYPTION_KEY is not valid base64: %w", err)
	}
	return NewCipher(key)
}

// NewCipher builds a Cipher from a raw 32-byte key.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// KeyVersion returns the version tag written with ciphertexts.
func (c *Cipher) KeyVersion() string { return cipherVersion }

// Encrypt seals plaintext. Empty input yields empty output (no secret stored).
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), []byte(cipherVersion))
	return cipherVersion + ":" + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt opens a value produced by Encrypt.
func (c *Cipher) Decrypt(enc string) (string, error) {
	if enc == "" {
		return "", nil
	}
	version, payload, ok := strings.Cut(enc, ":")
	if !ok || version != cipherVersion {
		return "", ErrDecrypt
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(raw) < c.aead.NonceSize() {
		return "", ErrDecrypt
	}
	ns := c.aead.NonceSize()
	pt, err := c.aead.Open(nil, raw[:ns], raw[ns:], []byte(cipherVersion))
	if err != nil {
		return "", ErrDecrypt
	}
	return string(pt), nil
}
