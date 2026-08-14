// Package buildinfo provides general information about the exec build.
package buildinfo

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/oxynote/oxynote/server/core/pkg/logutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
)

var (
	_name            = "oxynote_core"
	_version         = "0.0.0-dev"
	_commit          = "0000000"
	_timestampString = "2006-12-09T19:00:00Z"

	_info = BuildInfo{
		Env:    "dev",
		Name:   _name,
		Commit: _commit,
	}
)

func init() { //nolint:gochecknoinits // we need to prepare build info
	logutil.ShowCritical = IsDevEnv()

	// only strings can be inserted via ldflags which means
	// some information needs to be parsed. Malformed injected values
	// fall back to zero defaults instead of failing the boot.

	version, err := semver.ParseTolerant(_version)
	if err != nil {
		version = semver.Version{}
	}

	_info.Version = version

	timestamp, err := timeutil.Parse(_timestampString)
	if err != nil {
		timestamp = time.Time{}
	}

	_info.Timestamp = timestamp
}

// BuildInfo contains information about the executable.
type BuildInfo struct {
	// Env specifies the execution environment type.
	Env string `json:"env"`

	// Name specifies the name of the executable.
	Name string `json:"name"`

	// Version specifies the version of the executable.
	Version semver.Version `json:"version"`

	// Commit specifies the hash of the commit that was used to build
	// the executable.
	Commit string `json:"commit"`

	// Timestamp specifies the build timestamp of the executable.
	Timestamp time.Time `json:"timestamp"`
}

// String turns build information into a string.
func (b BuildInfo) String() string {
	return fmt.Sprintf(
		"%s %s (%s; %s)",
		b.Name,
		b.Version,
		b.Commit,
		b.FormattedTimestamp(),
	)
}

// VersionName extracts the name of the version from semver build
// metadata.
func (b BuildInfo) VersionName() string {
	if len(b.Version.Build) > 0 {
		return b.Version.Build[0]
	}

	return "unknown"
}

// PlainVersion returns a version string without metadata.
func (b BuildInfo) PlainVersion() string {
	v := b.Version
	v.Build = nil

	return v.String()
}

// FormattedTimestamp returns a formatted timestamp's string.
func (b BuildInfo) FormattedTimestamp() string {
	return b.Timestamp.Format(timeutil.ReadableFormat)
}

// EnvName combines the provided key with executable name.
func EnvName(key string) string {
	return fmt.Sprintf("%s_%s", strings.ToUpper(_name), key)
}

// Getenv retrieves an environment variable value by the provided key
// and name combination.
func Getenv(key string) string {
	return os.Getenv(EnvName(key))
}

// Full returns all information about the executable.
func Full() BuildInfo {
	return _info
}

// IsDevEnv determines whether the environment is dev.
func IsDevEnv() bool {
	return _info.Env == "dev"
}
