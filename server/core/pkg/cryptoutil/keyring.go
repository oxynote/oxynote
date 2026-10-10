package cryptoutil

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	// _keyringVersion is the version byte every Keyring ciphertext starts
	// with. A later layout bumps it, so rows written by both can coexist.
	_keyringVersion byte = 1

	// _keyIDSize is the length of the key id a ciphertext carries.
	_keyIDSize = 8

	// _keyringHeaderSize is the length of the version byte and key id
	// that precede the nonce.
	_keyringHeaderSize = 1 + _keyIDSize
)

var (
	// ErrEmptyKeyring is returned when the keyring list holds no key.
	ErrEmptyKeyring = errors.New("keyring holds no keys")

	// ErrDuplicateKey is returned when the keyring lists a key twice.
	ErrDuplicateKey = errors.New("keyring lists the same key twice")

	// ErrUnknownVersion is returned when a ciphertext starts with a
	// version byte this keyring does not read.
	ErrUnknownVersion = errors.New("ciphertext version is not supported")

	// ErrUnknownKey is returned when a ciphertext names a key the keyring
	// does not hold.
	ErrUnknownKey = errors.New("ciphertext was sealed with a key the keyring does not hold")

	// ErrCiphertextTooShort is returned when a ciphertext cannot even hold
	// its header and nonce.
	ErrCiphertextTooShort = errors.New("ciphertext too short")
)

// keyringKey is one key of a Keyring, identified by the first bytes of its
// SHA-256 so operators never have to name keys.
type keyringKey struct {
	id   [_keyIDSize]byte
	aead cipher.AEAD
}

// Keyring holds the AES-256-GCM keys data is sealed and opened with. The
// first key seals; every key opens what it sealed. Rotation is prepending a
// key, re-encrypting what the old one sealed, and dropping the old one.
type Keyring struct {
	keys []keyringKey
	byID map[[_keyIDSize]byte]cipher.AEAD
}

// ParseKeyring parses a comma-separated list of keys, newest first, each
// the standard base64 encoding of exactly KeySize random bytes.
func ParseKeyring(list string) (*Keyring, error) {
	if strings.TrimSpace(list) == "" {
		return nil, ErrEmptyKeyring
	}

	kr := &Keyring{
		byID: make(map[[_keyIDSize]byte]cipher.AEAD),
	}

	for i, entry := range strings.Split(list, ",") {
		key, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(entry))
		if err != nil {
			return nil, fmt.Errorf("key %d: %w", i+1, err)
		}

		if len(key) != KeySize {
			return nil, fmt.Errorf("key %d: %w", i+1, ErrInvalidKeySize)
		}

		kk, err := newKeyringKey(key)
		if err != nil {
			// NOCOV: aes.NewCipher and cipher.NewGCM cannot fail with a
			// key of KeySize bytes.
			return nil, fmt.Errorf("key %d: %w", i+1, err)
		}

		if _, ok := kr.byID[kk.id]; ok {
			return nil, fmt.Errorf("key %d: %w", i+1, ErrDuplicateKey)
		}

		kr.keys = append(kr.keys, kk)
		kr.byID[kk.id] = kk.aead
	}

	return kr, nil
}

// Encrypt seals the plaintext under the newest key, bound to the associated
// data, which Decrypt must be handed again. The result is the version byte,
// the key id, the nonce and the sealed bytes.
func (k *Keyring) Encrypt(plaintext, aad []byte) ([]byte, error) {
	kk := k.keys[0]
	nonceSize := kk.aead.NonceSize()

	out := make([]byte, _keyringHeaderSize+nonceSize, _keyringHeaderSize+nonceSize+len(plaintext)+kk.aead.Overhead())
	out[0] = _keyringVersion
	copy(out[1:], kk.id[:])

	if _, err := io.ReadFull(rand.Reader, out[_keyringHeaderSize:]); err != nil {
		// NOCOV: crypto/rand failures cannot be simulated in tests.
		return nil, err
	}

	return kk.aead.Seal(out, out[_keyringHeaderSize:], plaintext, aad), nil //nolint:gosec // the nonce is read from crypto/rand above
}

// Decrypt opens a ciphertext Encrypt produced under any key the keyring
// holds, given the same associated data.
func (k *Keyring) Decrypt(ciphertext, aad []byte) ([]byte, error) {
	if len(ciphertext) < _keyringHeaderSize {
		return nil, ErrCiphertextTooShort
	}

	if ciphertext[0] != _keyringVersion {
		return nil, ErrUnknownVersion
	}

	aead, ok := k.byID[[_keyIDSize]byte(ciphertext[1:_keyringHeaderSize])]
	if !ok {
		return nil, ErrUnknownKey
	}

	rest := ciphertext[_keyringHeaderSize:]
	if len(rest) < aead.NonceSize() {
		return nil, ErrCiphertextTooShort
	}

	return aead.Open(nil, rest[:aead.NonceSize()], rest[aead.NonceSize():], aad)
}

// IsCurrent reports whether the ciphertext was sealed under the newest key.
// Anything the keyring would not read at all is not current either.
func (k *Keyring) IsCurrent(ciphertext []byte) bool {
	return len(ciphertext) >= _keyringHeaderSize &&
		ciphertext[0] == _keyringVersion &&
		[_keyIDSize]byte(ciphertext[1:_keyringHeaderSize]) == k.keys[0].id
}

// newKeyringKey derives the key's id and prepares its cipher.
func newKeyringKey(key []byte) (keyringKey, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return keyringKey{}, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return keyringKey{}, err
	}

	sum := sha256.Sum256(key)

	return keyringKey{
		id:   [_keyIDSize]byte(sum[:_keyIDSize]),
		aead: aead,
	}, nil
}
