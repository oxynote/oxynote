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
		return "", err
	}

	// Creates a new byte array the size of the nonce
	// which must be passed to Seal.
	nonce := make([]byte, gcm.NonceSize())

	// Populates our nonce with a cryptographically secure
	// random sequence.
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
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
