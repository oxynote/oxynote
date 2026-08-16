package document

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func Test_Summaries_Swap(t *testing.T) {
	t.Parallel()

	t.Run("Moves the summary to the target index", func(t *testing.T) {
		t.Parallel()

		ss, ids := stubSummaries()

		res, err := ss.Swap(ids[0], 2)
		require.NoError(t, err)

		assert.Equal(t, []string{"b", "c", "a"}, []string{
			res[0].DocumentName,
			res[1].DocumentName,
			res[2].DocumentName,
		})
	})

	t.Run("Out-of-range index fails", func(t *testing.T) {
		t.Parallel()

		ss, ids := stubSummaries()

		_, err := ss.Swap(ids[0], 3)
		require.Error(t, err)

		_, err = ss.Swap(ids[0], -1)
		require.Error(t, err)
	})

	t.Run("Unknown ID fails with not found", func(t *testing.T) {
		t.Parallel()

		ss, _ := stubSummaries()

		_, err := ss.Swap(xid.New(), 0)
		assert.ErrorIs(t, err, errutil.ErrNotFound)
	})
}

func Test_Summaries_Remove(t *testing.T) {
	t.Parallel()

	t.Run("Removes the summary by ID", func(t *testing.T) {
		t.Parallel()

		ss, ids := stubSummaries()

		res, err := ss.Remove(ids[1])
		require.NoError(t, err)

		require.Len(t, res, 2)
		assert.Equal(t, "a", res[0].DocumentName)
		assert.Equal(t, "c", res[1].DocumentName)
	})

	t.Run("Unknown ID fails with not found", func(t *testing.T) {
		t.Parallel()

		ss, _ := stubSummaries()

		_, err := ss.Remove(xid.New())
		assert.ErrorIs(t, err, errutil.ErrNotFound)
	})
}
