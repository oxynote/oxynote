package processor

import (
	"context"
	"fmt"
	"hash/fnv"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

const (
	// _defaultLogsLimit is how many lines a page holds when the caller
	// names no limit.
	_defaultLogsLimit = 100

	// _maxLogsLimit caps the lines one page may ask for.
	_maxLogsLimit = 1000

	// _maxLogFieldValues caps the values returned for one field.
	_maxLogFieldValues = 1000

	// _logLevelLabel names the label the log volume series are split by.
	_logLevelLabel = "level"

	// _maxTimestampGroup caps the lines read for one timestamp whose
	// group fills a whole page. It is Loki's default per-query cap, the
	// lowest of the log sources.
	_maxTimestampGroup = 5000

	// _limitKey names the page size in query params.
	_limitKey = "limit"

	// _cursorKey names the cursor in query params.
	_cursorKey = "cursor"
)

// All available LogLevel constants.
const (
	LogLevelCritical LogLevel = "critical"
	LogLevelError    LogLevel = "error"
	LogLevelWarning  LogLevel = "warning"
	LogLevelInfo     LogLevel = "info"
	LogLevelDebug    LogLevel = "debug"
	LogLevelTrace    LogLevel = "trace"
	LogLevelUnknown  LogLevel = "unknown"
)

const (
	// LogFieldKindLabel is a name the source indexes, such as a Loki
	// stream label, which can narrow a stream selector.
	LogFieldKindLabel LogFieldKind = "label"

	// LogFieldKindField is any other name a line carries.
	LogFieldKindField LogFieldKind = "field"
)

var (
	// ErrInvalidLogsLimit is returned when a logs page asks for a limit
	// outside 1 to _maxLogsLimit.
	ErrInvalidLogsLimit = errutil.New(http.StatusBadRequest, "limit.invalid", "limit must be a whole number from 1 to 1000")

	// ErrInvalidCursor is returned when a cursor was not produced by a
	// logs response.
	ErrInvalidCursor = errutil.New(http.StatusBadRequest, "cursor.invalid", "cursor must come from a previous logs response")

	// ErrInvalidLogLevel is returned when a log level is not one of the
	// normalized levels.
	ErrInvalidLogLevel = errutil.New(http.StatusBadRequest, "log_level.invalid", "log level must be critical, error, warning, info, debug, trace or unknown")

	// ErrInvalidLogFieldKind is returned when a log field kind is
	// neither label nor field.
	ErrInvalidLogFieldKind = errutil.New(http.StatusBadRequest, "log_field_kind.invalid", "log field kind must be label or field")
)

// _cursorUnits maps a cursor's text prefix to the precision its end
// counts in.
var _cursorUnits = map[string]time.Duration{
	"ns": time.Nanosecond,
	"ms": time.Millisecond,
}

// _logLevelNames maps the level names sources write, lowercased, to the
// normalized level. OpenTelemetry severity numbers are included because
// some sources store them as text.
var _logLevelNames = map[string]LogLevel{
	"crit":          LogLevelCritical,
	"critical":      LogLevelCritical,
	"fatal":         LogLevelCritical,
	"panic":         LogLevelCritical,
	"emerg":         LogLevelCritical,
	"emergency":     LogLevelCritical,
	"alert":         LogLevelCritical,
	"err":           LogLevelError,
	"error":         LogLevelError,
	"eror":          LogLevelError,
	"severe":        LogLevelError,
	"warn":          LogLevelWarning,
	"warning":       LogLevelWarning,
	"info":          LogLevelInfo,
	"information":   LogLevelInfo,
	"informational": LogLevelInfo,
	"notice":        LogLevelInfo,
	"debug":         LogLevelDebug,
	"dbug":          LogLevelDebug,
	"trace":         LogLevelTrace,
}

// _otelSeverityLevels holds the level of each OpenTelemetry severity
// number range, in steps of four starting at 1.
var _otelSeverityLevels = []LogLevel{
	LogLevelTrace,
	LogLevelDebug,
	LogLevelInfo,
	LogLevelWarning,
	LogLevelError,
	LogLevelCritical,
}

// LogLevel is a line's severity, normalized to the names Grafana uses so
// that colours match what users know.
type LogLevel string

// newLogLevel returns the normalized level of a level name a source
// wrote. Names are matched case-insensitively; anything unknown is
// LogLevelUnknown.
func newLogLevel(raw string) LogLevel {
	name := strings.ToLower(strings.TrimSpace(raw))

	if lvl, ok := _logLevelNames[name]; ok {
		return lvl
	}

	n, err := strconv.Atoi(name)
	if err != nil || n < 1 || n > 4*len(_otelSeverityLevels) {
		return LogLevelUnknown
	}

	return _otelSeverityLevels[(n-1)/4]
}

// Validate reports whether the level is one of the normalized levels.
func (ll LogLevel) Validate() error {
	switch ll {
	case LogLevelCritical, LogLevelError, LogLevelWarning, LogLevelInfo, LogLevelDebug, LogLevelTrace, LogLevelUnknown:
		return nil
	default:
		return ErrInvalidLogLevel
	}
}

// MarshalText returns the level as text.
func (ll LogLevel) MarshalText() ([]byte, error) {
	return []byte(ll), nil
}

// UnmarshalText parses a normalized level, refusing anything else so a
// bad value is reported where it is decoded.
func (ll *LogLevel) UnmarshalText(text []byte) error {
	v := LogLevel(strings.ToLower(strings.TrimSpace(string(text))))
	if err := v.Validate(); err != nil {
		return err
	}

	*ll = v

	return nil
}

// LogFieldKind says whether a field name is indexed by the source.
type LogFieldKind string

// Validate reports whether the kind is label or field.
func (lfk LogFieldKind) Validate() error {
	switch lfk {
	case LogFieldKindLabel, LogFieldKindField:
		return nil
	default:
		return ErrInvalidLogFieldKind
	}
}

// MarshalText returns the kind as text.
func (lfk LogFieldKind) MarshalText() ([]byte, error) {
	return []byte(lfk), nil
}

// UnmarshalText parses a kind, refusing anything but label and field.
func (lfk *LogFieldKind) UnmarshalText(text []byte) error {
	v := LogFieldKind(strings.ToLower(strings.TrimSpace(string(text))))
	if err := v.Validate(); err != nil {
		return err
	}

	*lfk = v

	return nil
}

// Cursor marks where the next, older page of lines starts. It is the
// exclusive end of the window that page reads, counted in the precision
// of the source's timestamps.
type Cursor struct {
	// end is the exclusive end, in units since the epoch.
	end int64

	// unit is the precision end counts in.
	unit time.Duration
}

// newCursor creates a cursor ending at t, counted in unit.
func newCursor(t time.Time, unit time.Duration) Cursor {
	return Cursor{
		end:  t.UnixNano() / int64(unit),
		unit: unit,
	}
}

// endTime returns the instant the next page ends at, exclusive.
func (c Cursor) endTime() time.Time {
	return time.Unix(0, c.end*int64(c.unit)).UTC()
}

// MarshalText returns the cursor as "<unit>:<end>".
func (c Cursor) MarshalText() ([]byte, error) {
	for prefix, unit := range _cursorUnits {
		if unit == c.unit {
			return []byte(prefix + ":" + strconv.FormatInt(c.end, 10)), nil
		}
	}

	return nil, ErrInvalidCursor
}

// UnmarshalText parses a cursor a logs response produced.
func (c *Cursor) UnmarshalText(text []byte) error {
	prefix, rawEnd, ok := strings.Cut(string(text), ":")
	if !ok {
		return ErrInvalidCursor
	}

	unit, ok := _cursorUnits[prefix]
	if !ok {
		return ErrInvalidCursor
	}

	end, err := strconv.ParseInt(rawEnd, 10, 64)
	if err != nil || end < 0 {
		return ErrInvalidCursor
	}

	*c = Cursor{
		end:  end,
		unit: unit,
	}

	return nil
}

// LogsPage is which page of a window's lines a caller wants.
type LogsPage struct {
	// Limit specifies the most lines the page holds.
	Limit int

	// Cursor specifies where the page starts. Without one the page is
	// the newest of the window.
	Cursor null.Value[Cursor]
}

// ParseLogsPage parses a LogsPage from URL values. A missing limit is
// _defaultLogsLimit.
func ParseLogsPage(v url.Values) (LogsPage, error) {
	page := LogsPage{Limit: _defaultLogsLimit}

	if raw := v.Get(_limitKey); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > _maxLogsLimit {
			return LogsPage{}, ErrInvalidLogsLimit
		}

		page.Limit = limit
	}

	if raw := v.Get(_cursorKey); raw != "" {
		var c Cursor

		if err := c.UnmarshalText([]byte(raw)); err != nil {
			return LogsPage{}, err
		}

		page.Cursor = null.ValueFrom(c)
	}

	return page, nil
}

// LogLine is one line in the shape every log source is normalized to.
type LogLine struct {
	// ID specifies a key for the line that stays the same when the same
	// window is read again.
	ID string `json:"id"`

	// Time specifies when the line was written, in UTC.
	Time time.Time `json:"time"`

	// Level specifies the line's normalized severity.
	Level LogLevel `json:"level"`

	// Line specifies the message itself.
	Line string `json:"line"`

	// Labels specifies what the source indexes the line by, such as Loki
	// stream labels.
	Labels map[string]string `json:"labels"`

	// Fields specifies everything else the line carries, flattened, as
	// text.
	Fields map[string]string `json:"fields"`
}

// LogsResult is one page of a window's lines, newest first.
type LogsResult struct {
	// Status specifies whether the page holds any line.
	Status QueryStatus `json:"status"`

	// Lines specifies the page's lines, newest first.
	Lines []LogLine `json:"lines"`

	// Cursor specifies where the next, older page starts. It is null when
	// the window holds nothing older.
	Cursor null.Value[Cursor] `json:"cursor"`

	// Partial indicates that lines may be missing from the page, because
	// the source or a cap cut them.
	Partial bool `json:"partial"`
}

// HasData reports whether the page holds any line.
func (lr *LogsResult) HasData() bool {
	return lr != nil && len(lr.Lines) > 0
}

// LogField is a field name a source's lines carry.
type LogField struct {
	// Name specifies the field's name.
	Name string `json:"name"`

	// Kind specifies whether the source indexes the field.
	Kind LogFieldKind `json:"kind"`
}

// LogFieldsResult is the field names a source's lines carry in a window.
type LogFieldsResult struct {
	// Fields specifies the names, sorted.
	Fields []LogField `json:"fields"`
}

// HasData reports whether the result holds any field.
func (lfr *LogFieldsResult) HasData() bool {
	return lfr != nil && len(lfr.Fields) > 0
}

// LogFieldValue is one value of a field.
type LogFieldValue struct {
	// Value specifies the value.
	Value string `json:"value"`

	// Count specifies how many lines carry the value. It is null when the
	// source cannot count it cheaply.
	Count null.Int `json:"count"`
}

// LogFieldValuesResult is the values one field takes in a window.
type LogFieldValuesResult struct {
	// Values specifies the values.
	Values []LogFieldValue `json:"values"`
}

// HasData reports whether the result holds any value.
func (lvr *LogFieldValuesResult) HasData() bool {
	return lvr != nil && len(lvr.Values) > 0
}

// NewLogFieldValuesResult creates the result holding the given values, in
// their order, at most _maxLogFieldValues of them, none counted.
func NewLogFieldValuesResult(values []string) *LogFieldValuesResult {
	values = values[:min(len(values), _maxLogFieldValues)]
	res := &LogFieldValuesResult{Values: make([]LogFieldValue, 0, len(values))}

	for _, v := range values {
		res.Values = append(res.Values, LogFieldValue{Value: v, Count: null.Int{}})
	}

	return res
}

// LogVolume counts lines per level and per time bucket, a bucket named by
// the unix second it starts at.
type LogVolume map[LogLevel]map[int64]float64

// Add counts n lines of the level in the bucket starting at t.
func (lv LogVolume) Add(lvl LogLevel, t int64, n float64) {
	if lv[lvl] == nil {
		lv[lvl] = make(map[int64]float64)
	}

	lv[lvl][t] += n
}

// Result returns the counts as one series per level, labelled with the
// level, levels and buckets in order. No count at all is no data.
func (lv LogVolume) Result() *QueryResult {
	if len(lv) == 0 {
		return &QueryResult{Status: QueryStatusNoData}
	}

	series := make([]QueryResultSeries, 0, len(lv))

	for _, lvl := range slices.Sorted(maps.Keys(lv)) {
		points := make([][2]any, 0, len(lv[lvl]))

		for _, t := range slices.Sorted(maps.Keys(lv[lvl])) {
			points = append(points, [2]any{t, lv[lvl][t]})
		}

		series = append(series, QueryResultSeries{
			Labels:  map[string]string{_logLevelLabel: string(lvl)},
			Metrics: points,
		})
	}

	return &QueryResult{
		Status: QueryStatusOK,
		Data:   series,
	}
}

// PageLogs reads one page of the window's lines through fetch.
//
// A page never splits the lines of one timestamp: the source has no
// tiebreaker that survives between two requests, so a page asks for one
// line more than it returns, drops every line at its oldest timestamp,
// and the next page starts with that whole group. A page whose lines all
// share one timestamp reads that timestamp alone, up to
// _maxTimestampGroup lines. unit is the precision of the source's
// timestamps, which is the step between one window end and the next.
func PageLogs(
	ctx context.Context,
	fetch LogsFetcher,
	tr TimeRange,
	page LogsPage,
	unit time.Duration,
) (*LogsResult, error) {
	end := tr.To

	if page.Cursor.Valid && page.Cursor.V.endTime().Before(end) {
		end = page.Cursor.V.endTime()
	}

	lines, err := fetch(ctx, tr.From, end, page.Limit+1)
	if err != nil {
		return nil, err
	}

	res := &LogsResult{
		Status: QueryStatusOK,
		Lines:  lines,
	}

	if len(lines) <= page.Limit {
		return res.withLineIDs(), nil
	}

	oldest := lines[len(lines)-1].Time
	next := oldest.Add(unit)

	if lines[0].Time.Equal(oldest) {
		lines, err = fetch(ctx, oldest, next, _maxTimestampGroup)
		if err != nil {
			return nil, err
		}

		res.Lines = lines
		res.Partial = len(lines) >= _maxTimestampGroup
		next = oldest
	} else {
		res.Lines = slices.DeleteFunc(lines, func(l LogLine) bool {
			return l.Time.Equal(oldest)
		})
	}

	if next.After(tr.From) {
		res.Cursor = null.ValueFrom(newCursor(next, unit))
	}

	return res.withLineIDs(), nil
}

// withLineIDs sets every line's ID and the status, and returns the
// result. Identical lines in one page get a counter so that no two IDs
// are equal.
func (lr *LogsResult) withLineIDs() *LogsResult {
	if len(lr.Lines) == 0 {
		lr.Status = QueryStatusNoData
		lr.Lines = []LogLine{}

		return lr
	}

	seen := make(map[string]int, len(lr.Lines))

	for i := range lr.Lines {
		id := lr.Lines[i].hash()

		if n := seen[id]; n > 0 {
			lr.Lines[i].ID = id + "-" + strconv.Itoa(n)
		} else {
			lr.Lines[i].ID = id
		}

		seen[id]++
	}

	return lr
}

// hash returns a hex digest of everything that identifies the line. An
// empty part separates the labels from the fields, so a label and a
// field of the same name and value hash differently.
func (ll LogLine) hash() string {
	parts := slices.Concat(
		[]string{strconv.FormatInt(ll.Time.UnixNano(), 10)},
		sortedPairs(ll.Labels),
		[]string{""},
		sortedPairs(ll.Fields),
		[]string{ll.Line},
	)

	h := fnv.New64a()
	h.Write([]byte(strings.Join(parts, "\x00"))) //nolint:gosec // hash writes never fail

	return fmt.Sprintf("%016x", h.Sum64())
}

// sortedPairs returns the map's pairs as "key=value", in key order.
func sortedPairs(m map[string]string) []string {
	pairs := make([]string, 0, len(m))

	for _, k := range slices.Sorted(maps.Keys(m)) {
		pairs = append(pairs, k+"="+m[k])
	}

	return pairs
}

// LogsFetcher reads the newest lines of [from, end), newest first, at
// most limit of them.
type LogsFetcher func(ctx context.Context, from, end time.Time, limit int) ([]LogLine, error)
