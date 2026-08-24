package provider

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Status_Active(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Status Status
		Result bool
	}{
		"Active runs":             {Status: StatusActive, Result: true},
		"Active but weak runs":    {Status: StatusActiveButWeak, Result: true},
		"Too weak does not run":   {Status: StatusInactiveTooWeak},
		"Inactive does not run":   {Status: StatusInactive},
		"Zero value does not run": {},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Status.Active())
		})
	}
}

func Test_Options_ModelStatus(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Opts   Options
		Result Status
		Err    error
	}{
		"Invalid options": {
			Opts: Options{Provider: ProviderClaude, Model: "claude-opus-5"},
			Err:  ErrMissingAPIKey,
		},
		"Unsupported model": {
			Opts:   Options{Provider: ProviderClaude, Model: "claude-2", APIKey: "k"},
			Result: StatusInactive,
		},
		"Full strength model": {
			Opts:   Options{Provider: ProviderClaude, Model: "claude-opus-5", APIKey: "k"},
			Result: StatusActive,
		},
		"Weaker model": {
			Opts:   Options{Provider: ProviderClaude, Model: "claude-sonnet-5", APIKey: "k"},
			Result: StatusActiveButWeak,
		},
		"Too weak model": {
			Opts:   Options{Provider: ProviderClaude, Model: "claude-haiku-4-5", APIKey: "k"},
			Result: StatusInactiveTooWeak,
		},
		"Supported ollama model": {
			Opts: Options{
				Provider: ProviderOllama,
				Model:    "llama3.3:70b",
				BaseURL:  "http://localhost:11434",
			},
			Result: StatusActiveButWeak,
		},
		"Supported openrouter model": {
			Opts:   Options{Provider: ProviderOpenRouter, Model: "anthropic/claude-opus-5", APIKey: "k"},
			Result: StatusActive,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			status, err := c.Opts.ModelStatus()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, status)
		})
	}
}

func Test_Provider_DefaultModel(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Provider Provider
		Result   string
	}{
		"Claude":           {Provider: ProviderClaude, Result: "claude-opus-5"},
		"OpenAI":           {Provider: ProviderOpenAI, Result: "gpt-5.1"},
		"Gemini":           {Provider: ProviderGemini, Result: "gemini-2.5-pro"},
		"Ollama":           {Provider: ProviderOllama, Result: "llama3.3:70b"},
		"OpenRouter":       {Provider: ProviderOpenRouter, Result: "anthropic/claude-opus-5"},
		"Unknown provider": {Provider: "cohere"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			result := c.Provider.DefaultModel()
			assert.Equal(t, c.Result, result)

			if result == "" {
				return
			}

			// a default that is not on its provider's supported list
			// would disable the assistant instead of enabling it.
			assert.NotEqual(t, StatusInactive, _models[c.Provider].status(result))
		})
	}
}

func Test_parseModels(t *testing.T) {
	t.Parallel()

	// error
	models, err := parseModels([]byte("{"))
	require.Error(t, err)
	assert.Nil(t, models)

	// success: parse the embedded file itself, pinning that every
	// supported provider has an entry, every entry belongs to a
	// supported provider, each default is on its own list, every group
	// carries a status a listed model can have, and no model id appears
	// in two groups — map iteration would make such a duplicate resolve
	// to a random status.
	models, err = parseModels(_modelsJSON)
	require.NoError(t, err)

	for _, p := range _providers {
		require.Contains(t, models, p)
	}

	for p, ml := range models {
		require.NoError(t, p.Validate())
		assert.NotEqual(t, StatusInactive, ml.status(ml.Default), p)

		seen := make(map[string]bool)

		for status, ids := range ml.Models {
			assert.Contains(
				t,
				[]Status{StatusActive, StatusActiveButWeak, StatusInactiveTooWeak},
				status,
				string(p),
			)

			for _, id := range ids {
				assert.False(t, seen[id], "%s/%s listed twice", string(p), id)

				seen[id] = true
			}
		}
	}
}

func Test_modelList_status(t *testing.T) {
	t.Parallel()

	ml := modelList{
		Models: map[Status][]string{
			StatusActive:          {"strong"},
			StatusActiveButWeak:   {"medium"},
			StatusInactiveTooWeak: {"small"},
		},
	}

	cc := map[string]struct {
		Model  string
		Result Status
	}{
		"Full strength model": {Model: "strong", Result: StatusActive},
		"Weaker model":        {Model: "medium", Result: StatusActiveButWeak},
		"Too weak model":      {Model: "small", Result: StatusInactiveTooWeak},
		"Unlisted model":      {Model: "unknown", Result: StatusInactive},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, ml.status(c.Model))
		})
	}
}
