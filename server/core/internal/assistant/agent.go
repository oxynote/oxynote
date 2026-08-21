package assistant

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/oxynote/oxynote/server/core/internal/assistant/middleware"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
)

const (
	// _agentName identifies the assistant in agent events and logs.
	_agentName = "rubber-duck"

	// _agentDescription describes the assistant to the framework. It is
	// only surfaced when an agent is used as a tool by another agent,
	// which Oxynote does not do, but the framework asks for it.
	_agentDescription = "Reads and edits documents in the user's organisation."
)

// _maxAgentTurns caps the model/tool cycles in a single turn so a
// runaway model cannot burn budget indefinitely.
const _maxAgentTurns = 50

// newAgent builds the chat agent for one session. Tools are bound to
// the session's organisation and user, so an agent can never reach
// another organisation's documents.
func (m *Manager) newAgent(
	ctx context.Context,
	toolSet *tools.Set,
	obs *middleware.Observer,
) (adk.Agent, error) {
	compaction, err := middleware.NewCompaction(ctx, m.summary, m.offload, toolSet.WriteNames())
	if err != nil {
		return nil, err
	}

	// the observer runs outermost so it sees the conversation as the
	// compaction middlewares left it.
	handlers := append([]adk.ChatModelAgentMiddleware{obs}, compaction...)

	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          _agentName,
		Description:   _agentDescription,
		Model:         m.model,
		MaxIterations: _maxAgentTurns,
		GenModelInput: genModelInput,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: toolSet.Tools(),

				// tools run one at a time, in the order the model asked
				// for them. Reads are order-independent, but an edit may
				// reference a block an earlier edit in the same batch
				// created, and two edits to one document must not race.
				ExecuteSequentially: true,
			},
		},
		Handlers: handlers,
	})
	if err != nil {
		return nil, fmt.Errorf("building assistant agent: %w", err)
	}

	return agent, nil
}
