package processor

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
)

const (
	// _maxDataPoints defines the default maximum number of data points for interval calculations.
	_maxDataPoints = 100

	// _minInterval is the smallest step interval used for queries.
	_minInterval = 15 * time.Second

	// _maxInterval is the largest step interval used for queries.
	_maxInterval = 24 * time.Hour

	// _hoursPerDay is used when formatting durations as days.
	_hoursPerDay = 24

	// _fromKey names the range start in query params and macro variables.
	_fromKey = "from"

	// _timeColumn is the canonical name of the time column produced by
	// the $__time-family macros and expected in SQL chart results.
	_timeColumn = "time"

	// _maxVolumeBuckets is the most buckets a log volume splits its range
	// into.
	_maxVolumeBuckets = 100
)

// _volumeSteps are the steps a log volume counts on, shortest first. Each
// is at most 3.5 times the one before it. So the shortest step that leaves
// at most _maxVolumeBuckets buckets still leaves at least 20, for any range
// of 20 seconds or more.
var _volumeSteps = []time.Duration{
	time.Second,
	2 * time.Second,
	5 * time.Second,
	10 * time.Second,
	15 * time.Second,
	30 * time.Second,
	time.Minute,
	2 * time.Minute,
	5 * time.Minute,
	10 * time.Minute,
	15 * time.Minute,
	30 * time.Minute,
	time.Hour,
	2 * time.Hour,
	6 * time.Hour,
	12 * time.Hour,
	24 * time.Hour,
	2 * 24 * time.Hour,
	7 * 24 * time.Hour,
	14 * 24 * time.Hour,
	30 * 24 * time.Hour,
	90 * 24 * time.Hour,
	180 * 24 * time.Hour,
	365 * 24 * time.Hour,
}

// TimeRange represents a time range with a start and end time.
type TimeRange struct {
	// From is the start time of the range.
	From time.Time `json:"from"`

	// To is the end time of the range.
	To time.Time `json:"to"`
}

// ParseTimeRange parses a TimeRange from URL values.
func ParseTimeRange(
	fromOptional bool,
	v url.Values,
) (*TimeRange, error) {
	from, err := time.Parse(time.RFC3339, v.Get(_fromKey))
	if err != nil {
		if !fromOptional || v.Get(_fromKey) != "" {
			return nil, errutil.New(http.StatusBadRequest, "from.invalid", "from parameter must be a valid RFC3339 timestamp")
		}

		from = time.Time{}
	}

	to := timeutil.Now()
	rawTo := v.Get("to")

	if rawTo != "" {
		to, err = time.Parse(time.RFC3339, rawTo)
		if err != nil {
			return nil, errutil.New(http.StatusBadRequest, "to.invalid", "to parameter must be a valid RFC3339 timestamp")
		}
	}

	if !from.IsZero() && from.After(to) {
		return nil, errutil.New(
			http.StatusBadRequest,
			"from.after_to",
			"from parameter must not be after the to parameter",
		)
	}

	return &TimeRange{
		From: from,
		To:   to,
	}, nil
}

// QueryStep calculates the step duration for range queries based on the
// time range. calculateInterval already floors at _minInterval, so no
// extra clamp is needed here.
func (tr TimeRange) QueryStep() time.Duration {
	return tr.calculateInterval()
}

// VolumeStep returns the step a log volume of the range counts on: the
// shortest of _volumeSteps that leaves at most _maxVolumeBuckets buckets,
// or the longest one. A range that does not start on a step boundary
// touches one bucket more than it fills, so the step fits the range into
// one bucket fewer. A range shorter than 20 seconds gets one bucket per
// second, which is fewer than 20.
func (tr TimeRange) VolumeStep() time.Duration {
	span := tr.To.Sub(tr.From)

	for _, step := range _volumeSteps {
		if step*(_maxVolumeBuckets-1) >= span {
			return step
		}
	}

	return _volumeSteps[len(_volumeSteps)-1]
}

// Normalize adjusts the From and To times to align with the specified step duration.
func (tr TimeRange) Normalize() TimeRange {
	return tr.AlignTo(tr.QueryStep())
}

// AlignTo rounds From down and To up to a multiple of d, so ranges a few
// seconds apart within one interval of d become the same range. A zero
// time is left as it is.
func (tr TimeRange) AlignTo(d time.Duration) TimeRange {
	from := tr.From
	to := tr.To

	seconds := int64(d.Seconds())

	if !from.IsZero() {
		from = time.Unix((from.Unix()/seconds)*seconds, 0).UTC()
	}

	if !to.IsZero() {
		// Unix drops the fraction of a second, which would round To down.
		sec := to.Unix()
		if to.Nanosecond() > 0 {
			sec++
		}

		to = time.Unix(((sec + seconds - 1) / seconds * seconds), 0).UTC()
	}

	return TimeRange{
		From: from,
		To:   to,
	}
}

// calculateInterval calculates the appropriate step interval based on the time range
// and maximum number of data points.
func (tr TimeRange) calculateInterval() time.Duration {
	duration := tr.To.Sub(tr.From)
	if duration <= 0 {
		return _minInterval
	}

	interval := duration / time.Duration(_maxDataPoints)

	return roundInterval(interval)
}

// _timeVarRe matches ${__from} and ${__to} with optional :date modifiers.
//
// Examples: ${__from}, ${__from:date}, ${__from:date:iso}, ${__from:date:YYYY-MM}.
var _timeVarRe = regexp.MustCompile(`\$\{__(from|to)(?::(date)(?::([^}]+))?)?\}`)

// _isoMillisFormat is ISO 8601/RFC 3339 with millisecond precision.
const _isoMillisFormat = "2006-01-02T15:04:05.000Z07:00"

// _timeFormatReplacer converts date format tokens (YYYY, MM, DD, etc.) to Go time layout tokens.
// Longer tokens must appear before shorter ones to ensure correct matching.
var _timeFormatReplacer = strings.NewReplacer(
	"YYYY", "2006",
	"YY", "06",
	"MMMM", "January",
	"MMM", "Jan",
	"MM", "01",
	"M", "1",
	"DD", "02",
	"D", "2",
	"dddd", "Monday",
	"ddd", "Mon",
	"HH", "15",
	"hh", "03",
	"h", "3",
	"mm", "04",
	"m", "4",
	"ss", "05",
	"s", "5",
	"SSS", ".000",
	"A", "PM",
	"a", "pm",
	"ZZ", "-0700",
)

// ProcessQuery processes the query string to replace generic macros
// that are shared across all data source types.
//
// Supported macros:
//
//	$__from                  → unix milliseconds of the start time
//	$__to                    → unix milliseconds of the end time
//	$__interval              → calculated step interval (e.g., "15s", "5m", "1h")
//	${__from}                → unix milliseconds of the start time
//	${__to}                  → unix milliseconds of the end time
//	${__from:date}           → ISO 8601 with milliseconds (e.g., "2020-07-13T20:19:09.254Z")
//	${__from:date:iso}       → ISO 8601 with milliseconds
//	${__from:date:seconds}   → unix seconds
//	${__from:date:YYYY-MM}   → custom date format
//
// Source-specific processors call this first, so its results can be used as
// arguments to their own macros ($__timeGroupAlias("time", $__interval)).
func (tr TimeRange) ProcessQuery(q string) string {
	// Replace ${__from...} and ${__to...} with modifiers first.
	q = _timeVarRe.ReplaceAllStringFunc(q, func(match string) string {
		submatch := _timeVarRe.FindStringSubmatch(match)

		var t time.Time
		if submatch[1] == _fromKey {
			t = tr.From.UTC()
		} else {
			t = tr.To.UTC()
		}

		// No :date modifier → unix milliseconds.
		if submatch[2] == "" {
			return strconv.FormatInt(t.UnixMilli(), 10)
		}

		// :date with optional format specifier.
		switch submatch[3] {
		case "", "iso":
			return t.Format(_isoMillisFormat)
		case "seconds":
			return strconv.FormatInt(t.Unix(), 10)
		default:
			return t.Format(toGoTimeFormat(submatch[3]))
		}
	})

	// Bare $__from/$__to (no braces) → unix milliseconds.
	q = strings.ReplaceAll(q, "$__from", strconv.FormatInt(tr.From.UnixMilli(), 10))
	q = strings.ReplaceAll(q, "$__to", strconv.FormatInt(tr.To.UnixMilli(), 10))
	q = strings.ReplaceAll(q, "$__interval", formatInterval(tr.calculateInterval()))

	return q
}

// toGoTimeFormat converts a date format string (e.g., "YYYY-MM-DD") to a Go time layout string.
func toGoTimeFormat(f string) string {
	return _timeFormatReplacer.Replace(f)
}

// formatInterval formats a duration as an interval string.
func formatInterval(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < _hoursPerDay*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/_hoursPerDay))
	}
}

// roundInterval rounds the given duration to an interval value
// that makes sense for Prometheus queries.
func roundInterval(d time.Duration) time.Duration {
	intervals := []time.Duration{
		15 * time.Second,
		30 * time.Second,
		time.Minute,
		2 * time.Minute,
		5 * time.Minute,
		10 * time.Minute,
		15 * time.Minute,
		30 * time.Minute,
		time.Hour,
		2 * time.Hour,
		6 * time.Hour,
		12 * time.Hour,
		24 * time.Hour,
	}

	for _, interval := range intervals {
		if d <= interval {
			return interval
		}
	}

	return _maxInterval
}
