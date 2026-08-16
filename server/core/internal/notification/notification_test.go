package notification

import (
	"errors"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_newNotification(t *testing.T) {
	t.Parallel()

	nc := Core{
		Code:     NotificationDocumentReviewRequest,
		Metadata: Metadata{_metaKeyUserID: "user1"},
	}

	nt := newNotification("org1", "user1", nc)

	require.NotNil(t, nt)
	assert.Equal(t, nc, nt.Core)
	assert.False(t, nt.ID.IsNil())
	assert.Equal(t, "user1", nt.UserID)
	assert.Equal(t, "org1", nt.OrganizationID)
	assert.False(t, nt.Read)
	assert.WithinDuration(t, timeutil.Now(), nt.CreatedAt, time.Second)
}

func Test_Metadata_Value(t *testing.T) {
	t.Parallel()

	md := Metadata{_metaKeyUserID: "user1", "read": true}

	v, err := md.Value()
	require.NoError(t, err)

	assert.JSONEq(t, `{"userId":"user1","read":true}`, string(v.([]byte)))
}

func Test_Metadata_Scan(t *testing.T) {
	cc := map[string]struct {
		Src    any
		Result Metadata
		Err    error
	}{
		"Invalid source type": {
			Src: 123,
			Err: errors.New("invalid metadata type"),
		},
		"Malformed JSON": {
			Src: []byte("{"),
			Err: assert.AnError,
		},
		"Successful scan from bytes": {
			Src:    []byte(`{"userId":"user1"}`),
			Result: Metadata{_metaKeyUserID: "user1"},
		},
		"Successful scan from string": {
			Src:    `{"userId":"user1"}`,
			Result: Metadata{_metaKeyUserID: "user1"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var md Metadata

			err := md.Scan(c.Src)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, md)
		})
	}
}
