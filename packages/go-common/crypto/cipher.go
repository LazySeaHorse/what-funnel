// Package crypto provides AES-256-GCM encrypt/decrypt for sensitive fields
// (specifically ai_provider_config on the accounts table).
//
// Key management: the encryption key is sourced from ENCRYPTION_KEY env var,
// expected as a 32-character raw string (or 64-char hex — see NewKey).
// In production, rotate via secrets manager; re-encryption on key rotation is
// a v2 concern. For v1, a single static key per deployment is sufficient.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// ErrInvalidKey is returned when the provided key is not a valid length.
var ErrInvalidKey = errors.New("crypto: key must be 32 bytes (AES-256)")

// ErrInvalidCiphertext is returned when decryption fails (bad key, tampering, or truncation).
var ErrInvalidCiphertext = errors.New("crypto: invalid ciphertext")

// Cipher wraps an AES-GCM AEAD for encrypt/decrypt operations.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipherFromHex constructs a Cipher from a 64-hex-character (32-byte) key.
func NewCipherFromHex(hexKey string) (*Cipher, error) {
	raw, err := hex.DecodeString(hexKey)
	if err != nil {
		// Fall back: try treating the input as a raw 32-byte string
		raw = []byte(hexKey)
	}
	return newCipher(raw)
}

// NewCipherFromBytes constructs a Cipher from a raw 32-byte key.
func NewCipherFromBytes(key []byte) (*Cipher, error) {
	return newCipher(key)
}

func newCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("%w: got %d bytes", ErrInvalidKey, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: create GCM: %w", err)
	}
	return &Cipher{aead: gcm}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM and returns
// nonce || ciphertext as a hex-encoded string suitable for storage.
// A fresh random nonce is generated for every call.
func (c *Cipher) Encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: generate nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, plaintext, nil)
	return hex.EncodeToString(sealed), nil
}

// Decrypt decrypts a hex-encoded ciphertext produced by Encrypt.
func (c *Cipher) Decrypt(hexCiphertext string) ([]byte, error) {
	data, err := hex.DecodeString(hexCiphertext)
	if err != nil {
		return nil, fmt.Errorf("%w: hex decode: %v", ErrInvalidCiphertext, err)
	}

	nonceSize := c.aead.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("%w: too short", ErrInvalidCiphertext)
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCiphertext, err)
	}
	return plaintext, nil
}
