package document

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

// stubSummaries builds three summaries and returns them with their IDs.
func stubSummaries() (Summaries, []xid.ID) {
	ids := []xid.ID{xid.New(), xid.New(), xid.New()}

	return Summaries{
		{ID: ids[0], DocumentName: "a"},
		{ID: ids[1], DocumentName: "b"},
		{ID: ids[2], DocumentName: "c"},
	}, ids
}

// summaryNames lists the document names of the summaries in order.
func summaryNames(ss []Summary) []string {
	names := make([]string, 0, len(ss))

	for _, s := range ss {
		names = append(names, s.DocumentName)
	}

	return names
}

func Test_Summaries_Swap(t *testing.T) {
	t.Parallel()

	type tcase struct {
		Summaries Summaries
		ID        xid.ID
		SortIndex int
		Result    []string
		Err       error
	}

	cc := map[string]tcase{
		"Moves the summary to the target index": func() tcase {
			ss, ids := stubSummaries()

			return tcase{Summaries: ss, ID: ids[0], SortIndex: 2, Result: []string{"b", "c", "a"}}
		}(),
		"Index above the range fails": func() tcase {
			ss, ids := stubSummaries()

			return tcase{Summaries: ss, ID: ids[0], SortIndex: 3, Err: assert.AnError}
		}(),
		"Index below the range fails": func() tcase {
			ss, ids := stubSummaries()

			return tcase{Summaries: ss, ID: ids[0], SortIndex: -1, Err: assert.AnError}
		}(),
		"Unknown ID fails with not found": func() tcase {
			ss, _ := stubSummaries()

			return tcase{Summaries: ss, ID: xid.New(), Err: errutil.ErrNotFound}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			names := summaryNames(c.Summaries)

			res, err := c.Summaries.Swap(c.ID, c.SortIndex)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, summaryNames(res))

			// the receiver is left untouched.
			assert.Equal(t, names, summaryNames(c.Summaries))
		})
	}
}

func Test_Summaries_Remove(t *testing.T) {
	t.Parallel()

	type tcase struct {
		Summaries Summaries
		ID        xid.ID
		Result    []string
		Err       error
	}

	cc := map[string]tcase{
		"Removes the summary by ID": func() tcase {
			ss, ids := stubSummaries()

			return tcase{Summaries: ss, ID: ids[1], Result: []string{"a", "c"}}
		}(),
		"Unknown ID fails with not found": func() tcase {
			ss, _ := stubSummaries()

			return tcase{Summaries: ss, ID: xid.New(), Err: errutil.ErrNotFound}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			names := summaryNames(c.Summaries)

			res, err := c.Summaries.Remove(c.ID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, summaryNames(res))

			// the receiver is left untouched.
			assert.Equal(t, names, summaryNames(c.Summaries))
		})
	}
}

func Test_Summaries_Descendants(t *testing.T) {
	t.Parallel()

	type tcase struct {
		Summaries Summaries
		Result    []string
	}

	cc := map[string]tcase{
		"Empty": {Summaries: Summaries{}, Result: []string{}},
		// every summary is followed by its own descendants, so the flat
		// list reads in document order.
		"Nested": func() tcase {
			ss, ids := stubSummaries()
			ss[0].Children = Summaries{{ID: ids[1], DocumentName: "a1", Children: Summaries{
				{ID: ids[2], DocumentName: "a1a"},
			}}}

			return tcase{Summaries: ss, Result: []string{"a", "a1", "a1a", "b", "c"}}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			names := summaryNames(c.Summaries)

			assert.Equal(t, c.Result, summaryNames(c.Summaries.Descendants()))

			// the receiver is left untouched.
			assert.Equal(t, names, summaryNames(c.Summaries))
		})
	}
}
