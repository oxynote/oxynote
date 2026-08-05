package githubapp

import (
	"context"
	"errors"
	"testing"
)

func Test_NewManager(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		Opt            Options
		WantErr        bool
		WantConfigured bool
	}{
		"Zero app ID creates an unconfigured manager": {
			Opt:            Options{},
			WantErr:        false,
			WantConfigured: false,
		},
		"App ID with missing private key fails": {
			Opt: Options{
				AppID:          123,
				PrivateKeyPath: "testdata/missing.pem",
			},
			WantErr: true,
		},
		"App ID with valid private key creates a configured manager": {
			Opt: Options{
				AppID:          123,
				PrivateKeyPath: "testdata/test-key.pem",
			},
			WantErr:        false,
			WantConfigured: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			man, err := NewManager(nil, tc.Opt)

			if tc.WantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got := man.Configured(); got != tc.WantConfigured {
				t.Fatalf("Configured() = %v, want %v", got, tc.WantConfigured)
			}
		})
	}
}

func Test_Manager_HasInstallationClient(t *testing.T) {
	t.Parallel()

	man, err := NewManager(nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = man.HasInstallationClient(context.Background(), "org")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func Test_Manager_GetInstallationClient(t *testing.T) {
	t.Parallel()

	man, err := NewManager(nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = man.GetInstallationClient(context.Background(), "org")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}
