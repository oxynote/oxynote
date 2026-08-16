package buildinfo

import (
	"testing"
	"time"

	"github.com/blang/semver/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_parseBuildValues(t *testing.T) {
	cc := map[string]struct {
		RawVersion   string
		RawTimestamp string
		Version      semver.Version
		Timestamp    time.Time
	}{
		"Malformed version": {
			RawVersion:   "not-a-version",
			RawTimestamp: "2006-12-09T19:00:00Z",
			Version:      semver.Version{},
			Timestamp:    time.Date(2006, 12, 9, 19, 0, 0, 0, time.UTC),
		},
		"Malformed timestamp": {
			RawVersion:   "1.2.3",
			RawTimestamp: "not-a-timestamp",
			Version:      semver.MustParse("1.2.3"),
			Timestamp:    time.Time{},
		},
		"Successful parsing": {
			RawVersion:   "v1.2.3-rc.1+name",
			RawTimestamp: "2021-01-02T03:04:05Z",
			Version:      semver.MustParse("1.2.3-rc.1+name"),
			Timestamp:    time.Date(2021, 1, 2, 3, 4, 5, 0, time.UTC),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			version, timestamp := parseBuildValues(c.RawVersion, c.RawTimestamp)
			assert.Equal(t, c.Version, version)
			assert.Equal(t, c.Timestamp, timestamp)
		})
	}
}

func Test_BuildInfo_String(t *testing.T) {
	t.Parallel()

	b := BuildInfo{
		Name:      "oxynote_core",
		Version:   semver.MustParse("1.2.3"),
		Commit:    "abc1234",
		Timestamp: time.Date(2021, 1, 2, 3, 4, 0, 0, time.UTC),
	}

	assert.Equal(
		t,
		"oxynote_core 1.2.3 (abc1234; 2021-01-02 03:04 UTC)",
		b.String(),
	)
}

func Test_BuildInfo_VersionName(t *testing.T) {
	cc := map[string]struct {
		Version semver.Version
		Result  string
	}{
		"No build metadata": {
			Version: semver.MustParse("1.2.3"),
			Result:  "unknown",
		},
		"Build metadata present": {
			Version: semver.MustParse("1.2.3+name.extra"),
			Result:  "name",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			b := BuildInfo{Version: c.Version}
			assert.Equal(t, c.Result, b.VersionName())
		})
	}
}

func Test_BuildInfo_PlainVersion(t *testing.T) {
	t.Parallel()

	b := BuildInfo{Version: semver.MustParse("1.2.3-rc.1+name")}
	assert.Equal(t, "1.2.3-rc.1", b.PlainVersion())
}

func Test_BuildInfo_FormattedTimestamp(t *testing.T) {
	t.Parallel()

	b := BuildInfo{Timestamp: time.Date(2021, 1, 2, 3, 4, 0, 0, time.UTC)}
	assert.Equal(t, "2021-01-02 03:04 UTC", b.FormattedTimestamp())
}

func Test_EnvName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "OXYNOTE_CORE_FOO", EnvName("FOO"))
}

func Test_Getenv(t *testing.T) {
	t.Setenv("OXYNOTE_CORE_TEST_KEY", "test-value")

	assert.Equal(t, "test-value", Getenv("TEST_KEY"))
}

func Test_Full(t *testing.T) {
	// no t.Parallel: Test_IsDevEnv mutates the shared _info variable.
	full := Full()

	require.NotZero(t, full)
	assert.Equal(t, _info, full)
}

func Test_IsDevEnv(t *testing.T) {
	// the environment name lives in the package-level _info variable,
	// so the false branch temporarily mutates it. No t.Parallel to
	// keep other tests from observing the mutation.
	assert.True(t, IsDevEnv())

	env := _info.Env
	defer func() { _info.Env = env }()

	_info.Env = "prod"

	assert.False(t, IsDevEnv())
}
