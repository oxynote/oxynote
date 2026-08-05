package slackapp

import (
	"errors"
	"testing"
)

func Test_Manager_CreateExternalInstallationURL(t *testing.T) {
	t.Parallel()

	_, err := newDisabledManager(t).CreateExternalInstallationURL("org")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func Test_Manager_CreateInternalInstallationURL(t *testing.T) {
	t.Parallel()

	_, err := newDisabledManager(t).CreateInternalInstallationURL("team")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func Test_Manager_VerifyInstallationState(t *testing.T) {
	t.Parallel()

	_, err := newDisabledManager(t).VerifyInstallationState("state")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}
