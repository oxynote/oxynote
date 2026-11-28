package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/release"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
)

func Test_Server_fetchLatestRelease(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Latest   null.Value[release.Release]
		RespCode int
		RespJSON string
	}{
		"No release known": {
			RespCode: http.StatusOK,
			RespJSON: `null`,
		},
		"Release known": {
			Latest: null.ValueFrom(release.Release{
				Version:  semver.MustParse("0.11.0"),
				NotesURL: "https://oxynote.io/blog/oxynote-v0-11",
			}),
			RespCode: http.StatusOK,
			RespJSON: `{"version":"0.11.0","notesUrl":"https://oxynote.io/blog/oxynote-v0-11"}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			releases := &ReleaseCheckerMock{
				LatestFunc: func() null.Value[release.Release] {
					return c.Latest
				},
			}

			s := Server{
				log:      slog.New(slog.DiscardHandler),
				releases: releases,
			}

			req := httptest.NewRequest(http.MethodGet, "http://test.com/", http.NoBody)
			rec := httptest.NewRecorder()

			s.fetchLatestRelease(rec, req)

			testutil.AssertResponse(t, rec, c.RespCode, c.RespJSON)
			assert.Len(t, releases.LatestCalls(), 1)
		})
	}
}
