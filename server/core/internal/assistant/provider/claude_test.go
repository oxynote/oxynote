package provider

import (
	"context"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_newClaude(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Opts        Options
		Err         error
		ErrContains string
	}{
		"Direct with api key": {
			Opts: Options{
				Provider: ProviderClaude,
				Model:    "claude-opus-4-6",
				APIKey:   "k",
			},
		},
		"Bedrock needs no api key": {
			Opts: Options{
				Provider: ProviderClaude,
				Model:    "anthropic.claude-opus-4-6",
				Bedrock: BedrockOptions{
					Enabled:         true,
					Region:          "us-east-1",
					AccessKey:       "ak",
					SecretAccessKey: "sk",
				},
			},
		},
		"Optional tuning": {
			Opts: tunedOptions(ProviderClaude, "claude-opus-4-6"),
		},
		"Vertex without a project id": {
			Opts: Options{
				Provider: ProviderClaude,
				Model:    "claude-opus-4-6",
				Vertex:   VertexOptions{Enabled: true, Region: "us-east5"},
			},
			Err:         assert.AnError,
			ErrContains: "project ID",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			cm, err := newClaude(context.Background(), c.Opts)

			testutil.RequireEqualError(t, c.Err, err)

			if c.Err != nil {
				assert.Contains(t, err.Error(), c.ErrContains)
				assert.Nil(t, cm)

				return
			}

			require.NotNil(t, cm)
		})
	}
}
