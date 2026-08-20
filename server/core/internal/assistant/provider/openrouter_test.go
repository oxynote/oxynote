package provider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_newOpenRouter(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Opts Options
	}{
		"Direct with api key": {
			Opts: Options{
				Provider: ProviderOpenRouter,
				Model:    "anthropic/claude-opus-4.6",
				APIKey:   "k",
			},
		},
		"Optional tuning": {
			Opts: tunedOptions(ProviderOpenRouter, "anthropic/claude-opus-4.6"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			cm, err := newOpenRouter(context.Background(), c.Opts)

			require.NoError(t, err)
			require.NotNil(t, cm)
		})
	}
}
