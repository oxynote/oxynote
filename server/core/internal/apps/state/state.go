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

// Encode marshals the state, binds it to the given purpose and encrypts it
// with the given secret, producing the opaque token handed to the third
// party. The purpose names the flow the token is minted for, so flows
// sharing a signing secret cannot accept each other's tokens.
func Encode[T Stamped](v T, purpose, secret string) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshaling state: %w", err)
	}

	wrapped, err := json.Marshal(envelope{
		Purpose: purpose,
		State:   data,
	})
	if err != nil {
		// NOCOV: error case cannot happen since the payload is
		// already marshaled.
		return "", fmt.Errorf("marshaling state envelope: %w", err)
	}

	token, err := cryptoutil.EncryptText(string(wrapped), []byte(secret))
	if err != nil {
		return "", fmt.Errorf("encrypting state: %w", err)
	}

	return token, nil
}

// Decode decrypts the token the third party handed back, decodes it and
// rejects anything issued more than ttl ago. A token minted for another
// purpose is as invalid as a tampered one — decoding it as this type would
// let one flow's token replay in another. All failures are reported with
// this package's sentinels, since every app names them in its own namespace.
func Decode[T Stamped](token, purpose, secret string, ttl time.Duration) (T, error) {
	var v T

	decrypted, err := cryptoutil.DecryptText(token, []byte(secret))
	if err != nil {
		return v, ErrInvalid
	}

	var env envelope

	if err := json.Unmarshal([]byte(decrypted), &env); err != nil {
		return v, ErrInvalid
	}

	if env.Purpose != purpose {
		return v, ErrInvalid
	}

	if err := json.Unmarshal(env.State, &v); err != nil {
		return v, ErrInvalid
	}

	if time.Since(v.Created()) > ttl {
		return v, ErrExpired
	}

	return v, nil
}

// envelope wraps a state payload with the purpose it was minted for.
type envelope struct {
	// Purpose names the flow the token was issued for.
	Purpose string `json:"purpose"`

	// State is the flow's own payload.
	State json.RawMessage `json:"state"`
}

// Stamped is implemented by every state payload, so Decode can enforce the
// time-to-live without knowing what the payload carries.
type Stamped interface {
	// Created should report when the state was issued.
	Created() time.Time
}
