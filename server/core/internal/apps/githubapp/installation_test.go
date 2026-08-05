package githubapp

import (
	"errors"
	"testing"
)

func Test_Manager_CreateInstallationURL(t *testing.T) {
	t.Parallel()

	man, err := NewManager(nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = man.CreateInstallationURL("org")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func Test_Manager_VerifyInstallationState(t *testing.T) {
	t.Parallel()

	man, err := NewManager(nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = man.VerifyInstallationState("state")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}
