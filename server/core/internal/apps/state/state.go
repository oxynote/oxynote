// Package state encodes and decodes the short-lived encrypted state
// parameters that carry installation and linking context through a third
// party's redirect and back.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/cryptoutil"
)

var (
	// ErrInvalid is returned when the token cannot be decrypted or decoded,
	// which means it was tampered with or truncated.
	ErrInvalid = errors.New("state is invalid")

	// ErrExpired is returned when the token was issued longer ago than its
	// time-to-live allows.
	ErrExpired = errors.New("state has expired")
)

// Encode marshals the state and encrypts it with the given secret, producing
// the opaque token handed to the third party.
func Encode[T Stamped](v T, secret string) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshaling state: %w", err)
	}

	token, err := cryptoutil.EncryptText(string(data), []byte(secret))
	if err != nil {
		return "", fmt.Errorf("encrypting state: %w", err)
	}

	return token, nil
}

// Decode decrypts the token the third party handed back, decodes it and
// rejects anything issued more than ttl ago. Both failures are reported with
// this package's sentinels, since every app names them in its own namespace.
func Decode[T Stamped](token, secret string, ttl time.Duration) (T, error) {
	var v T

	decrypted, err := cryptoutil.DecryptText(token, []byte(secret))
	if err != nil {
		return v, ErrInvalid
	}

	if err := json.Unmarshal([]byte(decrypted), &v); err != nil {
		return v, ErrInvalid
	}

	if time.Since(v.Created()) > ttl {
		return v, ErrExpired
	}

	return v, nil
}

// Stamped is implemented by every state payload, so Decode can enforce the
// time-to-live without knowing what the payload carries.
type Stamped interface {
	// Created should report when the state was issued.
	Created() time.Time
}
