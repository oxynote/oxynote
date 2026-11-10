package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
)

// This file is the whole of the assistant's coupling to eino. Tools
// describe themselves and do their work in this package's own
// vocabulary; einoTool translates that into the interface the agent
// framework calls. Nothing else in the package imports eino.

// NameReadToolOutput retrieves a tool result too large to keep in the
// conversation. eino's reduction middleware names it but does not
// provide it.
const NameReadToolOutput Name = "read_tool_output"

// readToolOutput hands the model back a result the reduction middleware
// moved out of the conversation.
type readToolOutput struct {
	// FilePath is the stored output to retrieve.
	FilePath string `json:"file_path"`
}

// Info returns the tool's model-facing description.
func (readToolOutput) Info() Info {
	return Info{
		Name:        NameReadToolOutput,
		Traits:      Traits{Internal: true},
		Description: "Return the full output of an earlier tool call that was too large to keep in the conversation.",
		Properties: map[string]any{
			"file_path": map[string]any{"type": "string", "description": "The path shown in the truncation notice."},
		},
		Required: []string{"file_path"},
	}
}

// Validate checks the arguments are complete.
func (rt readToolOutput) Validate() error {
	if rt.FilePath == "" {
		return errRequired("file_path")
	}

	return nil
}

// Exec returns the stored output.
func (rt readToolOutput) Exec(inp Input) (any, error) {
	return inp.ReadToolOutput(rt.FilePath)
}

// einoTool adapts one of this package's tools to the interface the
// agent framework invokes.
type einoTool struct {
	// reg is the tool being adapted.
	reg registration

	// deps is the session wiring each call's input is built from.
	deps *Deps

	// info is the tool's description, resolved once because it never
	// varies by call.
	info Info
}

// Info describes the tool to the model, converting this package's
// description into the framework's shape.
func (et *einoTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return et.info.toEino()
}

// Run performs the call. A failure names the tool.
func (et *einoTool) Run(ctx context.Context, args json.RawMessage) (string, error) {
	out, err := et.call(ctx, args)
	if err != nil {
		return "", fmt.Errorf("%s: %w", et.info.Name, err)
	}

	return out, nil
}

// call decodes the arguments into the tool, runs it and encodes its
// result. A string result is encoded already, so it is returned as it is.
func (et *einoTool) call(ctx context.Context, args json.RawMessage) (string, error) {
	tl, err := et.reg.decode(args)
	if err != nil {
		return "", err
	}

	res, err := tl.Exec(et.input(ctx))
	if err != nil {
		return "", err
	}

	if s, ok := res.(string); ok {
		return s, nil
	}

	return encodeResult(res)
}

// InvokableRun performs the call for the agent framework. A failure is
// returned as the call's result, since an error would end the turn and
// the model can often fix the call.
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

	tl, err := et.reg.decode(args)
	if err != nil {
		return ""
	}

	if t, ok := tl.(titler); ok {
		return t.Title(et.input(ctx))
	}

	return ""
}

// Summary describes the pending write for the user. It is only reached
// for a tool the registry gated, which is only ever a write, and every
// write describes itself. A failure names the tool.
func (et *einoTool) Summary(ctx context.Context, args json.RawMessage) (ActionSummary, error) {
	tl, err := et.reg.decode(args)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: %w", et.info.Name, err)
	}

	s, ok := tl.(summarizer)
	if !ok {
		return ActionSummary{}, fmt.Errorf("%s: proposes no change to describe", et.info.Name)
	}

	sum, err := s.Summary(et.input(ctx))
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: %w", et.info.Name, err)
	}

	return sum, nil
}

// input assembles the per-call input the tool is handed.
func (et *einoTool) input(ctx context.Context) *input {
	return et.deps.newInput(ctx, et.info.Name)
}

// newEinoTool creates a fresh instance of einoTool.
func newEinoTool(reg registration, deps *Deps) *einoTool {
	return &einoTool{reg: reg, deps: deps, info: reg.info}
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

// encodeResult encodes a tool result as JSON. <, > and & are left as
// they are, since escaped they cost the model six characters each.
func encodeResult(v any) (string, error) {
	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(v); err != nil {
		return "", fmt.Errorf("encoding tool result: %w", err)
	}

	return strings.TrimSuffix(buf.String(), "\n"), nil
}
