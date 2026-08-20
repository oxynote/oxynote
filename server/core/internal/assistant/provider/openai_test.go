package provider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_newOpenAI(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Opts Options
	}{
		"Direct with api key": {
			Opts: Options{
				Provider: ProviderOpenAI,
				Model:    "gpt-5",
				APIKey:   "k",
			},
		},
		"Compatible endpoint": {
			Opts: Options{
				Provider: ProviderOpenAI,
				Model:    "llama-3.3-70b",
				APIKey:   "k",
				BaseURL:  "https://example.invalid/v1",
			},
		},
		"Optional tuning": {
			Opts: tunedOptions(ProviderOpenAI, "gpt-5"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			cm, err := newOpenAI(context.Background(), c.Opts)

			require.NoError(t, err)
			require.NotNil(t, cm)
		})
	}
}
