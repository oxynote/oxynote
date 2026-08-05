package slackapp

import (
	"errors"
	"testing"
)

func Test_Manager_CreateLinkURL(t *testing.T) {
	t.Parallel()

	_, err := newDisabledManager(t).CreateLinkURL("user", "team", "org")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func Test_Manager_VerifyLinkState(t *testing.T) {
	t.Parallel()

	_, err := newDisabledManager(t).VerifyLinkState("state")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}
