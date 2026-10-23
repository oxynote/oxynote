package cryptoutil

import (
	"crypto/aes"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_ValidateKey(t *testing.T) {
	cc := map[string]struct {
		Key string
		Err error
	}{
		"Empty key": {
			Key: "",
			Err: ErrInvalidKeySize,
		},
		"Key of 16 bytes": {
			Key: "keykeykeykeykeyk",
			Err: ErrInvalidKeySize,
		},
		"Key of 33 bytes": {
			Key: "keykeykeykeykeykeykeykeykeykeykey",
			Err: ErrInvalidKeySize,
		},
		"Key of 32 bytes": {
			Key: "keykeykeykeykeykeykeykeykeykeyke",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, ValidateKey(c.Key))
		})
	}
}

func Test_EncryptText(t *testing.T) {
	cc := map[string]struct {
		Key []byte
		Err error
	}{
		"Invalid key length": {
			Key: []byte("key"),
			Err: aes.KeySizeError(3),
		},
		"Successful encryption": {
			Key: []byte("keykeykeykeykeykeykeykeykeykeyke"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			encryptedText, err := EncryptText("text", c.Key)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Empty(t, encryptedText)

				return
			}

			assert.NotEmpty(t, encryptedText)
		})
	}
}

func Test_DecryptText(t *testing.T) {
	cc := map[string]struct {
		EncryptedText string
		Key           []byte
		Result        string
		Err           error
	}{
		"Invalid hex": {
			EncryptedText: "3",
			Key:           []byte("key"),
			Err:           hex.ErrLength,
		},
		"Invalid key length": {
			EncryptedText: "36e00b9632148fc4793599ebfe5e110f30fa446bd431b1379708217c20e0736e",
			Key:           []byte("key"),
			Err:           aes.KeySizeError(3),
		},
		"Invalid encrypted text": {
			EncryptedText: "36e00b9632148fc4793599ebfe5e110f30fa446bd431b1379708217c20ee",
			Key:           []byte("keykeykeykeykeykeykeykeykeykeyke"),
			Err:           errors.New("cipher: message authentication failed"),
		},
		"Invalid encrypted text length": {
			EncryptedText: "36",
			Key:           []byte("keykeykeykeykeykeykeykeykeykeyke"),
			Err:           errors.New("ciphertext too short"),
		},
		"Successful decryption": {
			EncryptedText: "36e00b9632148fc4793599ebfe5e110f30fa446bd431b1379708217c20e0736e",
			Key:           []byte("keykeykeykeykeykeykeykeykeykeyke"),
			Result:        "text",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			text, err := DecryptText(c.EncryptedText, c.Key)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Empty(t, text)

				return
			}

			assert.Equal(t, c.Result, text)
		})
	}
}
