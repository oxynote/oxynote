package cryptoutil

import (
	"crypto/aes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_EncryptText(t *testing.T) {
	// Invalid key length.
	encryptedText, err := EncryptText("text", []byte("key"))
	require.Equal(t, aes.KeySizeError(3), err)
	assert.Empty(t, encryptedText)

	// Success.
	encryptedText, err = EncryptText("text", []byte("keykeykeykeykeykeykeykeykeykeyke"))
	require.NoError(t, err)
	assert.NotEmpty(t, encryptedText)
}

func Test_DecryptText(t *testing.T) {
	// Invalid hex.
	text, err := DecryptText("3", []byte("key"))
	require.Contains(t, err.Error(), "encoding/hex: odd length hex string")
	assert.Empty(t, text)

	// Invalid key length.
	text, err = DecryptText("36e00b9632148fc4793599ebfe5e110f30fa446bd431b1379708217c20e0736e", []byte("key"))
	require.Equal(t, aes.KeySizeError(3), err)
	assert.Empty(t, text)

	// Invalid encrypted text.
	text, err = DecryptText("36e00b9632148fc4793599ebfe5e110f30fa446bd431b1379708217c20ee", []byte("keykeykeykeykeykeykeykeykeykeyke"))
	require.Contains(t, err.Error(), "cipher: message authentication failed")
	assert.Empty(t, text)

	// Invalid encrypted text length.
	text, err = DecryptText("36", []byte("keykeykeykeykeykeykeykeykeykeyke"))
	require.Contains(t, err.Error(), "ciphertext too short")
	assert.Empty(t, text)

	// Success.
	text, err = DecryptText("36e00b9632148fc4793599ebfe5e110f30fa446bd431b1379708217c20e0736e", []byte("keykeykeykeykeykeykeykeykeykeyke"))
	require.NoError(t, err)
	assert.Equal(t, "text", text)
}
