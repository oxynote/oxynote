package processor

import (
	"context"
	"net/url"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _logsBase is the instant the paging fixtures start at.
var _logsBase = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

func Test_newLogLevel(t *testing.T) {
	cc := map[string]struct {
		Raw    string
		Result LogLevel
	}{
		"Critical":                  {Raw: "critical", Result: LogLevelCritical},
		"Crit":                      {Raw: "crit", Result: LogLevelCritical},
		"Fatal":                     {Raw: "fatal", Result: LogLevelCritical},
		"Panic":                     {Raw: "panic", Result: LogLevelCritical},
		"Emerg":                     {Raw: "emerg", Result: LogLevelCritical},
		"Emergency":                 {Raw: "emergency", Result: LogLevelCritical},
		"Alert":                     {Raw: "alert", Result: LogLevelCritical},
		"Error":                     {Raw: "error", Result: LogLevelError},
		"Err":                       {Raw: "err", Result: LogLevelError},
		"Eror":                      {Raw: "eror", Result: LogLevelError},
		"Severe":                    {Raw: "severe", Result: LogLevelError},
		"Warn":                      {Raw: "warn", Result: LogLevelWarning},
		"Warning":                   {Raw: "warning", Result: LogLevelWarning},
		"Info":                      {Raw: "info", Result: LogLevelInfo},
		"Information":               {Raw: "information", Result: LogLevelInfo},
		"Informational":             {Raw: "informational", Result: LogLevelInfo},
		"Notice":                    {Raw: "notice", Result: LogLevelInfo},
		"Debug":                     {Raw: "debug", Result: LogLevelDebug},
		"Dbug":                      {Raw: "dbug", Result: LogLevelDebug},
		"Trace":                     {Raw: "trace", Result: LogLevelTrace},
		"Upper case and whitespace": {Raw: "  ERROR ", Result: LogLevelError},
		"Severity number 1":         {Raw: "1", Result: LogLevelTrace},
		"Severity number 4":         {Raw: "4", Result: LogLevelTrace},
		"Severity number 5":         {Raw: "5", Result: LogLevelDebug},
		"Severity number 9":         {Raw: "9", Result: LogLevelInfo},
		"Severity number 13":        {Raw: "13", Result: LogLevelWarning},
		"Severity number 17":        {Raw: "17", Result: LogLevelError},
		"Severity number 21":        {Raw: "21", Result: LogLevelCritical},
		"Severity number 24":        {Raw: "24", Result: LogLevelCritical},
		"Severity number 0":         {Raw: "0", Result: LogLevelUnknown},
		"Severity number 25":        {Raw: "25", Result: LogLevelUnknown},
		"Empty":                     {Raw: "", Result: LogLevelUnknown},
		"Unknown name":              {Raw: "loud", Result: LogLevelUnknown},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, newLogLevel(c.Raw))
		})
	}
}

func Test_LogLevel_Validate(t *testing.T) {
	cc := map[string]struct {
		Level LogLevel
		Err   error
	}{
		"Critical": {Level: LogLevelCritical},
		"Error":    {Level: LogLevelError},
		"Warning":  {Level: LogLevelWarning},
		"Info":     {Level: LogLevelInfo},
		"Debug":    {Level: LogLevelDebug},
		"Trace":    {Level: LogLevelTrace},
		"Unknown":  {Level: LogLevelUnknown},
		"Alias":    {Level: "warn", Err: ErrInvalidLogLevel},
		"Empty":    {Level: "", Err: ErrInvalidLogLevel},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Level.Validate())
		})
	}
}

func Test_LogLevel_MarshalText(t *testing.T) {
	text, err := LogLevelWarning.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "warning", string(text))
}

func Test_LogLevel_UnmarshalText(t *testing.T) {
	cc := map[string]struct {
		Text   string
		Result LogLevel
		Err    error
	}{
		"Valid level":               {Text: "error", Result: LogLevelError},
		"Upper case and whitespace": {Text: " TRACE ", Result: LogLevelTrace},
		"Alias":                     {Text: "warn", Err: ErrInvalidLogLevel},
		"Empty":                     {Text: "", Err: ErrInvalidLogLevel},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var ll LogLevel

			err := ll.UnmarshalText([]byte(c.Text))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, ll)
		})
	}
}

func Test_LogFieldKind_Validate(t *testing.T) {
	cc := map[string]struct {
		Kind LogFieldKind
		Err  error
	}{
		"Label":   {Kind: LogFieldKindLabel},
		"Field":   {Kind: LogFieldKindField},
		"Unknown": {Kind: "column", Err: ErrInvalidLogFieldKind},
		"Empty":   {Kind: "", Err: ErrInvalidLogFieldKind},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Kind.Validate())
		})
	}
}

func Test_LogFieldKind_MarshalText(t *testing.T) {
	text, err := LogFieldKindLabel.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "label", string(text))
}

func Test_LogFieldKind_UnmarshalText(t *testing.T) {
	cc := map[string]struct {
		Text   string
		Result LogFieldKind
		Err    error
	}{
		"Valid kind":                {Text: "field", Result: LogFieldKindField},
		"Upper case and whitespace": {Text: " LABEL ", Result: LogFieldKindLabel},
		"Unknown":                   {Text: "column", Err: ErrInvalidLogFieldKind},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var lfk LogFieldKind

			err := lfk.UnmarshalText([]byte(c.Text))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, lfk)
		})
	}
}

func Test_LogsOrder_Validate(t *testing.T) {
	cc := map[string]struct {
		Order LogsOrder
		Err   error
	}{
		"Desc":    {Order: LogsOrderDesc},
		"Asc":     {Order: LogsOrderAsc},
		"Unknown": {Order: "sideways", Err: ErrInvalidLogsOrder},
		"Empty":   {Order: "", Err: ErrInvalidLogsOrder},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Order.Validate())
		})
	}
}

func Test_LogsOrder_MarshalText(t *testing.T) {
	text, err := LogsOrderAsc.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "asc", string(text))
}

func Test_LogsOrder_UnmarshalText(t *testing.T) {
	cc := map[string]struct {
		Text   string
		Result LogsOrder
		Err    error
	}{
		"Desc":                      {Text: "desc", Result: LogsOrderDesc},
		"Asc":                       {Text: "asc", Result: LogsOrderAsc},
		"Upper case and whitespace": {Text: " ASC ", Result: LogsOrderAsc},
		"Loki direction":            {Text: "forward", Err: ErrInvalidLogsOrder},
		"Unknown":                   {Text: "sideways", Err: ErrInvalidLogsOrder},
		"Empty":                     {Text: "", Err: ErrInvalidLogsOrder},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var lo LogsOrder

			err := lo.UnmarshalText([]byte(c.Text))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, lo)
		})
	}
}

func Test_LogsOrder_SortLines(t *testing.T) {
	at := func(offset time.Duration, line string) LogLine {
		return LogLine{Time: _logsBase.Add(offset), Line: line}
	}

	lines := []LogLine{at(time.Second, "b"), at(2*time.Second, "c"), at(0, "a"), at(time.Second, "d")}

	cc := map[string]struct {
		Order  LogsOrder
		Result []LogLine
	}{
		"Desc": {
			Order:  LogsOrderDesc,
			Result: []LogLine{at(2*time.Second, "c"), at(time.Second, "b"), at(time.Second, "d"), at(0, "a")},
		},
		"Asc": {
			Order:  LogsOrderAsc,
			Result: []LogLine{at(0, "a"), at(time.Second, "b"), at(time.Second, "d"), at(2*time.Second, "c")},
		},
		"Zero value sorts desc": {
			Result: []LogLine{at(2*time.Second, "c"), at(time.Second, "b"), at(time.Second, "d"), at(0, "a")},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			sorted := slices.Clone(lines)
			c.Order.SortLines(sorted)
			assert.Equal(t, c.Result, sorted)
		})
	}
}

func Test_newCursor(t *testing.T) {
	cc := map[string]struct {
		Time   time.Time
		Unit   time.Duration
		Result Cursor
	}{
		"Nanoseconds": {
			Time:   time.Unix(0, 1793019458693281911),
			Unit:   time.Nanosecond,
			Result: Cursor{at: 1793019458693281911, unit: time.Nanosecond},
		},
		"Milliseconds": {
			Time:   time.Unix(0, 1793019458693000000),
			Unit:   time.Millisecond,
			Result: Cursor{at: 1793019458693, unit: time.Millisecond},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, newCursor(c.Time, c.Unit))
		})
	}
}

func Test_Cursor_instant(t *testing.T) {
	cc := map[string]struct {
		Cursor Cursor
		Result time.Time
	}{
		"Nanoseconds": {
			Cursor: Cursor{at: 1793019458693281911, unit: time.Nanosecond},
			Result: time.Unix(0, 1793019458693281911).UTC(),
		},
		"Milliseconds": {
			Cursor: Cursor{at: 1793019458693, unit: time.Millisecond},
			Result: time.Unix(0, 1793019458693000000).UTC(),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Cursor.instant())
		})
	}
}

func Test_Cursor_MarshalText(t *testing.T) {
	cc := map[string]struct {
		Cursor Cursor
		Result string
		Err    error
	}{
		"Nanoseconds":  {Cursor: Cursor{at: 42, unit: time.Nanosecond}, Result: "ns:42"},
		"Milliseconds": {Cursor: Cursor{at: 42, unit: time.Millisecond}, Result: "ms:42"},
		"Unknown unit": {Cursor: Cursor{at: 42, unit: time.Second}, Err: ErrInvalidCursor},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			text, err := c.Cursor.MarshalText()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, string(text))
		})
	}
}

func Test_Cursor_UnmarshalText(t *testing.T) {
	cc := map[string]struct {
		Text   string
		Result Cursor
		Err    error
	}{
		"Nanoseconds":      {Text: "ns:1793019458693281911", Result: Cursor{at: 1793019458693281911, unit: time.Nanosecond}},
		"Milliseconds":     {Text: "ms:1793019458693", Result: Cursor{at: 1793019458693, unit: time.Millisecond}},
		"No separator":     {Text: "1793019458693281911", Err: ErrInvalidCursor},
		"Unknown unit":     {Text: "s:1793019458", Err: ErrInvalidCursor},
		"Not a number":     {Text: "ns:soon", Err: ErrInvalidCursor},
		"Negative instant": {Text: "ns:-1", Err: ErrInvalidCursor},
		"Empty instant":    {Text: "ns:", Err: ErrInvalidCursor},
		"Empty text":       {Text: "", Err: ErrInvalidCursor},
		"Trailing space":   {Text: "ns:42 ", Err: ErrInvalidCursor},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var cur Cursor

			err := cur.UnmarshalText([]byte(c.Text))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, cur)
		})
	}
}

func Test_ParseLogsPage(t *testing.T) {
	cc := map[string]struct {
		Values url.Values
		Result LogsPage
		Err    error
	}{
		"Defaults": {
			Values: url.Values{},
			Result: LogsPage{Limit: _defaultLogsLimit, Order: LogsOrderDesc},
		},
		"Limit and cursor": {
			Values: url.Values{"limit": {"7"}, "cursor": {"ns:42"}},
			Result: LogsPage{
				Limit:  7,
				Cursor: null.ValueFrom(Cursor{at: 42, unit: time.Nanosecond}),
				Order:  LogsOrderDesc,
			},
		},
		"Largest limit": {
			Values: url.Values{"limit": {"1000"}},
			Result: LogsPage{Limit: 1000, Order: LogsOrderDesc},
		},
		"Asc": {
			Values: url.Values{"order": {"asc"}},
			Result: LogsPage{Limit: _defaultLogsLimit, Order: LogsOrderAsc},
		},
		"Desc": {
			Values: url.Values{"order": {"desc"}},
			Result: LogsPage{Limit: _defaultLogsLimit, Order: LogsOrderDesc},
		},
		"Invalid order": {
			Values: url.Values{"order": {"sideways"}},
			Err:    ErrInvalidLogsOrder,
		},
		"Zero limit": {
			Values: url.Values{"limit": {"0"}},
			Err:    ErrInvalidLogsLimit,
		},
		"Limit over the cap": {
			Values: url.Values{"limit": {"1001"}},
			Err:    ErrInvalidLogsLimit,
		},
		"Limit not a number": {
			Values: url.Values{"limit": {"many"}},
			Err:    ErrInvalidLogsLimit,
		},
		"Invalid cursor": {
			Values: url.Values{"cursor": {"later"}},
			Err:    ErrInvalidCursor,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			page, err := ParseLogsPage(c.Values)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, page)
		})
	}
}

func Test_LogsResult_HasData(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Result *LogsResult
		Has    bool
	}{
		"No result": {},
		"No lines":  {Result: &LogsResult{Status: QueryStatusNoData}},
		"A line":    {Result: &LogsResult{Status: QueryStatusOK, Lines: []LogLine{{Line: "started"}}}, Has: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Has, c.Result.HasData())
		})
	}
}

func Test_LogFieldsResult_HasData(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Result *LogFieldsResult
		Has    bool
	}{
		"No result": {},
		"No fields": {Result: &LogFieldsResult{}},
		"A field":   {Result: &LogFieldsResult{Fields: []LogField{{Name: "app"}}}, Has: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Has, c.Result.HasData())
		})
	}
}

func Test_LogFieldValuesResult_HasData(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Result *LogFieldValuesResult
		Has    bool
	}{
		"No result": {},
		"No values": {Result: &LogFieldValuesResult{}},
		"A value":   {Result: &LogFieldValuesResult{Values: []LogFieldValue{{Value: "api"}}}, Has: true},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Has, c.Result.HasData())
		})
	}
}

func Test_NewLogFieldValuesResult(t *testing.T) {
	many := make([]string, _maxLogFieldValues+5)
	for i := range many {
		many[i] = strconv.Itoa(i)
	}

	cc := map[string]struct {
		Values []string
		Len    int
		First  string
	}{
		"No values":           {Values: nil, Len: 0},
		"Values keep order":   {Values: []string{"b", "a"}, Len: 2, First: "b"},
		"Values past the cap": {Values: many, Len: _maxLogFieldValues, First: "0"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res := NewLogFieldValuesResult(c.Values)
			require.NotNil(t, res.Values)
			require.Len(t, res.Values, c.Len)

			for _, v := range res.Values {
				assert.False(t, v.Count.Valid)
			}

			if c.Len > 0 {
				assert.Equal(t, c.First, res.Values[0].Value)
			}
		})
	}
}

func Test_LogVolume_Add(t *testing.T) {
	t.Parallel()

	lv := LogVolume{}
	lv.Add(LogLevelError, 60, 2)
	lv.Add(LogLevelError, 60, 3)
	lv.Add(LogLevelInfo, 0, 1)

	assert.Equal(t, LogVolume{
		LogLevelError: {60: 5},
		LogLevelInfo:  {0: 1},
	}, lv)
}

func Test_LogVolume_Result(t *testing.T) {
	cc := map[string]struct {
		Volume LogVolume
		First  int64
		Last   int64
		Step   int64
		Result *QueryResult
	}{
		"No counts": {
			Volume: LogVolume{},
			First:  0,
			Last:   180,
			Step:   60,
			Result: &QueryResult{
				Status: QueryStatusNoData,
				Data: []QueryResultSeries{
					{Labels: map[string]string{}, Metrics: [][2]any{{int64(0), float64(0)}, {int64(60), float64(0)}, {int64(120), float64(0)}, {int64(180), float64(0)}}},
				},
			},
		},
		"Levels in order with every bucket and zero where no line fell": {
			Volume: LogVolume{
				LogLevelWarning: {120: 1, 60: 4},
				LogLevelError:   {60: 2},
			},
			First: 0,
			Last:  180,
			Step:  60,
			Result: &QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{Labels: map[string]string{"level": "error"}, Metrics: [][2]any{{int64(0), float64(0)}, {int64(60), float64(2)}, {int64(120), float64(0)}, {int64(180), float64(0)}}},
					{Labels: map[string]string{"level": "warning"}, Metrics: [][2]any{{int64(0), float64(0)}, {int64(60), float64(4)}, {int64(120), float64(1)}, {int64(180), float64(0)}}},
				},
			},
		},
		"Range shorter than a step": {
			Volume: LogVolume{
				LogLevelInfo: {60: 1},
			},
			First: 60,
			Last:  0,
			Step:  60,
			Result: &QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{Labels: map[string]string{"level": "info"}, Metrics: [][2]any{}},
				},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Volume.Result(c.First, c.Last, c.Step))
		})
	}
}

func Test_volumePoints(t *testing.T) {
	cc := map[string]struct {
		Counts map[int64]float64
		First  int64
		Last   int64
		Step   int64
		Result [][2]any
	}{
		"Every bucket with its count or zero": {
			Counts: map[int64]float64{60: 2},
			First:  0,
			Last:   120,
			Step:   60,
			Result: [][2]any{{int64(0), float64(0)}, {int64(60), float64(2)}, {int64(120), float64(0)}},
		},
		"No counts": {
			First:  0,
			Last:   60,
			Step:   60,
			Result: [][2]any{{int64(0), float64(0)}, {int64(60), float64(0)}},
		},
		"Range shorter than a step": {
			Counts: map[int64]float64{60: 1},
			First:  60,
			Last:   0,
			Step:   60,
			Result: [][2]any{},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, volumePoints(c.Counts, c.First, c.Last, c.Step))
		})
	}
}

func Test_PageLogs(t *testing.T) {
	window := TimeRange{
		From: _logsBase,
		To:   _logsBase.Add(time.Hour),
	}

	at := func(offset time.Duration, seq int) LogLine {
		return LogLine{
			Time:   _logsBase.Add(offset),
			Level:  LogLevelInfo,
			Line:   "line " + strconv.Itoa(seq),
			Labels: map[string]string{"service_name": "checkout"},
			Fields: map[string]string{"seq": strconv.Itoa(seq)},
		}
	}

	// the traps of the shared corpus: one timestamp holding more lines
	// than any page, groups of four sharing a timestamp, and pairs one
	// nanosecond apart.
	traps := make([]LogLine, 0, 1020)

	for i := range 600 {
		traps = append(traps, at(time.Duration(i)*time.Second, len(traps)))
	}

	for range 300 {
		traps = append(traps, at(300*time.Second+500*time.Millisecond, len(traps)))
	}

	for g := range 20 {
		for range 4 {
			traps = append(traps, at(time.Duration(g*13)*time.Second+250*time.Millisecond, len(traps)))
		}
	}

	for g := range 20 {
		offset := time.Duration(g*17)*time.Second + 750*time.Millisecond
		traps = append(traps, at(offset, len(traps)), at(offset+time.Nanosecond, len(traps)+1))
	}

	// three lines a second apart, then two sharing the oldest timestamp.
	small := []LogLine{
		at(4*time.Second, 4),
		at(3*time.Second, 3),
		at(2*time.Second, 2),
		at(time.Second, 1),
		at(time.Second, 0),
	}

	// three lines a second apart, then two sharing the newest timestamp.
	early := []LogLine{
		at(time.Second, 0),
		at(2*time.Second, 1),
		at(3*time.Second, 2),
		at(4*time.Second, 3),
		at(4*time.Second, 4),
	}

	asc := func(limit int, cursor, unit time.Duration) LogsPage {
		page := LogsPage{Limit: limit, Order: LogsOrderAsc}

		if cursor > 0 {
			page.Cursor = null.ValueFrom(newCursor(_logsBase.Add(cursor), unit))
		}

		return page
	}

	type tcase struct {
		Lines     []LogLine
		FetchErrs []error
		Range     TimeRange
		Page      LogsPage
		Unit      time.Duration
		WalkLimit int
		Result    *LogsResult
		Err       error
	}

	cc := map[string]tcase{
		"Error returned by the page fetch": {
			Lines:     small,
			FetchErrs: []error{assert.AnError},
			Range:     window,
			Page:      LogsPage{Limit: 2},
			Unit:      time.Nanosecond,
			Err:       assert.AnError,
		},
		"Error returned by the timestamp group fetch": {
			Lines:     []LogLine{at(time.Second, 1), at(time.Second, 0)},
			FetchErrs: []error{nil, assert.AnError},
			Range:     window,
			Page:      LogsPage{Limit: 1},
			Unit:      time.Nanosecond,
			Err:       assert.AnError,
		},
		"Empty window": {
			Range: window,
			Page:  LogsPage{Limit: 2},
			Unit:  time.Nanosecond,
			Result: &LogsResult{
				Status: QueryStatusNoData,
				Lines:  []LogLine{},
			},
		},
		"Fewer lines than the limit": {
			Lines: small,
			Range: window,
			Page:  LogsPage{Limit: 5},
			Unit:  time.Nanosecond,
			Result: &LogsResult{
				Status:         QueryStatusOK,
				Lines:          small,
				PreviousCursor: null.ValueFrom(newCursor(_logsBase.Add(4*time.Second+time.Nanosecond), time.Nanosecond)),
			},
		},
		"Page drops the line past the limit": {
			Lines: small,
			Range: window,
			Page:  LogsPage{Limit: 2},
			Unit:  time.Nanosecond,
			Result: &LogsResult{
				Status:         QueryStatusOK,
				Lines:          small[:2],
				PreviousCursor: null.ValueFrom(newCursor(_logsBase.Add(4*time.Second+time.Nanosecond), time.Nanosecond)),
				NextCursor:     null.ValueFrom(newCursor(_logsBase.Add(2*time.Second+time.Nanosecond), time.Nanosecond)),
			},
		},
		"Page drops the whole oldest timestamp group": {
			Lines: small,
			Range: window,
			Page:  LogsPage{Limit: 4},
			Unit:  time.Nanosecond,
			Result: &LogsResult{
				Status:         QueryStatusOK,
				Lines:          small[:3],
				PreviousCursor: null.ValueFrom(newCursor(_logsBase.Add(4*time.Second+time.Nanosecond), time.Nanosecond)),
				NextCursor:     null.ValueFrom(newCursor(_logsBase.Add(time.Second+time.Nanosecond), time.Nanosecond)),
			},
		},
		"Page of one timestamp reads the whole group": {
			Lines: small,
			Range: window,
			Page: LogsPage{
				Limit:  1,
				Cursor: null.ValueFrom(newCursor(_logsBase.Add(time.Second+time.Nanosecond), time.Nanosecond)),
			},
			Unit: time.Nanosecond,
			Result: &LogsResult{
				Status:         QueryStatusOK,
				Lines:          small[3:],
				PreviousCursor: null.ValueFrom(newCursor(_logsBase.Add(time.Second+time.Nanosecond), time.Nanosecond)),
				NextCursor:     null.ValueFrom(newCursor(_logsBase.Add(time.Second), time.Nanosecond)),
			},
		},
		"Timestamp group after the window start sets the cursor": {
			Lines: []LogLine{at(2*time.Second, 1), at(2*time.Second, 0), at(time.Second, 2)},
			Range: window,
			Page:  LogsPage{Limit: 1},
			Unit:  time.Nanosecond,
			Result: &LogsResult{
				Status:         QueryStatusOK,
				Lines:          []LogLine{at(2*time.Second, 1), at(2*time.Second, 0)},
				PreviousCursor: null.ValueFrom(newCursor(_logsBase.Add(2*time.Second+time.Nanosecond), time.Nanosecond)),
				NextCursor:     null.ValueFrom(newCursor(_logsBase.Add(2*time.Second), time.Nanosecond)),
			},
		},
		"Timestamp group reaching the cap is partial": func() tcase {
			group := make([]LogLine, 0, _maxTimestampGroup+1)

			for i := range _maxTimestampGroup + 1 {
				group = append(group, at(time.Minute, i))
			}

			return tcase{
				Lines: group,
				Range: window,
				Page:  LogsPage{Limit: 10},
				Unit:  time.Nanosecond,
				Result: &LogsResult{
					Status:         QueryStatusOK,
					Lines:          group[:_maxTimestampGroup],
					PreviousCursor: null.ValueFrom(newCursor(_logsBase.Add(time.Minute+time.Nanosecond), time.Nanosecond)),
					NextCursor:     null.ValueFrom(newCursor(_logsBase.Add(time.Minute), time.Nanosecond)),
					Partial:        true,
				},
			}
		}(),
		"Cursor after the window end is ignored": {
			Lines: small,
			Range: TimeRange{From: _logsBase, To: _logsBase.Add(3 * time.Second)},
			Page: LogsPage{
				Limit:  5,
				Cursor: null.ValueFrom(newCursor(_logsBase.Add(time.Hour), time.Nanosecond)),
			},
			Unit: time.Nanosecond,
			Result: &LogsResult{
				Status:         QueryStatusOK,
				Lines:          small[2:],
				PreviousCursor: null.ValueFrom(newCursor(_logsBase.Add(2*time.Second+time.Nanosecond), time.Nanosecond)),
			},
		},
		"Newest group at the window end sets no previous cursor": {
			Lines: small,
			Range: TimeRange{From: _logsBase, To: _logsBase.Add(4*time.Second + time.Nanosecond)},
			Page:  LogsPage{Limit: 5},
			Unit:  time.Nanosecond,
			Result: &LogsResult{
				Status: QueryStatusOK,
				Lines:  small,
			},
		},
		"Millisecond unit steps the cursor by a millisecond": {
			Lines: small,
			Range: window,
			Page:  LogsPage{Limit: 2},
			Unit:  time.Millisecond,
			Result: &LogsResult{
				Status:         QueryStatusOK,
				Lines:          small[:2],
				PreviousCursor: null.ValueFrom(newCursor(_logsBase.Add(4*time.Second+time.Millisecond), time.Millisecond)),
				NextCursor:     null.ValueFrom(newCursor(_logsBase.Add(2*time.Second+time.Millisecond), time.Millisecond)),
			},
		},
		"Walk at limit 1 reads every line once": {
			Lines:     traps,
			Range:     window,
			Unit:      time.Nanosecond,
			WalkLimit: 1,
		},
		"Walk at limit 7 reads every line once": {
			Lines:     traps,
			Range:     window,
			Unit:      time.Nanosecond,
			WalkLimit: 7,
		},
		"Walk at limit 100 reads every line once": {
			Lines:     traps,
			Range:     window,
			Unit:      time.Nanosecond,
			WalkLimit: 100,
		},
		"Explicit desc order pages like the default": {
			Lines: small,
			Range: window,
			Page:  LogsPage{Limit: 2, Order: LogsOrderDesc},
			Unit:  time.Nanosecond,
			Result: &LogsResult{
				Status:         QueryStatusOK,
				Lines:          small[:2],
				PreviousCursor: null.ValueFrom(newCursor(_logsBase.Add(4*time.Second+time.Nanosecond), time.Nanosecond)),
				NextCursor:     null.ValueFrom(newCursor(_logsBase.Add(2*time.Second+time.Nanosecond), time.Nanosecond)),
			},
		},
		"Error returned by the asc timestamp group fetch": {
			Lines:     []LogLine{at(time.Second, 0), at(time.Second, 1)},
			FetchErrs: []error{nil, assert.AnError},
			Range:     window,
			Page:      asc(1, 0, time.Nanosecond),
			Unit:      time.Nanosecond,
			Err:       assert.AnError,
		},
		"Asc with fewer lines than the limit": {
			Lines: early,
			Range: window,
			Page:  asc(5, 0, time.Nanosecond),
			Unit:  time.Nanosecond,
			Result: &LogsResult{
				Status:         QueryStatusOK,
				Lines:          early,
				PreviousCursor: null.ValueFrom(newCursor(_logsBase.Add(time.Second), time.Nanosecond)),
			},
		},
		"Asc page drops the line past the limit": {
			Lines: early,
			Range: window,
			Page:  asc(2, 0, time.Nanosecond),
			Unit:  time.Nanosecond,
			Result: &LogsResult{
				Status:         QueryStatusOK,
				Lines:          early[:2],
				PreviousCursor: null.ValueFrom(newCursor(_logsBase.Add(time.Second), time.Nanosecond)),
				NextCursor:     null.ValueFrom(newCursor(_logsBase.Add(3*time.Second), time.Nanosecond)),
			},
		},
		"Asc page drops the whole newest timestamp group": {
			Lines: early,
			Range: window,
			Page:  asc(4, 0, time.Nanosecond),
			Unit:  time.Nanosecond,
			Result: &LogsResult{
				Status:         QueryStatusOK,
				Lines:          early[:3],
				PreviousCursor: null.ValueFrom(newCursor(_logsBase.Add(time.Second), time.Nanosecond)),
				NextCursor:     null.ValueFrom(newCursor(_logsBase.Add(4*time.Second), time.Nanosecond)),
			},
		},
		"Asc page of one timestamp reads the whole group": {
			Lines: early,
			Range: window,
			Page:  asc(1, 4*time.Second, time.Nanosecond),
			Unit:  time.Nanosecond,
			Result: &LogsResult{
				Status:         QueryStatusOK,
				Lines:          early[3:],
				PreviousCursor: null.ValueFrom(newCursor(_logsBase.Add(4*time.Second), time.Nanosecond)),
				NextCursor:     null.ValueFrom(newCursor(_logsBase.Add(4*time.Second+time.Nanosecond), time.Nanosecond)),
			},
		},
		"Asc timestamp group before the window end sets the cursor": {
			Lines: []LogLine{at(time.Second, 0), at(time.Second, 1), at(2*time.Second, 2)},
			Range: window,
			Page:  asc(1, 0, time.Nanosecond),
			Unit:  time.Nanosecond,
			Result: &LogsResult{
				Status:         QueryStatusOK,
				Lines:          []LogLine{at(time.Second, 0), at(time.Second, 1)},
				PreviousCursor: null.ValueFrom(newCursor(_logsBase.Add(time.Second), time.Nanosecond)),
				NextCursor:     null.ValueFrom(newCursor(_logsBase.Add(time.Second+time.Nanosecond), time.Nanosecond)),
			},
		},
		"Asc timestamp group at the window end sets no cursor": {
			Lines: early,
			Range: TimeRange{From: _logsBase, To: _logsBase.Add(4*time.Second + time.Nanosecond)},
			Page:  asc(1, 4*time.Second, time.Nanosecond),
			Unit:  time.Nanosecond,
			Result: &LogsResult{
				Status:         QueryStatusOK,
				Lines:          early[3:],
				PreviousCursor: null.ValueFrom(newCursor(_logsBase.Add(4*time.Second), time.Nanosecond)),
			},
		},
		"Asc timestamp group reaching the cap is partial": func() tcase {
			group := make([]LogLine, 0, _maxTimestampGroup+1)

			for i := range _maxTimestampGroup + 1 {
				group = append(group, at(time.Minute, i))
			}

			return tcase{
				Lines: group,
				Range: window,
				Page:  asc(10, 0, time.Nanosecond),
				Unit:  time.Nanosecond,
				Result: &LogsResult{
					Status:         QueryStatusOK,
					Lines:          group[:_maxTimestampGroup],
					PreviousCursor: null.ValueFrom(newCursor(_logsBase.Add(time.Minute), time.Nanosecond)),
					NextCursor:     null.ValueFrom(newCursor(_logsBase.Add(time.Minute+time.Nanosecond), time.Nanosecond)),
					Partial:        true,
				},
			}
		}(),
		"Asc cursor before the window start is ignored": {
			Lines: early,
			Range: TimeRange{From: _logsBase.Add(2 * time.Second), To: _logsBase.Add(time.Hour)},
			Page:  asc(5, time.Nanosecond, time.Nanosecond),
			Unit:  time.Nanosecond,
			Result: &LogsResult{
				Status: QueryStatusOK,
				Lines:  early[1:],
			},
		},
		"Asc cursor at the window end reads nothing without fetching": {
			Lines:     early,
			FetchErrs: []error{assert.AnError},
			Range:     TimeRange{From: _logsBase, To: _logsBase.Add(4 * time.Second)},
			Page:      asc(5, 4*time.Second, time.Nanosecond),
			Unit:      time.Nanosecond,
			Result: &LogsResult{
				Status: QueryStatusNoData,
				Lines:  []LogLine{},
			},
		},
		"Asc cursor past the window end reads nothing without fetching": {
			Lines:     early,
			FetchErrs: []error{assert.AnError},
			Range:     TimeRange{From: _logsBase, To: _logsBase.Add(4 * time.Second)},
			Page:      asc(5, 5*time.Second, time.Nanosecond),
			Unit:      time.Nanosecond,
			Result: &LogsResult{
				Status: QueryStatusNoData,
				Lines:  []LogLine{},
			},
		},
		"Cursor at the window start reads nothing without fetching": {
			Lines:     small,
			FetchErrs: []error{assert.AnError},
			Range:     TimeRange{From: _logsBase.Add(2 * time.Second), To: _logsBase.Add(time.Hour)},
			Page: LogsPage{
				Limit:  5,
				Cursor: null.ValueFrom(newCursor(_logsBase.Add(2*time.Second), time.Nanosecond)),
			},
			Unit: time.Nanosecond,
			Result: &LogsResult{
				Status: QueryStatusNoData,
				Lines:  []LogLine{},
			},
		},
		"Cursor before the window start reads nothing without fetching": {
			Lines:     small,
			FetchErrs: []error{assert.AnError},
			Range:     TimeRange{From: _logsBase.Add(2 * time.Second), To: _logsBase.Add(time.Hour)},
			Page: LogsPage{
				Limit:  5,
				Cursor: null.ValueFrom(newCursor(_logsBase.Add(time.Second), time.Nanosecond)),
			},
			Unit: time.Nanosecond,
			Result: &LogsResult{
				Status: QueryStatusNoData,
				Lines:  []LogLine{},
			},
		},
		"Asc millisecond unit steps the cursor by a millisecond": {
			Lines: early,
			Range: window,
			Page:  asc(1, 4*time.Second, time.Millisecond),
			Unit:  time.Millisecond,
			Result: &LogsResult{
				Status:         QueryStatusOK,
				Lines:          early[3:],
				PreviousCursor: null.ValueFrom(newCursor(_logsBase.Add(4*time.Second), time.Millisecond)),
				NextCursor:     null.ValueFrom(newCursor(_logsBase.Add(4*time.Second+time.Millisecond), time.Millisecond)),
			},
		},
		"Asc walk at limit 1 reads every line once": {
			Lines:     traps,
			Range:     window,
			Page:      asc(0, 0, time.Nanosecond),
			Unit:      time.Nanosecond,
			WalkLimit: 1,
		},
		"Asc walk at limit 7 reads every line once": {
			Lines:     traps,
			Range:     window,
			Page:      asc(0, 0, time.Nanosecond),
			Unit:      time.Nanosecond,
			WalkLimit: 7,
		},
		"Asc walk at limit 100 reads every line once": {
			Lines:     traps,
			Range:     window,
			Page:      asc(0, 0, time.Nanosecond),
			Unit:      time.Nanosecond,
			WalkLimit: 100,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			fetch := stubLogsFetcher(c.Lines, c.FetchErrs)

			if c.WalkLimit > 0 {
				page := LogsPage{Limit: c.WalkLimit, Order: c.Page.Order}
				assertWalkReadsEachOnce(t, fetch, c.Range, page, c.Unit, len(c.Lines))

				return
			}

			res, err := PageLogs(context.Background(), fetch, c.Range, c.Page, c.Unit)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			for i := range res.Lines {
				assert.NotEmpty(t, res.Lines[i].ID)
				res.Lines[i].ID = ""
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_LogsResult_withLineIDs(t *testing.T) {
	line := LogLine{
		Time:   _logsBase,
		Line:   "deploy started",
		Labels: map[string]string{"service_name": "deploy-bot"},
		Fields: map[string]string{},
	}
	other := line
	other.Line = "deploy finished"

	cc := map[string]struct {
		Result *LogsResult
		Status QueryStatus
		IDs    []string
	}{
		"No lines": {
			Result: &LogsResult{Status: QueryStatusOK},
			Status: QueryStatusNoData,
			IDs:    []string{},
		},
		"Distinct lines": {
			Result: &LogsResult{Status: QueryStatusOK, Lines: []LogLine{line, other}},
			Status: QueryStatusOK,
			IDs:    []string{line.hash(), other.hash()},
		},
		"Identical lines get a counter": {
			Result: &LogsResult{Status: QueryStatusOK, Lines: []LogLine{line, line, other, line}},
			Status: QueryStatusOK,
			IDs:    []string{line.hash(), line.hash() + "-1", other.hash(), line.hash() + "-2"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res := c.Result.withLineIDs()
			assert.Equal(t, c.Status, res.Status)
			require.NotNil(t, res.Lines)

			ids := make([]string, 0, len(res.Lines))

			for _, l := range res.Lines {
				ids = append(ids, l.ID)
			}

			assert.Equal(t, c.IDs, ids)
		})
	}
}

func Test_LogLine_hash(t *testing.T) {
	base := LogLine{
		Time:   _logsBase,
		Level:  LogLevelInfo,
		Line:   "deploy started",
		Labels: map[string]string{"service_name": "deploy-bot", "env": "production"},
		Fields: map[string]string{"sha": "3f9a1c2"},
	}

	with := func(change func(l *LogLine)) LogLine {
		l := base
		l.Labels = map[string]string{"service_name": "deploy-bot", "env": "production"}
		l.Fields = map[string]string{"sha": "3f9a1c2"}
		change(&l)

		return l
	}

	cc := map[string]struct {
		Other LogLine
		Equal bool
	}{
		"Same line":        {Other: with(func(*LogLine) {}), Equal: true},
		"Level is ignored": {Other: with(func(l *LogLine) { l.Level = LogLevelError }), Equal: true},
		"Different time":   {Other: with(func(l *LogLine) { l.Time = l.Time.Add(time.Nanosecond) }), Equal: false},
		"Different text":   {Other: with(func(l *LogLine) { l.Line = "deploy finished" }), Equal: false},
		"Different label":  {Other: with(func(l *LogLine) { l.Labels["env"] = "staging" }), Equal: false},
		"Different field":  {Other: with(func(l *LogLine) { l.Fields["sha"] = "0000000" }), Equal: false},
		"Label moved to fields": {
			Other: with(func(l *LogLine) {
				delete(l.Labels, "env")
				l.Fields["env"] = "production"
			}),
			Equal: false,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			h := base.hash()
			assert.Regexp(t, "^[0-9a-f]{16}$", h)
			assert.Equal(t, c.Equal, h == c.Other.hash())
		})
	}
}

func Test_sortedPairs(t *testing.T) {
	cc := map[string]struct {
		Map    map[string]string
		Result []string
	}{
		"Nil map":     {Map: nil, Result: []string{}},
		"Sorted keys": {Map: map[string]string{"b": "2", "a": "1", "c": "3"}, Result: []string{"a=1", "b=2", "c=3"}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, sortedPairs(c.Map))
		})
	}
}

// stubLogsFetcher returns a fetcher over lines that fails its nth call
// with errs[n-1] when that error is set.
func stubLogsFetcher(lines []LogLine, errs []error) LogsFetcher {
	var calls int

	return func(_ context.Context, from, end time.Time, limit int, order LogsOrder) ([]LogLine, error) {
		calls++

		if calls <= len(errs) && errs[calls-1] != nil {
			return nil, errs[calls-1]
		}

		sorted := slices.Clone(lines)
		order.SortLines(sorted)

		var out []LogLine

		for _, l := range sorted {
			if !l.Time.Before(from) && l.Time.Before(end) && len(out) < limit {
				out = append(out, l)
			}
		}

		return out, nil
	}
}

// walkLogs pages through the window from the given first page and
// returns every line read, in the order read, and the last page.
func walkLogs(t *testing.T, fetch LogsFetcher, tr TimeRange, page LogsPage, unit time.Duration) ([]LogLine, *LogsResult) {
	t.Helper()

	var lines []LogLine

	for {
		res, err := PageLogs(context.Background(), fetch, tr, page, unit)
		require.NoError(t, err)

		lines = append(lines, res.Lines...)

		if !res.NextCursor.Valid {
			return lines, res
		}

		page.Cursor = res.NextCursor
	}
}

// walkLogsBack pages back from last through the previous cursors, in the
// order opposite to order, and returns last's own lines and every line
// read after them, in that order.
func walkLogsBack(
	t *testing.T,
	fetch LogsFetcher,
	tr TimeRange,
	order LogsOrder,
	limit int,
	last *LogsResult,
	unit time.Duration,
) []LogLine {
	t.Helper()

	own := slices.Clone(last.Lines)
	slices.Reverse(own)

	if !last.PreviousCursor.Valid {
		return own
	}

	page := LogsPage{
		Limit:  limit,
		Order:  oppositeLogsOrder(order),
		Cursor: last.PreviousCursor,
	}
	lines, _ := walkLogs(t, fetch, tr, page, unit)

	return append(own, lines...)
}

// oppositeLogsOrder returns the order that reads the other way.
func oppositeLogsOrder(order LogsOrder) LogsOrder {
	if order == LogsOrderAsc {
		return LogsOrderDesc
	}

	return LogsOrderAsc
}

// assertWalkReadsEachOnce pages through the window from the given first
// page, then back from the last page through the previous cursors. Each
// walk must read every one of total lines, told apart by their "seq"
// field, exactly once and in the walk's order.
func assertWalkReadsEachOnce(
	t *testing.T,
	fetch LogsFetcher,
	tr TimeRange,
	page LogsPage,
	unit time.Duration,
	total int,
) {
	t.Helper()

	lines, last := walkLogs(t, fetch, tr, page, unit)
	assertLinesOrdered(t, page.Order, lines)
	assertReadEachOnce(t, lines, total)

	back := walkLogsBack(t, fetch, tr, page.Order, page.Limit, last, unit)
	assertLinesOrdered(t, oppositeLogsOrder(page.Order), back)
	assertReadEachOnce(t, back, total)
}

// assertReadEachOnce checks that lines hold every one of total lines,
// told apart by their "seq" field, exactly once.
func assertReadEachOnce(t *testing.T, lines []LogLine, total int) {
	t.Helper()

	seen := make(map[string]int, total)

	for _, l := range lines {
		seen[l.Fields["seq"]]++
	}

	assert.Len(t, seen, total)

	for seq, n := range seen {
		assert.Equal(t, 1, n, "line %s read %d times", seq, n)
	}
}

// assertLinesOrdered checks that lines are oldest first when order is
// asc and newest first otherwise.
func assertLinesOrdered(t *testing.T, order LogsOrder, lines []LogLine) {
	t.Helper()

	sorted := slices.Clone(lines)
	order.SortLines(sorted)

	assert.True(t, slices.EqualFunc(lines, sorted, func(a, b LogLine) bool {
		return a.Time.Equal(b.Time)
	}), "lines are not in %s order", order)
}
