package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
)

// This file is the whole of the assistant's coupling to eino. Tools
// describe themselves and do their work in this package's own
// vocabulary; einoTool translates that into the interface the agent
// framework calls. Nothing else in the package imports eino.

// NameReadToolOutput retrieves a tool result that was too large to keep
// in the conversation.
//
// It exists because of eino: the reduction middleware offloads an
// oversized result, leaves a notice naming the path it was stored at,
// and tells the model to fetch it with a read tool — which it names but
// does not provide. This is that tool.
//
// It is deliberately not called read_file: the assistant's vocabulary
// is documents and blocks, and a generic file tool sitting next to
// get_document invites the model to reach for the wrong one.
const NameReadToolOutput Name = "read_tool_output"

// readToolOutputArgs is what read_tool_output is called with.
type readToolOutputArgs struct {
	// FilePath is the stored output to retrieve.
	FilePath string `json:"file_path"`
}

// Validate checks the arguments are complete.
func (a readToolOutputArgs) Validate() error {
	if a.FilePath == "" {
		return errRequired("file_path")
	}

	return nil
}

// readToolOutput hands the model back a result the reduction middleware
// moved out of the conversation.
type readToolOutput struct{}

// Info returns the tool's model-facing description.
func (readToolOutput) Info() Info {
	return Info{
		Name:   NameReadToolOutput,
		Traits: Traits{Internal: true},
		Description: "Retrieve the full output of an earlier tool call that was too large to keep in the conversation. " +
			"Pass the path shown in the truncation notice.",
		Properties: map[string]any{
			"file_path": map[string]any{"type": "string", "description": "The stored output path from the truncation notice."},
		},
		Required: []string{"file_path"},
	}
}

// Execute returns the stored output.
func (readToolOutput) Execute(inp *input) (string, error) {
	var in readToolOutputArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	return inp.offload.Read(inp.Context(), in.FilePath)
}

// einoTool adapts one of this package's tools to the interface the
// agent framework invokes.
type einoTool struct {
	// tl is the tool being adapted.
	tl Tool

	// deps is the session wiring each call's input is built from.
	deps *Deps

	// info is the tool's description, resolved once because it never
	// varies by call.
	info Info
}

// newEinoTool creates a fresh instance of einoTool.
func newEinoTool(tl Tool, deps *Deps) *einoTool {
	return &einoTool{tl: tl, deps: deps, info: tl.Info()}
}

// Info describes the tool to the model, converting this package's
// description into the framework's shape.
func (et *einoTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return et.info.toEino()
}

// Run performs the call, handing the tool an input built from this
// call's context and arguments. A failure names the tool.
func (et *einoTool) Run(ctx context.Context, args json.RawMessage) (string, error) {
	out, err := et.tl.Execute(et.input(ctx, string(args)))
	if err != nil {
		return "", fmt.Errorf("%s: %w", et.info.Name, err)
	}

	return out, nil
}

// InvokableRun performs the call for the agent framework, which hands
// its arguments over as a string and may pass options this adapter has
// no use for.
//
// A failed call comes back as the call's result rather than as an
// error. The framework ends the whole turn on a tool error, and most of
// what fails here is the model's own doing — a query the data source
// rejected, a block the schema refuses, an id that names nothing — which
// it can fix if it is told. Handing the text back lets it try again,
// which is what a person watching the chat expects; ending the turn
// leaves them with nothing.
//
// Only this surface is affected. The confirmation gate wraps this method
// and raises its interrupt before reaching it, so a pending write still
// pauses the turn, and a caller that runs a tool directly — the MCP
// server does — still sees the error itself.
func (et *einoTool) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	out, err := et.Run(ctx, json.RawMessage(argumentsInJSON))
	if err != nil {
		//nolint:nilerr // the failure is the call's result here, not the turn's
		return err.Error(), nil
	}

	return out, nil
}

// Title returns the status line shown while the tool runs: a write's
// summary, a read's own title, or nothing for a read without one or
// arguments that cannot be described.
func (et *einoTool) Title(ctx context.Context, args json.RawMessage) string {
	if et.info.Traits.Write {
		sum, err := et.Summary(ctx, args)
		if err != nil {
			return ""
		}

		return sum.Summary
	}

	if t, ok := et.tl.(titler); ok {
		return t.Title(et.input(ctx, string(args)))
	}

	return ""
}

// Summary describes the pending write for the user. It is only reached
// for a tool the registry gated, which is only ever a write, and every
// write describes itself. A failure names the tool.
func (et *einoTool) Summary(ctx context.Context, args json.RawMessage) (ActionSummary, error) {
	s, ok := et.tl.(summarizer)
	if !ok {
		return ActionSummary{}, fmt.Errorf("%s: proposes no change to describe", et.info.Name)
	}

	sum, err := s.Summary(et.input(ctx, string(args)))
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: %w", et.info.Name, err)
	}

	return sum, nil
}

// input assembles the per-call input the tool is handed.
func (et *einoTool) input(ctx context.Context, args string) *input {
	return et.deps.newInput(ctx, et.info.Name, json.RawMessage(args))
}

// toEino converts the description into the framework's shape, wrapping
// the schema Info already knows how to state. It is a method on Info
// but lives here, so the framework's vocabulary stays in this file
// rather than spreading to tool.go.
func (info Info) toEino() (*schema.ToolInfo, error) {
	raw, err := json.Marshal(info.Schema())
	if err != nil {
		// NOCOV: the property maps are literals of JSON-encodable types.
		return nil, fmt.Errorf("marshalling schema: %w", err)
	}

	js := &jsonschema.Schema{}
	if err := json.Unmarshal(raw, js); err != nil {
		// NOCOV: the payload was just produced by json.Marshal.
		return nil, fmt.Errorf("unmarshalling schema: %w", err)
	}

	return &schema.ToolInfo{
		Name:        string(info.Name),
		Desc:        info.Description,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(js),
	}, nil
}
