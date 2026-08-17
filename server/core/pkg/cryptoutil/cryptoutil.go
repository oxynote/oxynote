// Package cryptoutil implements various functions for cryptographic operations.
package cryptoutil

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
)

// KeySize is the key length EncryptText and DecryptText expect, in bytes.
// AES itself also accepts 16 and 24, but pinning one size keeps every caller
// on AES-256 instead of silently dropping to a weaker cipher.
const KeySize = 32

// ErrInvalidKeySize is returned when a key is not KeySize bytes long.
var ErrInvalidKeySize = errors.New("key must be 32 bytes long")

// ValidateKey checks whether the key can be used for encryption. Callers
// validate their configured keys at boot, so a misconfigured deployment
// fails to start rather than failing every encrypt call at runtime.
func ValidateKey(key string) error {
	if len(key) != KeySize {
		return ErrInvalidKeySize
	}

	return nil
}

// EncryptText encrypts the text using the key.
// Key must be 32 bytes long.
// The result is a hex encoded string.
// Inspired by: https://tutorialedge.net/golang/go-encrypt-decrypt-aes-tutorial/
func EncryptText(text string, key []byte) (string, error) {
	// Generate a new aes cipher using our 32 byte long key.
	c, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	// GCM or Galois/Counter Mode, is a mode of operation
	// for symmetric key cryptographic block ciphers
	// https://en.wikipedia.org/wiki/Galois/Counter_Mode.
	gcm, err := cipher.NewGCM(c)
	if err != nil {
		// NOCOV: cipher.NewGCM cannot fail with a valid AES block.
		return "", err
	}

	// Creates a new byte array the size of the nonce
	// which must be passed to Seal.
	nonce := make([]byte, gcm.NonceSize())

	// Populates our nonce with a cryptographically secure
	// random sequence.
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		// NOCOV: crypto/rand failures cannot be simulated in tests.
		return "", err
	}

	// Here we encrypt our text using the Seal function
	// Seal encrypts and authenticates plaintext, authenticates the
	// additional data and appends the result to dst, returning the updated
	// slice. The nonce must be NonceSize() bytes long and unique for all
	// time, for a given key. Finally, we encode the resulting byte array
	// to a string using hex encoding.
	return hex.EncodeToString(
		gcm.Seal(
			nonce,
			nonce,
			[]byte(text),
			nil,
		),
	), nil
}

// DecryptText decrypts the text using the key.
// Key must be 32 bytes long.
// Inspired by: https://tutorialedge.net/golang/go-encrypt-decrypt-aes-tutorial/
func DecryptText(encryptedText string, key []byte) (string, error) {
	ciphertext, err := hex.DecodeString(encryptedText)
	if err != nil {
		return "", err
	}

	c, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		// NOCOV: cipher.NewGCM cannot fail with a valid AES block.
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
