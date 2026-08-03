package processor

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/oxynote/purse/util/errutil"
	"github.com/oxynote/purse/util/timeutil"
)

const (
	// _maxDataPoints defines the default maximum number of data points for interval calculations.
	_maxDataPoints = 100

	// _minQueryStep defines the default step duration for range queries.
	_minQueryStep = time.Second * 15
)

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
	from, err := time.Parse(time.RFC3339, v.Get("from"))
	if err != nil {
		if !fromOptional || v.Get("from") != "" {
			return nil, errutil.New(http.StatusBadRequest, "from.invalid", "From parameter must be a valid RFC3339 timestamp.")
		}

		from = time.Time{}
	}

	to := timeutil.Now()
	rawTo := v.Get("to")

	if rawTo != "" {
		to, err = time.Parse(time.RFC3339, rawTo)
		if err != nil {
			return nil, errutil.New(http.StatusBadRequest, "to.invalid", "To parameter must be a valid RFC3339 timestamp.")
		}
	}

	return &TimeRange{
		From: from,
		To:   to,
	}, nil
}

// QueryStep calculates the step duration for range queries based on the time range.
func (tr TimeRange) QueryStep() time.Duration {
	return max(tr.calculateInterval(), _minQueryStep)
}

// Normalize adjusts the From and To times to align with the specified step duration.
func (tr TimeRange) Normalize() TimeRange {
	step := tr.QueryStep()

	from := tr.From
	to := tr.To

	stepSeconds := int64(step.Seconds())

	if !from.IsZero() {
		from = time.Unix((from.Unix()/stepSeconds)*stepSeconds, 0).UTC()
	}

	if !to.IsZero() {
		to = time.Unix(((to.Unix() + stepSeconds - 1) / stepSeconds * stepSeconds), 0).UTC()
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
		return 15 * time.Second
	}

	interval := duration / time.Duration(_maxDataPoints)

	return roundInterval(interval)
}

// _timeVarRe matches ${__from} and ${__to} with optional :date modifiers.
//
// Examples: ${__from}, ${__from:date}, ${__from:date:iso}, ${__from:date:YYYY-MM}
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
// Source-specific processors should call this after their own macro expansion
// to avoid conflicts (e.g., Prometheus $__rate_interval contains $__interval as a substring).
func (tr TimeRange) ProcessQuery(q string) string {
	// Replace ${__from...} and ${__to...} with modifiers first.
	q = _timeVarRe.ReplaceAllStringFunc(q, func(match string) string {
		submatch := _timeVarRe.FindStringSubmatch(match)

		var t time.Time
		if submatch[1] == "from" {
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
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
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

	return 24 * time.Hour
}

