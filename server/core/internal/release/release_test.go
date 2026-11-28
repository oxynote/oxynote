package release

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/guregu/null/v5"
	"github.com/jarcoal/httpmock"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

const (
	// _testReleaseURL is the GitHub page of the release the tests fetch.
	_testReleaseURL = "https://github.com/oxynote/oxynote/releases/tag/v0.11.0"

	// _testBlogURL is the blog post that announces v0.11.0.
	_testBlogURL = "https://oxynote.io/blog/oxynote-v0-11"
)

// stubMockHTTP returns a client whose transport answers the latest release
// endpoint with latest, every blog post with blog, and the registry's token
// and manifest endpoints with a token and with manifest. Each one answers
// only a request that carries the headers it expects.
func stubMockHTTP(latest, manifest, blog httpmock.Responder) (*http.Client, *httpmock.MockTransport) {
	client, mt := testutil.MockHTTP()

	mt.RegisterMatcherResponder(
		http.MethodGet,
		_latestURL,
		httpmock.HeaderIs("Accept", "application/vnd.github+json").
			And(httpmock.HeaderIs("User-Agent", "oxynote-core")),
		latest,
	)

	mt.RegisterMatcherResponder(
		http.MethodGet,
		_registryAuthURL,
		httpmock.HeaderIs("User-Agent", "oxynote-core"),
		httpmock.NewStringResponder(http.StatusOK, `{"token":"pull-token"}`),
	)

	mt.RegisterRegexpMatcherResponder(
		http.MethodHead,
		regexp.MustCompile(`^https://ghcr\.io/v2/oxynote/oxynote/manifests/`),
		httpmock.HeaderIs("Authorization", "Bearer pull-token").
			And(httpmock.HeaderIs("Accept", _manifestAccept)).
			And(httpmock.HeaderIs("User-Agent", "oxynote-core")),
		manifest,
	)

	mt.RegisterRegexpMatcherResponder(
		http.MethodHead,
		regexp.MustCompile(`^https://oxynote\.io/blog/`),
		httpmock.HeaderIs("User-Agent", "oxynote-core"),
		blog,
	)

	return client, mt
}

// okResp answers 200 with an empty body.
func okResp() httpmock.Responder {
	return httpmock.NewStringResponder(http.StatusOK, "")
}

func Test_NewChecker(t *testing.T) {
	t.Parallel()

	client := &http.Client{}

	c := NewChecker(slog.New(slog.DiscardHandler), client)
	require.NotNil(t, c)
	assert.NotNil(t, c.log)
	assert.Same(t, client, c.client)
	assert.False(t, c.latest.Valid)
}

func Test_Checker_Start(t *testing.T) {
	t.Parallel()

	client, mt := stubMockHTTP(
		httpmock.NewStringResponder(http.StatusServiceUnavailable, ""),
		okResp(),
		okResp(),
	)

	// the context is cancelled up front, so Start runs its first check and
	// returns instead of waiting for the next one.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	NewChecker(slog.New(slog.DiscardHandler), client).Start(ctx)

	assert.Equal(t, 1, mt.GetTotalCallCount())
}

func Test_Checker_Latest(t *testing.T) {
	t.Parallel()

	rel := Release{
		Version:  semver.MustParse("0.11.0"),
		NotesURL: _testBlogURL,
	}

	cc := map[string]struct {
		Latest null.Value[Release]
		Result null.Value[Release]
	}{
		"No release known": {},
		"Release known": {
			Latest: null.ValueFrom(rel),
			Result: null.ValueFrom(rel),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ch := &Checker{latest: c.Latest}

			assert.Equal(t, c.Result, ch.Latest())
		})
	}
}

func Test_Checker_check(t *testing.T) {
	t.Parallel()

	prev := Release{
		Version:  semver.MustParse("0.10.0"),
		NotesURL: "https://oxynote.io/blog/oxynote-v0-10",
	}

	cc := map[string]struct {
		Latest null.Value[Release]
		Resp   httpmock.Responder
		Result null.Value[Release]
		Log    string
	}{
		"Error returned by fetch without a previous release": {
			Resp: httpmock.NewStringResponder(http.StatusServiceUnavailable, ""),
			Log:  `level=WARN msg="cannot check the latest release" error="latest release endpoint returned status 503"`,
		},
		"Error returned by fetch keeps the previous release": {
			Latest: null.ValueFrom(prev),
			Resp:   httpmock.NewErrorResponder(assert.AnError),
			Result: null.ValueFrom(prev),
			Log:    `level=WARN msg="cannot check the latest release"`,
		},
		"Successful check replaces the previous release": {
			Latest: null.ValueFrom(prev),
			Resp: httpmock.NewStringResponder(
				http.StatusOK,
				`{"tag_name":"v0.11.1","html_url":"https://github.com/oxynote/oxynote/releases/tag/v0.11.1"}`,
			),
			Result: null.ValueFrom(Release{
				Version:  semver.MustParse("0.11.1"),
				NotesURL: "https://github.com/oxynote/oxynote/releases/tag/v0.11.1",
			}),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client, _ := stubMockHTTP(c.Resp, okResp(), httpmock.NewStringResponder(http.StatusNotFound, ""))

			var buf bytes.Buffer

			ch := &Checker{
				log:    slog.New(slog.NewTextHandler(&buf, nil)),
				client: client,
				latest: c.Latest,
			}

			ch.check(context.Background())

			assert.Equal(t, c.Result, ch.Latest())

			if c.Log == "" {
				assert.Empty(t, buf.String())
				return
			}

			assert.Contains(t, buf.String(), c.Log)
		})
	}
}

func Test_Checker_fetch(t *testing.T) {
	t.Parallel()

	minor := `{"tag_name":"v0.11.0","html_url":"` + _testReleaseURL + `"}`

	cc := map[string]struct {
		Resp         httpmock.Responder
		ManifestResp httpmock.Responder
		BlogResp     httpmock.Responder
		Calls        int
		Result       Release
		Err          error
	}{
		"Error returned by http.Client.Do": {
			Resp:         httpmock.NewErrorResponder(assert.AnError),
			ManifestResp: okResp(),
			BlogResp:     okResp(),
			Calls:        1,
			Err:          assert.AnError,
		},
		"Unexpected status code": {
			Resp:         httpmock.NewStringResponder(http.StatusNotFound, `{"message":"Not Found"}`),
			ManifestResp: okResp(),
			BlogResp:     okResp(),
			Calls:        1,
			Err:          errors.New("latest release endpoint returned status 404"),
		},
		"Malformed JSON": {
			Resp:         httpmock.NewStringResponder(http.StatusOK, `{"tag_name":`),
			ManifestResp: okResp(),
			BlogResp:     okResp(),
			Calls:        1,
			Err:          assert.AnError,
		},
		"Invalid version": {
			Resp:         httpmock.NewStringResponder(http.StatusOK, `{"tag_name":"nightly","html_url":"`+_testReleaseURL+`"}`),
			ManifestResp: okResp(),
			BlogResp:     okResp(),
			Calls:        1,
			Err:          assert.AnError,
		},
		"Error returned by imagePublished": {
			Resp:         httpmock.NewStringResponder(http.StatusOK, minor),
			ManifestResp: httpmock.NewErrorResponder(assert.AnError),
			BlogResp:     okResp(),
			Calls:        3,
			Err:          assert.AnError,
		},
		"Image not published yet": {
			Resp:         httpmock.NewStringResponder(http.StatusOK, minor),
			ManifestResp: httpmock.NewStringResponder(http.StatusNotFound, ""),
			BlogResp:     okResp(),
			Calls:        3,
			Err:          errors.New("image 0.11.0 is not published yet"),
		},
		"Successful fetch": {
			Resp:         httpmock.NewStringResponder(http.StatusOK, minor),
			ManifestResp: okResp(),
			BlogResp:     okResp(),
			Calls:        4,
			Result: Release{
				Version:  semver.MustParse("0.11.0"),
				NotesURL: _testBlogURL,
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client, mt := stubMockHTTP(c.Resp, c.ManifestResp, c.BlogResp)
			ch := &Checker{
				log:    slog.New(slog.DiscardHandler),
				client: client,
			}

			res, err := ch.fetch(context.Background())
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Calls, mt.GetTotalCallCount())

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_Checker_imagePublished(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		TokenResp    httpmock.Responder
		ManifestResp httpmock.Responder
		Calls        int
		Result       bool
		Err          error
	}{
		"Error returned by pullToken": {
			TokenResp:    httpmock.NewErrorResponder(assert.AnError),
			ManifestResp: okResp(),
			Calls:        1,
			Err:          assert.AnError,
		},
		"Error returned by http.Client.Do": {
			ManifestResp: httpmock.NewErrorResponder(assert.AnError),
			Calls:        2,
			Err:          assert.AnError,
		},
		"Unexpected status code": {
			ManifestResp: httpmock.NewStringResponder(http.StatusUnauthorized, ""),
			Calls:        2,
			Err:          errors.New("image manifest endpoint returned status 401"),
		},
		"Image not found": {
			ManifestResp: httpmock.NewStringResponder(http.StatusNotFound, ""),
			Calls:        2,
		},
		"Image found": {
			ManifestResp: okResp(),
			Calls:        2,
			Result:       true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tokenResp := c.TokenResp
			if tokenResp == nil {
				tokenResp = httpmock.NewStringResponder(http.StatusOK, `{"token":"pull-token"}`)
			}

			client, mt := testutil.MockHTTP()
			mt.RegisterResponder(http.MethodGet, _registryAuthURL, tokenResp)
			mt.RegisterMatcherResponder(
				http.MethodHead,
				"https://ghcr.io/v2/oxynote/oxynote/manifests/0.11.0",
				httpmock.HeaderIs("Authorization", "Bearer pull-token").
					And(httpmock.HeaderIs("Accept", _manifestAccept)).
					And(httpmock.HeaderIs("User-Agent", "oxynote-core")),
				c.ManifestResp,
			)

			ch := &Checker{
				log:    slog.New(slog.DiscardHandler),
				client: client,
			}

			res, err := ch.imagePublished(context.Background(), semver.MustParse("0.11.0"))
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Calls, mt.GetTotalCallCount())

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_Checker_pullToken(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Resp   httpmock.Responder
		Result string
		Err    error
	}{
		"Error returned by http.Client.Do": {
			Resp: httpmock.NewErrorResponder(assert.AnError),
			Err:  assert.AnError,
		},
		"Unexpected status code": {
			Resp: httpmock.NewStringResponder(http.StatusServiceUnavailable, ""),
			Err:  errors.New("token endpoint returned status 503"),
		},
		"Malformed JSON": {
			Resp: httpmock.NewStringResponder(http.StatusOK, `{"token":`),
			Err:  assert.AnError,
		},
		"Successful request": {
			Resp:   httpmock.NewStringResponder(http.StatusOK, `{"token":"pull-token"}`),
			Result: "pull-token",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client, mt := testutil.MockHTTP()
			mt.RegisterMatcherResponder(
				http.MethodGet,
				_registryAuthURL,
				httpmock.HeaderIs("User-Agent", "oxynote-core"),
				c.Resp,
			)

			ch := &Checker{
				log:    slog.New(slog.DiscardHandler),
				client: client,
			}

			res, err := ch.pullToken(context.Background())
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, 1, mt.GetTotalCallCount())

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_Checker_notesURL(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Version  semver.Version
		BlogResp httpmock.Responder
		Calls    int
		Result   string
		Log      string
	}{
		"Pre-release skips the blog post": {
			Version:  semver.MustParse("0.11.0-rc.1"),
			BlogResp: httpmock.NewStringResponder(http.StatusOK, ""),
			Result:   _testReleaseURL,
		},
		"Error returned by http.Client.Do": {
			Version:  semver.MustParse("0.11.0"),
			BlogResp: httpmock.NewErrorResponder(assert.AnError),
			Calls:    1,
			Result:   _testReleaseURL,
			Log:      `level=DEBUG msg="cannot check the release blog post"`,
		},
		"Blog post not found": {
			Version:  semver.MustParse("0.11.0"),
			BlogResp: httpmock.NewStringResponder(http.StatusNotFound, ""),
			Calls:    1,
			Result:   _testReleaseURL,
		},
		"Blog post found": {
			Version:  semver.MustParse("0.11.0"),
			BlogResp: httpmock.NewStringResponder(http.StatusOK, ""),
			Calls:    1,
			Result:   _testBlogURL,
		},
		"Blog post of a patch release found": {
			Version:  semver.MustParse("0.11.2"),
			BlogResp: httpmock.NewStringResponder(http.StatusOK, ""),
			Calls:    1,
			Result:   "https://oxynote.io/blog/oxynote-v0-11-2",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client, mt := stubMockHTTP(okResp(), okResp(), c.BlogResp)

			var buf bytes.Buffer

			ch := &Checker{
				log:    slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
				client: client,
			}

			assert.Equal(t, c.Result, ch.notesURL(context.Background(), c.Version, _testReleaseURL))
			assert.Equal(t, c.Calls, mt.GetTotalCallCount())

			if c.Log == "" {
				assert.Empty(t, buf.String())
				return
			}

			assert.Contains(t, buf.String(), c.Log)
		})
	}
}

func Test_blogVersion(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Version semver.Version
		Result  string
	}{
		"Minor release": {Version: semver.MustParse("0.11.0"), Result: "0-11"},
		"Patch release": {Version: semver.MustParse("0.11.2"), Result: "0-11-2"},
		"Major release": {Version: semver.MustParse("1.0.0"), Result: "1-0"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, blogVersion(c.Version))
		})
	}
}

func Test_IsReleaseVersion(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Version string
		Result  bool
	}{
		"Release":           {Version: "0.11.0", Result: true},
		"Development build": {Version: "dev"},
		"Empty":             {Version: ""},
		"Leading v":         {Version: "v0.11.0"},
		"Pre-release":       {Version: "0.12.0-rc.1"},
		"Build metadata":    {Version: "0.11.0+abc"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, IsReleaseVersion(c.Version))
		})
	}
}
