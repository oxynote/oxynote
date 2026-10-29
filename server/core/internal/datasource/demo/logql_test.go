package demo

import (
	"regexp"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _deployLine is a line the LogQL tests filter.
var _deployLine = processor.LogLine{
	Line:   `deploy rolled back sha=3f9a1c2: health check failed`,
	Labels: map[string]string{"service_name": "deploy-bot", "env": "production"},
	Fields: map[string]string{"detected_level": "error", "outcome": "rolled_back"},
}

func Test_logQuery_matchesStream(t *testing.T) {
	cc := map[string]struct {
		Query  string
		Result bool
	}{
		"Equal":                  {Query: `{service_name="deploy-bot"}`, Result: true},
		"Not equal":              {Query: `{env="production", service_name!="deploy-bot"}`, Result: false},
		"Regexp":                 {Query: `{service_name=~"deploy-.*", env="production"}`, Result: true},
		"Label the stream lacks": {Query: `{team="core"}`, Result: false},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			lq, err := _client.parseLogQuery(c.Query)
			require.NoError(t, err)
			assert.Equal(t, c.Result, lq.matchesStream(_deployLine.Labels))
		})
	}
}

func Test_logQuery_matches(t *testing.T) {
	cc := map[string]struct {
		Query  string
		Result bool
	}{
		"No pipeline":          {Query: `{env="production"}`, Result: true},
		"Every stage keeps it": {Query: `{env="production"} |= "rolled back" | detected_level="error"`, Result: true},
		"One stage drops it":   {Query: `{env="production"} |= "rolled back" | detected_level="info"`, Result: false},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			lq, err := _client.parseLogQuery(c.Query)
			require.NoError(t, err)
			assert.Equal(t, c.Result, lq.matches(_deployLine))
		})
	}
}

func Test_logFilter_matches(t *testing.T) {
	matcher := labels.MustNewMatcher

	cc := map[string]struct {
		Filter logFilter
		Result bool
	}{
		"Contains":                     {Filter: logFilter{op: "|=", value: "health check"}, Result: true},
		"Contains is case-sensitive":   {Filter: logFilter{op: "|=", value: "Health"}, Result: false},
		"Does not contain":             {Filter: logFilter{op: "!=", value: "health check"}, Result: false},
		"Regexp is unanchored":         {Filter: logFilter{op: "|~", re: regexp.MustCompile(`sha=[0-9a-f]+`)}, Result: true},
		"Negated regexp":               {Filter: logFilter{op: "!~", re: regexp.MustCompile(`sha=[0-9a-f]+`)}, Result: false},
		"Label of the stream":          {Filter: logFilter{matcher: matcher(labels.MatchEqual, "env", "production")}, Result: true},
		"Structured metadata":          {Filter: logFilter{matcher: matcher(labels.MatchRegexp, "outcome", "rolled_.*")}, Result: true},
		"Regexp label filter anchored": {Filter: logFilter{matcher: matcher(labels.MatchRegexp, "outcome", "rolled")}, Result: false},
		"Missing field reads empty":    {Filter: logFilter{matcher: matcher(labels.MatchEqual, "team", "")}, Result: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Filter.matches(_deployLine))
		})
	}
}

func Test_Client_parseLogQuery(t *testing.T) {
	cc := map[string]struct {
		Query     string
		Selectors int
		Filters   int
		Err       error
	}{
		"Selector only":            {Query: ` {service_name="deploy-bot"} `, Selectors: 1},
		"Brace inside a value":     {Query: `{service_name=~"a{2}"}`, Selectors: 1},
		"Every supported stage":    {Query: "{env=\"production\"} |= \"a\" != `b` |~ \"c+\" !~ \"d\" | outcome=\"rolled_back\" | user!=\"ada\"", Selectors: 1, Filters: 6},
		"Metric query":             {Query: `rate({env="production"}[5m])`, Err: _errUnsupportedLogQuery},
		"Empty query":              {Query: ``, Err: _errUnsupportedLogQuery},
		"Empty selector":           {Query: `{}`, Err: _errEmptyCompatibleSelector},
		"Empty-compatible matcher": {Query: `{env=~".*"}`, Err: _errEmptyCompatibleSelector},
		"Invalid selector":         {Query: `{env=}`, Err: assert.AnError},
		"Unterminated selector":    {Query: `{env="production"`, Err: processor.NewInvalidQueryError("unterminated stream selector")},
		"Parser stage":             {Query: `{env="production"} | json`, Err: _errUnsupportedLogQuery},
		"Invalid regexp":           {Query: `{env="production"} |~ "("`, Err: assert.AnError},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			lq, err := _client.parseLogQuery(c.Query)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Len(t, lq.selector, c.Selectors)
			assert.Len(t, lq.filters, c.Filters)
		})
	}
}

func Test_parseLogFilters(t *testing.T) {
	cc := map[string]struct {
		Pipeline string
		Filters  int
		Err      error
	}{
		"Empty pipeline":          {Pipeline: "  ", Filters: 0},
		"Line then label filter":  {Pipeline: ` |= "a" | b="c"`, Filters: 2},
		"Error in a line filter":  {Pipeline: ` |= a`, Err: _errUnsupportedLogQuery},
		"Error in a label filter": {Pipeline: ` | json`, Err: _errUnsupportedLogQuery},
		"Unknown stage":           {Pipeline: ` or "b"`, Err: _errUnsupportedLogQuery},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			filters, err := parseLogFilters(c.Pipeline)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Len(t, filters, c.Filters)
		})
	}
}

func Test_lineFilterOp(t *testing.T) {
	cc := map[string]struct {
		Input string
		Op    string
		Ok    bool
	}{
		"Contains":         {Input: `|= "a"`, Op: "|=", Ok: true},
		"Does not contain": {Input: `!= "a"`, Op: "!=", Ok: true},
		"Matches":          {Input: `|~ "a"`, Op: "|~", Ok: true},
		"Does not match":   {Input: `!~ "a"`, Op: "!~", Ok: true},
		"Label filter":     {Input: `| a="b"`, Ok: false},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			op, ok := lineFilterOp(c.Input)
			assert.Equal(t, c.Op, op)
			assert.Equal(t, c.Ok, ok)
		})
	}
}

func Test_parseLineFilter(t *testing.T) {
	cc := map[string]struct {
		Op    string
		Input string
		Value string
		Rest  string
		Regex bool
		Err   error
	}{
		"Substring":      {Op: "|=", Input: `"rolled back" | a="b"`, Value: "rolled back", Rest: ` | a="b"`},
		"Regexp":         {Op: "|~", Input: "`sha=\\w+`", Value: `sha=\w+`, Regex: true},
		"Not a string":   {Op: "|=", Input: `rolled`, Err: _errUnsupportedLogQuery},
		"Invalid regexp": {Op: "!~", Input: `"["`, Err: assert.AnError},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			f, rest, err := parseLineFilter(c.Op, c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Op, f.op)
			assert.Equal(t, c.Value, f.value)
			assert.Equal(t, c.Regex, f.re != nil)
			assert.Equal(t, c.Rest, rest)
		})
	}
}

func Test_parseLabelFilter(t *testing.T) {
	cc := map[string]struct {
		Input string
		Type  labels.MatchType
		Name  string
		Value string
		Rest  string
		Err   error
	}{
		"Equal":              {Input: `detected_level="error" |= "x"`, Type: labels.MatchEqual, Name: "detected_level", Value: "error", Rest: ` |= "x"`},
		"Double equal":       {Input: `level=="error"`, Type: labels.MatchEqual, Name: "level", Value: "error"},
		"Not equal":          {Input: `level != "error"`, Type: labels.MatchNotEqual, Name: "level", Value: "error"},
		"Regexp":             {Input: `level=~"err.*"`, Type: labels.MatchRegexp, Name: "level", Value: "err.*"},
		"Negated regexp":     {Input: `level!~"err.*"`, Type: labels.MatchNotRegexp, Name: "level", Value: "err.*"},
		"Parser":             {Input: `json`, Err: _errUnsupportedLogQuery},
		"Comparison":         {Input: `status > 500`, Err: _errUnsupportedLogQuery},
		"No name":            {Input: `="error"`, Err: _errUnsupportedLogQuery},
		"Name with a digit":  {Input: `1level="error"`, Err: _errUnsupportedLogQuery},
		"Value not a string": {Input: `level=error`, Err: _errUnsupportedLogQuery},
		"Invalid regexp":     {Input: `level=~"("`, Err: assert.AnError},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			f, rest, err := parseLabelFilter(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.NotNil(t, f.matcher)
			assert.Equal(t, c.Type, f.matcher.Type)
			assert.Equal(t, c.Name, f.matcher.Name)
			assert.Equal(t, c.Value, f.matcher.Value)
			assert.Equal(t, c.Rest, rest)
		})
	}
}

func Test_readString(t *testing.T) {
	cc := map[string]struct {
		Input string
		Value string
		Rest  string
		Err   error
	}{
		"Double-quoted":           {Input: `"a b" rest`, Value: "a b", Rest: " rest"},
		"Escapes":                 {Input: `"a\"b\\" rest`, Value: `a"b\`, Rest: " rest"},
		"Raw string":              {Input: "`a\\d` rest", Value: `a\d`, Rest: " rest"},
		"Empty input":             {Input: ``, Err: _errUnsupportedLogQuery},
		"Not quoted":              {Input: `abc`, Err: _errUnsupportedLogQuery},
		"Unterminated raw string": {Input: "`abc", Err: processor.NewInvalidQueryError("unterminated raw string")},
		"Unterminated string":     {Input: `"abc`, Err: processor.NewInvalidQueryError("unterminated string")},
		"Invalid escape":          {Input: `"\q"`, Err: assert.AnError},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			value, rest, err := readString(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Value, value)
			assert.Equal(t, c.Rest, rest)
		})
	}
}

func Test_trimSpace(t *testing.T) {
	t.Parallel()

	assert.Equal(t, `|= "a" `, trimSpace(" \t\n|= \"a\" "))
}
