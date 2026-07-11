package integration

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/whatfunnel/whatfunnel/packages/go-common/crypto"
)

type CryptoTestCase struct {
	KeyHex     string `json:"key_hex"`
	Plaintext  string `json:"plaintext"`
	Ciphertext string `json:"ciphertext"` // hex encoded nonce || ciphertext
}

func TestCryptoInteropGoSide(t *testing.T) {
	// Paths for sharing test fixtures between Go and Python
	goEncFile := filepath.Join("..", "..", "services", "ai-kb-compiler", "tests", "go_encrypted.json")
	pythonEncFile := filepath.Join("..", "..", "services", "ai-kb-compiler", "tests", "python_encrypted.json")

	// 1. Generate Go encrypted fixtures for Python to decrypt
	testCases := []CryptoTestCase{
		{Plaintext: "hello from Go! standard ascii"},
		{Plaintext: "special characters: !@#$%^&*()_+={}[]|\\:;'<>,.?/~`"},
		{Plaintext: "unicode support: 🚀 🧑‍💻 🇨🇦 中文 UTF-8"},
	}

	for i := range testCases {
		// Generate random 32-byte key
		key := make([]byte, 32)
		_, err := rand.Read(key)
		require.NoError(t, err)
		keyHex := hex.EncodeToString(key)
		testCases[i].KeyHex = keyHex

		cipher, err := crypto.NewCipherFromHex(keyHex)
		require.NoError(t, err)

		ciphertext, err := cipher.Encrypt([]byte(testCases[i].Plaintext))
		require.NoError(t, err)
		testCases[i].Ciphertext = ciphertext
	}

	// Write fixtures to JSON file
	data, err := json.MarshalIndent(testCases, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(goEncFile, data, 0644)
	require.NoError(t, err)
	t.Logf("Wrote Go encrypted fixtures to %s", goEncFile)

	// 2. Read Python encrypted fixtures and decrypt them
	if _, err := os.Stat(pythonEncFile); err == nil {
		t.Logf("Found Python encrypted fixtures at %s. Decrypting...", pythonEncFile)
		pythonData, err := os.ReadFile(pythonEncFile)
		require.NoError(t, err)

		var pyTestCases []CryptoTestCase
		err = json.Unmarshal(pythonData, &pyTestCases)
		require.NoError(t, err)

		for _, tc := range pyTestCases {
			cipher, err := crypto.NewCipherFromHex(tc.KeyHex)
			require.NoError(t, err)

			decrypted, err := cipher.Decrypt(tc.Ciphertext)
			require.NoError(t, err, "Go failed to decrypt Python ciphertext")
			assert.Equal(t, tc.Plaintext, string(decrypted), "Go decrypted plaintext does not match Python's original")
		}
		t.Log("Successfully decrypted all Python fixtures!")
	} else {
		t.Log("No Python encrypted fixtures found yet. Run Python tests first to generate them.")
	}
}
