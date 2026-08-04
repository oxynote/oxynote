package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
)

func Test_Query_Validate(t *testing.T) {
	cc := map[string]struct {
		Query Query
		Err   error
	}{
		"Invalid count value": {
			Query: Query{
				Limit: 0,
				Page:  20,
			},
			Err: assert.AnError,
		},
		"Invalid page value": {
			Query: Query{
				Limit: 3,
				Page:  0,
			},
			Err: assert.AnError,
		},
		"Successful validation": {
			Query: Query{
				Limit: 3,
				Page:  20,
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()
			testutil.AssertEqualError(t, c.Err, c.Query.Validate())
		})
	}
}

func Test_ParseQuery(t *testing.T) {
	cc := map[string]struct {
		Input       map[string][]string
		NilPostBody bool
		Result      Query
		Err         error
	}{
		"DecodeForm returns an error": {
			NilPostBody: true,
			Err:         assert.AnError,
		},
		"Invalid filter value": {
			Input: map[string][]string{
				"filter-test": {""},
			},
			Err: assert.AnError,
		},
		"Invalid sorting order priority index": {
			Input: map[string][]string{
				"limit":          {"10"},
				"page":           {"15"},
				"filter-":        {"123"},
				"filter-hello":   {"123"},
				"filter-test":    {"333"},
				"sort-":          {"123"},
				"sort-test-good": {"asc"},
				"sort-2-evening": {"asc"},
			},
			Err: assert.AnError,
		},
		"Invalid sorting order value": {
			Input: map[string][]string{
				"limit":          {"10"},
				"page":           {"15"},
				"filter-":        {"123"},
				"filter-hello":   {"123"},
				"filter-test":    {"333"},
				"sort-":          {"123"},
				"sort-1-good":    {"asc"},
				"sort-2-evening": {"hello"},
			},
			Err: assert.AnError,
		},
		"Duplicate sort priority": {
			Input: map[string][]string{
				"limit":          {"10"},
				"page":           {"15"},
				"filter-":        {"123"},
				"filter-hello":   {"123"},
				"filter-test":    {"333"},
				"sort-":          {"123"},
				"sort-2":         {"123"},
				"sort-100-test1": {"desc"},
				"sort-1-test2":   {"asc"},
				"sort-300-test3": {"desc"},
				"sort-70-test4":  {"asc"},
				"sort-70-test5":  {"asc"},
			},
			Err: assert.AnError,
		},
		"Duplicate sort key": {
			Input: map[string][]string{
				"limit":          {"10"},
				"page":           {"15"},
				"filter-":        {"123"},
				"filter-hello":   {"123"},
				"filter-test":    {"333"},
				"sort-":          {"123"},
				"sort-2":         {"123"},
				"sort-100-test1": {"desc"},
				"sort-1-test2":   {"asc"},
				"sort-300-test3": {"desc"},
				"sort-70-test4":  {"asc"},
				"sort-71-test4":  {"asc"},
			},
			Err: assert.AnError,
		},
		"Successful execution": {
			Input: map[string][]string{
				"limit":                     {"10"},
				"page":                      {"15"},
				"filter-":                   {"123"},
				"filter-hello":              {"123"},
				"filter-test":               {"333"},
				"filter-test-hello-world":   {"333"},
				"sort-":                     {"123"},
				"sort-2":                    {"123"},
				"sort-100-test1":            {"desc"},
				"sort-1-test2":              {"asc"},
				"sort-300-test3":            {"desc"},
				"sort-70-test4":             {"asc"},
				"sort-71-test5-hello":       {"asc"},
				"sort-72-test6-hello-world": {"desc"},
				"omissions":                 {"hello", "hi"},
			},
			Result: Query{
				Limit: 10,
				Page:  15,
				Filters: map[string]string{
					"hello":            "123",
					"test":             "333",
					"test-hello-world": "333",
				},
				SortKeys: []SortKey{
					{
						Key: "test2",
						Asc: true,
					},
					{
						Key: "test4",
						Asc: true,
					},
					{
						Key: "test5-hello",
						Asc: true,
					},
					{
						Key: "test6-hello-world",
						Asc: false,
					},
					{
						Key: "test1",
						Asc: false,
					},
					{
						Key: "test3",
						Asc: false,
					},
				},
				Omissions: map[string]struct{}{
					"hello": {},
					"hi":    {},
				},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest("GET", "http://test.com/", http.NoBody)
			q := req.URL.Query()

			for k, vv := range c.Input {
				for _, v := range vv {
					q.Add(k, v)
				}
			}

			req.URL.RawQuery = q.Encode()

			if c.NilPostBody {
				req.Method = "POST"
				req.Body = nil
			}

			res, err := ParseQuery(req)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}
