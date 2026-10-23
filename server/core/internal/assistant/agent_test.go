package assistant

import (
	"context"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/assistant/middleware"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Manager_newAgent(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		OmitSummary        bool
		LoadToolsOnRequest bool
		Err                error
	}{
		"Agent is built": {},
		"Agent is built with tools loaded on request": {
			LoadToolsOnRequest: true,
		},
		"Summarization needs a model": {
			// the compaction middlewares are built from the summary
			// model, so the agent cannot exist without one.
			OmitSummary: true,
			Err:         assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m, _ := testManagerStores()
			if c.OmitSummary {
				m.summary = nil
			}

			m.loadToolsOnRequest = c.LoadToolsOnRequest

			toolSet := m.ToolSet("org", "user")

			agent, err := m.newAgent(
				context.Background(),
				toolSet,
				middleware.NewObserver(toolSet, nil, nil, nil),
			)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.NotNil(t, agent)
			assert.Equal(t, _agentName, agent.Name(context.Background()))
		})
	}
}
