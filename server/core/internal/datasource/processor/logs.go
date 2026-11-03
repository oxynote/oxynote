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

	// _orderKey names the paging order in query params.
	_orderKey = "order"
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

const (
	// LogsOrderDesc pages through a window newest first.
	LogsOrderDesc LogsOrder = "desc"

	// LogsOrderAsc pages through a window oldest first.
	LogsOrderAsc LogsOrder = "asc"
)

var (
	// ErrInvalidLogsLimit is returned when a logs page asks for a limit
	// outside 1 to _maxLogsLimit.
	ErrInvalidLogsLimit = errutil.New(http.StatusBadRequest, "limit.invalid", "limit must be a whole number from 1 to 1000")

	// ErrInvalidCursor is returned when a cursor was not produced by a
	// logs response.
	ErrInvalidCursor = errutil.New(http.StatusBadRequest, "cursor.invalid", "cursor must come from a previous logs response")

	// ErrInvalidLogsOrder is returned when a logs page asks for an order
	// other than asc or desc.
	ErrInvalidLogsOrder = errutil.New(http.StatusBadRequest, "order.invalid", "order must be asc or desc")

	// ErrInvalidLogLevel is returned when a log level is not one of the
	// normalized levels.
	ErrInvalidLogLevel = errutil.New(http.StatusBadRequest, "log_level.invalid", "log level must be critical, error, warning, info, debug, trace or unknown")

	// ErrInvalidLogFieldKind is returned when a log field kind is
	// neither label nor field.
	ErrInvalidLogFieldKind = errutil.New(http.StatusBadRequest, "log_field_kind.invalid", "log field kind must be label or field")
)

// _cursorUnits maps a cursor's text prefix to the precision its instant
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

// LogsOrder is the order a window's lines are paged through in.
type LogsOrder string

// Validate reports whether the order is asc or desc.
func (lo LogsOrder) Validate() error {
	switch lo {
	case LogsOrderDesc, LogsOrderAsc:
		return nil
	default:
		return ErrInvalidLogsOrder
	}
}

// MarshalText returns the order as text.
func (lo LogsOrder) MarshalText() ([]byte, error) {
	return []byte(lo), nil
}

// UnmarshalText parses an order, refusing anything but asc and desc.
func (lo *LogsOrder) UnmarshalText(text []byte) error {
	v := LogsOrder(strings.ToLower(strings.TrimSpace(string(text))))
	if err := v.Validate(); err != nil {
		return err
	}

	*lo = v

	return nil
}

// SortLines sorts the lines by time, oldest first when asc and newest
// first otherwise. Lines of one timestamp keep their order.
func (lo LogsOrder) SortLines(lines []LogLine) {
	slices.SortStableFunc(lines, func(a, b LogLine) int {
		if lo == LogsOrderAsc {
			return a.Time.Compare(b.Time)
		}

		return b.Time.Compare(a.Time)
	})
}

// Cursor marks where the next page of lines starts, counted in the
// precision of the source's timestamps. Paging newest first it is the
// exclusive end of the window that page reads; paging oldest first it is
// the inclusive start.
type Cursor struct {
	// at is the instant, in units since the epoch.
	at int64

	// unit is the precision at counts in.
	unit time.Duration
}

// newCursor creates a cursor at t, counted in unit.
func newCursor(t time.Time, unit time.Duration) Cursor {
	return Cursor{
		at:   t.UnixNano() / int64(unit),
		unit: unit,
	}
}

// instant returns the instant the next page starts from.
func (c Cursor) instant() time.Time {
	return time.Unix(0, c.at*int64(c.unit)).UTC()
}

// MarshalText returns the cursor as "<unit>:<at>".
func (c Cursor) MarshalText() ([]byte, error) {
	for prefix, unit := range _cursorUnits {
		if unit == c.unit {
			return []byte(prefix + ":" + strconv.FormatInt(c.at, 10)), nil
		}
	}

	return nil, ErrInvalidCursor
}

// UnmarshalText parses a cursor a logs response produced.
func (c *Cursor) UnmarshalText(text []byte) error {
	prefix, rawAt, ok := strings.Cut(string(text), ":")
	if !ok {
		return ErrInvalidCursor
	}

	unit, ok := _cursorUnits[prefix]
	if !ok {
		return ErrInvalidCursor
	}

	at, err := strconv.ParseInt(rawAt, 10, 64)
	if err != nil || at < 0 {
		return ErrInvalidCursor
	}

	*c = Cursor{
		at:   at,
		unit: unit,
	}

	return nil
}

// LogsPage is which page of a window's lines a caller wants.
type LogsPage struct {
	// Limit specifies the most lines the page holds.
	Limit int

	// Cursor specifies where the page starts. Without one the page is
	// the first of the window in the page's order.
	Cursor null.Value[Cursor]

	// Order specifies the order the window is paged through in. The
	// zero value is LogsOrderDesc.
	Order LogsOrder
}

// ParseLogsPage parses a LogsPage from URL values. A missing limit is
// _defaultLogsLimit and a missing order is LogsOrderDesc.
func ParseLogsPage(v url.Values) (LogsPage, error) {
	page := LogsPage{
		Limit: _defaultLogsLimit,
		Order: LogsOrderDesc,
	}

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

	if raw := v.Get(_orderKey); raw != "" {
		if err := page.Order.UnmarshalText([]byte(raw)); err != nil {
			return LogsPage{}, err
		}
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

// LogsResult is one page of a window's lines.
type LogsResult struct {
	// Status specifies whether the page holds any line.
	Status QueryStatus `json:"status"`

	// Lines specifies the page's lines, in the page's order.
	Lines []LogLine `json:"lines"`

	// Cursor specifies where the next page starts. It is null when the
	// window holds nothing past the page.
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

// PageLogs reads one page of the window's lines through fetch, in the
// page's order.
//
// A page never splits the lines of one timestamp: the source has no
// tiebreaker that survives between two requests, so a page asks for one
// line more than it returns, drops every line at its last timestamp, and
// the next page starts with that whole group. A page whose lines all
// share one timestamp reads that timestamp alone, up to
// _maxTimestampGroup lines. unit is the precision of the source's
// timestamps, which is the step a cursor takes past a timestamp.
func PageLogs(
	ctx context.Context,
	fetch LogsFetcher,
	tr TimeRange,
	page LogsPage,
	unit time.Duration,
) (*LogsResult, error) {
	order := page.Order
	if order != LogsOrderAsc {
		order = LogsOrderDesc
	}

	asc := order == LogsOrderAsc
	from, end := tr.From, tr.To

	if page.Cursor.Valid {
		at := page.Cursor.V.instant()

		switch {
		case asc && at.After(from):
			from = at
		case !asc && at.Before(end):
			end = at
		}
	}

	if !from.Before(end) {
		return &LogsResult{
			Status: QueryStatusNoData,
			Lines:  []LogLine{},
		}, nil
	}

	lines, err := fetch(ctx, from, end, page.Limit+1, order)
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

	last := lines[len(lines)-1].Time
	whole := lines[0].Time.Equal(last)

	if whole {
		lines, err = fetch(ctx, last, last.Add(unit), _maxTimestampGroup, order)
		if err != nil {
			return nil, err
		}

		res.Lines = lines
		res.Partial = len(lines) >= _maxTimestampGroup
	} else {
		res.Lines = slices.DeleteFunc(lines, func(l LogLine) bool {
			return l.Time.Equal(last)
		})
	}

	next := last

	// the next page starts with a cut group and skips a whole one. A desc
	// cursor is an exclusive end and an asc one an inclusive start, so the
	// cursor moves one unit past the group when desc keeps it or asc
	// skips it.
	if whole == asc {
		next = last.Add(unit)
	}

	if (asc && next.Before(tr.To)) || (!asc && next.After(tr.From)) {
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

// LogsFetcher reads at most limit lines of [from, end) in the order
// given: the newest lines, newest first, when desc, and the oldest lines,
// oldest first, when asc.
type LogsFetcher func(ctx context.Context, from, end time.Time, limit int, order LogsOrder) ([]LogLine, error)
