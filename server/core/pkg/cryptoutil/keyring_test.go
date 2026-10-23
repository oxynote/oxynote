package cryptoutil

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	// _keyA is a base64 test key.
	_keyA = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'a'}, KeySize))

	// _keyB is another base64 test key.
	_keyB = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'b'}, KeySize))
)

// testKeyring parses the given keys or fails the test.
func testKeyring(t *testing.T, keys ...string) *Keyring {
	t.Helper()

	kr, err := ParseKeyring(strings.Join(keys, ","))
	require.NoError(t, err)

	return kr
}

func Test_ParseKeyring(t *testing.T) {
	cc := map[string]struct {
		List   string
		Result [][KeySize]byte
		Err    error
	}{
		"Empty list": {
			List: "",
			Err:  ErrEmptyKeyring,
		},
		"Whitespace only": {
			List: "  ",
			Err:  ErrEmptyKeyring,
		},
		"Invalid base64": {
			List: "not base64!",
			Err:  assert.AnError,
		},
		"Key of 16 bytes": {
			List: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'a'}, 16)),
			Err:  assert.AnError,
		},
		"Key of 33 bytes": {
			List: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'a'}, 33)),
			Err:  assert.AnError,
		},
		"Duplicate key": {
			List: _keyA + "," + _keyB + "," + _keyA,
			Err:  assert.AnError,
		},
		"Trailing comma": {
			List: _keyA + ",",
			Err:  assert.AnError,
		},
		"One key": {
			List:   _keyA,
			Result: [][KeySize]byte{[KeySize]byte(bytes.Repeat([]byte{'a'}, KeySize))},
		},
		"Two keys keep their order": {
			List: " " + _keyB + " , " + _keyA,
			Result: [][KeySize]byte{
				[KeySize]byte(bytes.Repeat([]byte{'b'}, KeySize)),
				[KeySize]byte(bytes.Repeat([]byte{'a'}, KeySize)),
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			kr, err := ParseKeyring(c.List)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, kr)

				return
			}

			require.NotNil(t, kr)
			require.Len(t, kr.keys, len(c.Result))
			assert.Len(t, kr.byID, len(c.Result))

			for i, key := range c.Result {
				sum := sha256.Sum256(key[:])
				assert.Equal(t, [_keyIDSize]byte(sum[:_keyIDSize]), kr.keys[i].id)
				assert.Equal(t, kr.keys[i].aead, kr.byID[kr.keys[i].id])
			}
		})
	}
}

func Test_Keyring_Encrypt(t *testing.T) {
	kr := testKeyring(t, _keyA, _keyB)

	out, err := kr.Encrypt([]byte("text"), []byte("aad"))
	require.NoError(t, err)

	sum := sha256.Sum256(bytes.Repeat([]byte{'a'}, KeySize))
	nonceSize := kr.keys[0].aead.NonceSize()

	assert.Equal(t, _keyringVersion, out[0])
	assert.Equal(t, sum[:_keyIDSize], out[1:_keyringHeaderSize])
	assert.Len(t, out, _keyringHeaderSize+nonceSize+len("text")+kr.keys[0].aead.Overhead())

	// a second call draws a fresh nonce.
	again, err := kr.Encrypt([]byte("text"), []byte("aad"))
	require.NoError(t, err)
	assert.NotEqual(t, out[_keyringHeaderSize:_keyringHeaderSize+nonceSize], again[_keyringHeaderSize:_keyringHeaderSize+nonceSize])
}

func Test_Keyring_Decrypt(t *testing.T) {
	sealed := func(t *testing.T, kr *Keyring) []byte {
		t.Helper()

		out, err := kr.Encrypt([]byte("text"), []byte("aad"))
		require.NoError(t, err)

		return out
	}

	cc := map[string]struct {
		Keyring    *Keyring
		Ciphertext []byte
		AAD        []byte
		Result     []byte
		Err        error
	}{
		"Too short for the header": {
			Keyring:    testKeyring(t, _keyA),
			Ciphertext: []byte{_keyringVersion, 1, 2},
			AAD:        []byte("aad"),
			Err:        ErrCiphertextTooShort,
		},
		"Too short for the nonce": {
			Keyring:    testKeyring(t, _keyA),
			Ciphertext: sealed(t, testKeyring(t, _keyA))[:_keyringHeaderSize+3],
			AAD:        []byte("aad"),
			Err:        ErrCiphertextTooShort,
		},
		"Unknown version": {
			Keyring: testKeyring(t, _keyA),
			Ciphertext: func() []byte {
				out := sealed(t, testKeyring(t, _keyA))
				out[0] = 2

				return out
			}(),
			AAD: []byte("aad"),
			Err: ErrUnknownVersion,
		},
		"Unknown key": {
			Keyring:    testKeyring(t, _keyA),
			Ciphertext: sealed(t, testKeyring(t, _keyB)),
			AAD:        []byte("aad"),
			Err:        ErrUnknownKey,
		},
		"Associated data mismatch": {
			Keyring:    testKeyring(t, _keyA),
			Ciphertext: sealed(t, testKeyring(t, _keyA)),
			AAD:        []byte("other"),
			Err:        assert.AnError,
		},
		"Tampered ciphertext": {
			Keyring: testKeyring(t, _keyA),
			Ciphertext: func() []byte {
				out := sealed(t, testKeyring(t, _keyA))
				out[len(out)-1] ^= 1

				return out
			}(),
			AAD: []byte("aad"),
			Err: assert.AnError,
		},
		"Sealed under a retired key": {
			Keyring:    testKeyring(t, _keyB, _keyA),
			Ciphertext: sealed(t, testKeyring(t, _keyA)),
			AAD:        []byte("aad"),
			Result:     []byte("text"),
		},
		"Sealed under the newest key": {
			Keyring:    testKeyring(t, _keyA, _keyB),
			Ciphertext: sealed(t, testKeyring(t, _keyA)),
			AAD:        []byte("aad"),
			Result:     []byte("text"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.Keyring.Decrypt(c.Ciphertext, c.AAD)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, res)

				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_Keyring_IsCurrent(t *testing.T) {
	kr := testKeyring(t, _keyA, _keyB)

	out, err := kr.Encrypt([]byte("text"), nil)
	require.NoError(t, err)

	retired, err := testKeyring(t, _keyB).Encrypt([]byte("text"), nil)
	require.NoError(t, err)

	cc := map[string]struct {
		Ciphertext []byte
		Result     bool
	}{
		"Too short": {
			Ciphertext: out[:_keyringHeaderSize-1],
		},
		"Unknown version": {
			Ciphertext: func() []byte {
				other := bytes.Clone(out)
				other[0] = 2

				return other
			}(),
		},
		"Retired key": {
			Ciphertext: retired,
		},
		"Newest key": {
			Ciphertext: out,
			Result:     true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, kr.IsCurrent(c.Ciphertext))
		})
	}
}

func Test_newKeyringKey(t *testing.T) {
	cc := map[string]struct {
		Key []byte
		Err error
	}{
		"Error returned by aes.NewCipher": {
			Key: []byte("key"),
			Err: aes.KeySizeError(3),
		},
		"Successful creation": {
			Key: bytes.Repeat([]byte{'a'}, KeySize),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			kk, err := newKeyringKey(c.Key)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Equal(t, keyringKey{}, kk)

				return
			}

			block, err := aes.NewCipher(c.Key)
			require.NoError(t, err)

			aead, err := cipher.NewGCM(block)
			require.NoError(t, err)

			sum := sha256.Sum256(c.Key)
			assert.Equal(t, [_keyIDSize]byte(sum[:_keyIDSize]), kk.id)
			assert.Equal(t, aead, kk.aead)
		})
	}
}
