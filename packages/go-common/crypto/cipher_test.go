package crypto_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/crypto"
)

// 32-byte key for testing (DO NOT use in production).
const testKey = "test-key-exactly-32-bytes-padded"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c, err := crypto.NewCipherFromBytes([]byte(testKey))
	require.NoError(t, err)

	plaintext := []byte(`{"api_key":"sk-test-1234","base_url":"https://api.openai.com/v1"}`)

	ciphertext, err := c.Encrypt(plaintext)
	require.NoError(t, err)
	assert.NotEmpty(t, ciphertext)
	assert.NotEqual(t, string(plaintext), ciphertext, "ciphertext must differ from plaintext")

	recovered, err := c.Decrypt(ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, recovered, "decrypted value must match original plaintext")
}

// TestEncryptProducesDifferentOutputEachCall ensures the nonce is fresh per call.
func TestEncryptProducesDifferentOutputEachCall(t *testing.T) {
	c, err := crypto.NewCipherFromBytes([]byte(testKey))
	require.NoError(t, err)

	plaintext := []byte("same plaintext")
	ct1, err := c.Encrypt(plaintext)
	require.NoError(t, err)
	ct2, err := c.Encrypt(plaintext)
	require.NoError(t, err)
	assert.NotEqual(t, ct1, ct2, "two encryptions of the same plaintext must produce different ciphertexts (different nonces)")
}

// TestDecryptWrongKey ensures decryption fails when the key doesn't match.
func TestDecryptWrongKey(t *testing.T) {
	c1, err := crypto.NewCipherFromBytes([]byte(testKey))
	require.NoError(t, err)

	wrongKey := "wrong-key-exactly-32-bytes-paddd"
	c2, err := crypto.NewCipherFromBytes([]byte(wrongKey))
	require.NoError(t, err)

	ciphertext, err := c1.Encrypt([]byte("secret data"))
	require.NoError(t, err)

	_, err = c2.Decrypt(ciphertext)
	assert.ErrorIs(t, err, crypto.ErrInvalidCiphertext, "decryption with wrong key must fail")
}

// TestDecryptTamperedCiphertext ensures tampered bytes are detected.
func TestDecryptTamperedCiphertext(t *testing.T) {
	c, err := crypto.NewCipherFromBytes([]byte(testKey))
	require.NoError(t, err)

	ciphertext, err := c.Encrypt([]byte("secret"))
	require.NoError(t, err)

	decoded, err := hex.DecodeString(ciphertext)
	require.NoError(t, err)
	decoded[len(decoded)-1] ^= 0xff
	tampered := hex.EncodeToString(decoded)
	_, err = c.Decrypt(tampered)
	assert.Error(t, err, "tampered ciphertext must fail decryption")
}

// TestInvalidKeyLength ensures NewCipherFromBytes rejects bad key lengths.
func TestInvalidKeyLength(t *testing.T) {
	_, err := crypto.NewCipherFromBytes([]byte("short"))
	assert.ErrorIs(t, err, crypto.ErrInvalidKey)
}

// TestEncryptEmptyPlaintext ensures the cipher handles empty input gracefully.
func TestEncryptEmptyPlaintext(t *testing.T) {
	c, err := crypto.NewCipherFromBytes([]byte(testKey))
	require.NoError(t, err)

	ciphertext, err := c.Encrypt([]byte{})
	require.NoError(t, err)

	recovered, err := c.Decrypt(ciphertext)
	require.NoError(t, err)
	// GCM Open for empty input returns nil, which is semantically equivalent to empty.
	assert.Len(t, recovered, 0)
}

// TestNewCipherFromHex ensures hex-encoded and raw keys work properly without ambiguous fallback heuristics.
func TestNewCipherFromHex(t *testing.T) {
	t.Run("valid 64-char hex key", func(t *testing.T) {
		hexKey := strings.Repeat("ab", 32) // 64 chars
		c, err := crypto.NewCipherFromHex(hexKey)
		require.NoError(t, err)

		ct, err := c.Encrypt([]byte("hello"))
		require.NoError(t, err)
		plain, err := c.Decrypt(ct)
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), plain)
	})

	t.Run("invalid 64-char hex key reports hex error not length error", func(t *testing.T) {
		// 64 chars but contains non-hex 'z'
		invalidHex := strings.Repeat("ab", 31) + "zz"
		_, err := crypto.NewCipherFromHex(invalidHex)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid 64-character hex key")
	})

	t.Run("32-char raw key containing only hex digits is not misdecoded as 16 bytes", func(t *testing.T) {
		// "0123456789abcdef0123456789abcdef" is 32 chars of all hex digits.
		// Old fallback would hex.DecodeString to 16 bytes and fail.
		rawHexDigitsKey := "0123456789abcdef0123456789abcdef"
		c, err := crypto.NewCipherFromHex(rawHexDigitsKey)
		require.NoError(t, err)

		ct, err := c.Encrypt([]byte("hello"))
		require.NoError(t, err)
		plain, err := c.Decrypt(ct)
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), plain)
	})

	t.Run("32-char raw key with non-hex characters works", func(t *testing.T) {
		rawKey := "change-me-32-byte-hex-key-padded"
		c, err := crypto.NewCipherFromHex(rawKey)
		require.NoError(t, err)

		ct, err := c.Encrypt([]byte("hello"))
		require.NoError(t, err)
		plain, err := c.Decrypt(ct)
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), plain)
	})

	t.Run("invalid lengths return descriptive error", func(t *testing.T) {
		lengths := []int{0, 16, 31, 33, 63, 65}
		for _, l := range lengths {
			key := strings.Repeat("a", l)
			_, err := crypto.NewCipherFromHex(key)
			require.ErrorIs(t, err, crypto.ErrInvalidKey)
			assert.Contains(t, err.Error(), "64 hex characters or 32 raw bytes")
		}
	})
}
