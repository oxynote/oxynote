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
// read_block invites the model to reach for the wrong one.
const NameReadToolOutput Name = "read_tool_output"

// _offloadPathKey is the argument naming which offloaded result to
// retrieve. It matches the placeholder the reduction middleware leaves
// in the conversation.
const _offloadPathKey = "file_path"

// readToolOutput hands the model back a result the reduction middleware
// moved out of the conversation.
type readToolOutput struct{}

// Info returns the tool's model-facing description.
func (readToolOutput) Info() Info {
	return Info{
		Name: NameReadToolOutput,
		Description: "Retrieve the full output of an earlier tool call that was too large to keep in the conversation. " +
			"Pass the path shown in the truncation notice.",
		Properties: map[string]any{
			_offloadPathKey: stringProp("The stored output path from the truncation notice."),
		},
		Required: []string{_offloadPathKey},
	}
}

// Traits reports an internal read: the paths this takes are minted by
// the reduction middleware during a chat turn, so a client holding none
// of that state would be offered a tool it can never call.
func (readToolOutput) Traits() Traits {
	return Traits{Internal: true}
}

// Title returns no status line: fetching back an offloaded result is
// bookkeeping, not something the user asked for.
func (readToolOutput) Title(_ DescribeInput) string {
	return ""
}

// Execute returns the stored output.
func (readToolOutput) Execute(inp Input) (string, error) {
	var in struct {
		FilePath string `json:"file_path"`
	}

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if in.FilePath == "" {
		return "", fmt.Errorf("%s: %s is required", NameReadToolOutput, _offloadPathKey)
	}

	return inp.ReadOffloaded(in.FilePath)
}

// einoTool adapts one of this package's tools to the interface the
// agent framework invokes.
type einoTool struct {
	// tl is the tool being adapted.
	tl ReadTool

	// deps is the session wiring each call's Input is built from.
	deps *Deps

	// info is the tool's description, resolved once because it never
	// varies by call.
	info Info
}

// newEinoTool creates a fresh instance of einoTool.
func newEinoTool(tl ReadTool, deps *Deps) *einoTool {
	return &einoTool{tl: tl, deps: deps, info: tl.Info()}
}

// Info describes the tool to the model, converting this package's
// description into the framework's shape.
func (et *einoTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return toolInfo(et.info)
}

// InvokableRun performs the call, handing the tool an Input built from
// this call's context and arguments.
func (et *einoTool) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	return et.tl.Execute(et.input(ctx, argumentsInJSON))
}

// Title returns the status line shown while the tool runs.
func (et *einoTool) Title(ctx context.Context, args json.RawMessage) string {
	return et.tl.Title(et.input(ctx, string(args)))
}

// Confirm describes the pending write for the user. It is only reached
// for a tool the registry gated, which is only ever a WriteTool.
func (et *einoTool) Confirm(ctx context.Context, args json.RawMessage) ConfirmActionSummary {
	w, ok := et.tl.(WriteTool)
	if !ok {
		// NOCOV: the gate is only applied to tools implementing WriteTool.
		return ConfirmActionSummary{Tool: string(et.info.Name)}
	}

	return w.Confirm(et.input(ctx, string(args)))
}

// input assembles the per-call input the tool is handed.
func (et *einoTool) input(ctx context.Context, args string) *input {
	return et.deps.newInput(ctx, et.info.Name, json.RawMessage(args))
}

// toolInfo converts a tool's description into the framework's shape.
//
// The property map is transported through JSON because it is already a
// JSON-schema fragment, which keeps each tool's schema the single
// source of truth rather than restating it in a second vocabulary.
func toolInfo(info Info) (*schema.ToolInfo, error) {
	// an empty slice would emit "required": [] where the schema should
	// simply not say anything.
	required := info.Required
	if len(required) == 0 {
		required = nil
	}

	raw, err := json.Marshal(map[string]any{
		_keyType:     _typeObject,
		"properties": info.Properties,
		"required":   required,
	})
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
