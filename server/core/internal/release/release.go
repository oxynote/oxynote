// Package release finds the latest Oxynote release and where its notes are
// published.
package release

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/blang/semver/v4"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/logutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
)

const (
	// _latestURL is the GitHub API endpoint that describes the latest
	// release.
	_latestURL = "https://api.github.com/repos/oxynote/oxynote/releases/latest"

	// _registryAuthURL hands out an anonymous token that may pull the
	// public image.
	_registryAuthURL = "https://ghcr.io/token?scope=repository:oxynote/oxynote:pull"

	// _manifestURLFormat is the URL of the image manifest for a tag,
	// taking the version without its leading v.
	_manifestURLFormat = "https://ghcr.io/v2/oxynote/oxynote/manifests/%s"

	// _manifestAccept lists the manifest types the image is published as.
	_manifestAccept = "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json"

	// _blogURLFormat is the URL of the blog post that announces a
	// release, taking its version written with dashes, such as 0-11 or
	// 0-11-2.
	_blogURLFormat = "https://oxynote.io/blog/oxynote-v%s"

	// _userAgent identifies core to the servers it asks.
	_userAgent = "oxynote-core"

	// _requestTimeout bounds one outbound request.
	_requestTimeout = 10 * time.Second

	// _checkInterval specifies how often the latest release is checked.
	_checkInterval = 3 * time.Hour
)

// Release describes a published Oxynote release.
type Release struct {
	// Version specifies the version of the release.
	Version semver.Version `json:"version"`

	// NotesURL specifies where the notes of the release are published.
	NotesURL string `json:"notesUrl"`
}

// Checker keeps the latest release, checked periodically.
type Checker struct {
	log    *slog.Logger
	client *http.Client

	mu     sync.RWMutex
	latest null.Value[Release]
}

// NewChecker creates a fresh instance of Checker.
func NewChecker(log *slog.Logger, client *http.Client) *Checker {
	return &Checker{
		log:    log.With("component", "release-checker"),
		client: client,
	}
}

// Start checks the latest release at once and then every _checkInterval
// until ctx is cancelled. It blocks, so the caller owns the goroutine it
// runs on.
func (c *Checker) Start(ctx context.Context) {
	c.log.Info("starting")
	defer c.log.Info("stopped")

	timeutil.NewPeriodicExec(
		_checkInterval,
		0,
		c.check,
		logutil.RecoveryValue(c.log, logutil.NewRecoveryPlan("recovered from a panic while checking the latest release")),
		true,
	).Start(ctx)
}

// Latest returns the latest release, null while none is known.
func (c *Checker) Latest() null.Value[Release] {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.latest
}

// check fetches the latest release and keeps it. A failed check keeps the
// release found before.
func (c *Checker) check(ctx context.Context) {
	rel, err := c.fetch(ctx)
	if err != nil {
		// an install without internet access never reaches GitHub, so a
		// failure is expected and nothing a human must act on.
		c.log.Warn("cannot check the latest release", slog.String("error", err.Error()))

		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.latest = null.ValueFrom(rel)
}

// fetch asks GitHub for the latest release.
func (c *Checker) fetch(ctx context.Context) (Release, error) {
	reqCtx, cancel := context.WithTimeout(ctx, _requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, _latestURL, http.NoBody)
	if err != nil {
		// NOCOV: the method and the URL are constants.
		return Release{}, fmt.Errorf("creating the request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", _userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("requesting the latest release: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // error provides no meaningful info

	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("latest release endpoint returned status %d", resp.StatusCode)
	}

	var body struct {
		TagName string `json:"tag_name"` //nolint:tagliatelle // GitHub names its fields in snake case
		HTMLURL string `json:"html_url"` //nolint:tagliatelle // GitHub names its fields in snake case
	}

	if err = json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Release{}, fmt.Errorf("decoding the latest release: %w", err)
	}

	v, err := semver.ParseTolerant(body.TagName)
	if err != nil {
		return Release{}, fmt.Errorf("parsing the release version: %w", err)
	}

	// NOTE: a release is published before its image is built, so it
	// counts only once operators can pull it.
	published, err := c.imagePublished(ctx, v)
	if err != nil {
		return Release{}, fmt.Errorf("checking the release image: %w", err)
	}

	if !published {
		return Release{}, fmt.Errorf("image %s is not published yet", v)
	}

	return Release{
		Version:  v,
		NotesURL: c.notesURL(ctx, v, body.HTMLURL),
	}, nil
}

// imagePublished reports whether the image of the release can be pulled.
func (c *Checker) imagePublished(ctx context.Context, v semver.Version) (bool, error) {
	token, err := c.pullToken(ctx)
	if err != nil {
		return false, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, _requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(
		reqCtx,
		http.MethodHead,
		fmt.Sprintf(_manifestURLFormat, v),
		http.NoBody,
	)
	if err != nil {
		// NOCOV: the URL is a constant format filled with a version.
		return false, fmt.Errorf("creating the request: %w", err)
	}

	req.Header.Set("Accept", _manifestAccept)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", _userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("requesting the image manifest: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // error provides no meaningful info

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("image manifest endpoint returned status %d", resp.StatusCode)
	}
}

// pullToken asks the registry for an anonymous token that may pull the
// image.
func (c *Checker) pullToken(ctx context.Context) (string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, _requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, _registryAuthURL, http.NoBody)
	if err != nil {
		// NOCOV: the method and the URL are constants.
		return "", fmt.Errorf("creating the request: %w", err)
	}

	req.Header.Set("User-Agent", _userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting a pull token: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // error provides no meaningful info

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned status %d", resp.StatusCode)
	}

	var body struct {
		Token string `json:"token"`
	}

	if err = json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decoding the pull token: %w", err)
	}

	return body.Token, nil
}

// notesURL returns where the notes of the release are published. A
// release announced in a blog post, such as v0.11.0 or v0.11.2, links the
// post once it is online. Every other case falls back to the release page.
func (c *Checker) notesURL(ctx context.Context, v semver.Version, releaseURL string) string {
	if len(v.Pre) > 0 {
		return releaseURL
	}

	blogURL := fmt.Sprintf(_blogURLFormat, blogVersion(v))

	ctx, cancel := context.WithTimeout(ctx, _requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, blogURL, http.NoBody)
	if err != nil {
		// NOCOV: the URL is a constant format filled with numbers.
		return releaseURL
	}

	req.Header.Set("User-Agent", _userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		// the release page is a complete answer, so a blog that cannot be
		// reached only loses the nicer link.
		c.log.Debug("cannot check the release blog post", slog.String("error", err.Error()))

		return releaseURL
	}
	defer resp.Body.Close() //nolint:errcheck // error provides no meaningful info

	if resp.StatusCode != http.StatusOK {
		return releaseURL
	}

	return blogURL
}

// blogVersion writes the version the way blog post URLs do: 0-11 for a
// minor release and 0-11-2 for a patch release.
func blogVersion(v semver.Version) string {
	if v.Patch == 0 {
		return fmt.Sprintf("%d-%d", v.Major, v.Minor)
	}

	return fmt.Sprintf("%d-%d-%d", v.Major, v.Minor, v.Patch)
}

// IsReleaseVersion reports whether version names a published release,
// such as 0.11.0.
func IsReleaseVersion(version string) bool {
	v, err := semver.Parse(version)

	return err == nil && len(v.Pre) == 0 && len(v.Build) == 0
}
