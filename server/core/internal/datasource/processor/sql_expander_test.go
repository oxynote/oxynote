package processor

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _testTimeRange = TimeRange{
	From: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
	To:   time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
}

func Test_sqlExpander_expand(t *testing.T) {
	cc := map[string]struct {
		Query  string
		InArgs bool
		Vars   map[string]string
		Result string
		Next   int
		Err    error
	}{
		"Code byte":                           {Query: "SELECT", Result: "S", Next: 1},
		"String with a variable":              {Query: "'$v' x", Vars: map[string]string{"v": "a'b"}, Result: "'a''b'", Next: 4},
		"Reference":                           {Query: "$v x", Vars: map[string]string{"v": "1"}, Result: "1", Next: 2},
		"Macro call":                          {Query: "$__timeTo() x", Result: "'2024-01-15T11:00:00Z'::timestamptz", Next: 11},
		"Macro call in arguments is an error": {Query: "$__timeTo()", InArgs: true, Err: ErrMacroNested},
		"Unterminated string":                 {Query: "'a", Err: ErrQueryUnterminated},
		"Unsafe value in a string":            {Query: "'$v'", Vars: map[string]string{"v": "a\x00"}, Err: unsafeValueError("$v")},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			e := &sqlExpander{q: c.Query, d: _postgreSQLDialect, tr: _testTimeRange, vars: c.Vars}

			var b strings.Builder

			next, err := e.expand(&b, 0, c.InArgs)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, b.String())
			assert.Equal(t, c.Next, next)
		})
	}
}

func Test_sqlExpander_expandQuoted(t *testing.T) {
	cc := map[string]struct {
		Dialect sqlDialect
		Query   string
		Vars    map[string]string
		Result  string
		Err     error
	}{
		"String with a variable":         {Dialect: _postgreSQLDialect, Query: "'a $v b'", Vars: map[string]string{"v": "x'y"}, Result: "'a x''y b'"},
		"Quoted name with a variable":    {Dialect: _postgreSQLDialect, Query: `"$v"`, Vars: map[string]string{"v": `x"y`}, Result: `"x""y"`},
		"Dollar string with a variable":  {Dialect: _postgreSQLDialect, Query: "$q$ $v $q$", Vars: map[string]string{"v": "x'y"}, Result: "$q$ x'y $q$"},
		"Escaped dollar is no reference": {Dialect: _mySQLDialect, Query: `'\$v'`, Vars: map[string]string{"v": "1"}, Result: `'\$v'`},
		"Global macro":                   {Dialect: _postgreSQLDialect, Query: "'$__from'", Result: "'1705312800000'"},
		"Braced global macro":            {Dialect: _postgreSQLDialect, Query: "'${__from:date:seconds}'", Result: "'1705312800'"},
		"Unknown reference stays":        {Dialect: _postgreSQLDialect, Query: "'$x'", Result: "'$x'"},
		"Macro call stays as written":    {Dialect: _postgreSQLDialect, Query: "'$__timeTo()'", Result: "'$__timeTo()'"},
		"Unclosed braced macro":          {Dialect: _postgreSQLDialect, Query: "'${__from'", Err: ErrMacroUnclosedBrace},
		"Unsafe value":                   {Dialect: _postgreSQLDialect, Query: "$$ $v $$", Vars: map[string]string{"v": "$"}, Err: unsafeValueError("$v")},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tok, err := c.Dialect.token(c.Query, 0)
			require.NoError(t, err)

			e := &sqlExpander{q: c.Query, d: c.Dialect, tr: _testTimeRange, vars: c.Vars}

			var b strings.Builder

			err = e.expandQuoted(&b, tok)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, b.String())
		})
	}
}

func Test_sqlExpander_writeQuotedValue(t *testing.T) {
	cc := map[string]struct {
		Dialect sqlDialect
		Quoted  string
		Ref     string
		Value   string
		Result  string
		Err     error
	}{
		"Unknown reference stays":               {Dialect: _postgreSQLDialect, Quoted: "''", Ref: "$x", Value: "a'b", Result: "$x"},
		"String doubles its quote":              {Dialect: _postgreSQLDialect, Quoted: "''", Value: "a'b", Result: "a''b"},
		"Postgres string keeps a backslash":     {Dialect: _postgreSQLDialect, Quoted: "''", Value: `a\b`, Result: `a\b`},
		"Backslash string doubles backslashes":  {Dialect: _mySQLDialect, Quoted: "''", Value: `a\'`, Result: `a\\''`},
		"Quoted name doubles its quote":         {Dialect: _postgreSQLDialect, Quoted: `""`, Value: `a"b`, Result: `a""b`},
		"MySQL quoted name doubles its quote":   {Dialect: _mySQLDialect, Quoted: "``", Value: "a`b", Result: "a``b"},
		"Dollar string keeps a quote":           {Dialect: _postgreSQLDialect, Quoted: "$$$$", Value: "a'b", Result: "a'b"},
		"Dollar in a dollar string is an error": {Dialect: _postgreSQLDialect, Quoted: "$$$$", Value: "a$", Err: unsafeValueError("$v")},
		"NUL is an error":                       {Dialect: _postgreSQLDialect, Quoted: "''", Value: "a\x00", Err: unsafeValueError("$v")},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tok, err := c.Dialect.token(c.Quoted, 0)
			require.NoError(t, err)

			e := &sqlExpander{q: c.Quoted, d: c.Dialect, tr: _testTimeRange, vars: map[string]string{"v": c.Value}}

			var b strings.Builder

			ref := c.Ref
			if ref == "" {
				ref = "$v"
			}

			err = e.writeQuotedValue(&b, ref, tok)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, b.String())
		})
	}
}

func Test_sqlExpander_expandReference(t *testing.T) {
	cc := map[string]struct {
		Query   string
		Written string
		InArgs  bool
		Vars    map[string]string
		Result  string
		Next    int
		Err     error
	}{
		"Variable":                                 {Query: "$v", Vars: map[string]string{"v": "1"}, Result: "1", Next: 2},
		"Negative value":                           {Query: "$v", Written: "x > ", Vars: map[string]string{"v": "-1"}, Result: "x > -1", Next: 2},
		"Global macro":                             {Query: "$__to", Result: "1705316400000", Next: 5},
		"Braced global macro":                      {Query: "${__to:date:seconds}", Result: "1705316400", Next: 20},
		"Unknown macro is an error":                {Query: "$__nope", Err: NewInvalidQueryError("unknown macro $__nope")},
		"Macro without its call is an error":       {Query: "$__timeFilter", Err: NewInvalidQueryError("unknown macro $__timeFilter")},
		"Unknown variable stays":                   {Query: "$1", Result: "$1", Next: 2},
		"Unsafe value is an error":                 {Query: "$v", Vars: map[string]string{"v": "a b"}, Err: unsafeValueError("$v")},
		"Value before a quote is an error":         {Query: "$v'x'", Vars: map[string]string{"v": "E"}, Err: unsafeValueError("$v")},
		"Value before a name quote is an error":    {Query: `$v"x"`, Vars: map[string]string{"v": "a"}, Err: unsafeValueError("$v")},
		"Negative value after a minus is an error": {Query: "$v", Written: "1 -", Vars: map[string]string{"v": "-1"}, Err: unsafeValueError("$v")},
		"Macro call":                               {Query: "$__timeTo() x", Result: "'2024-01-15T11:00:00Z'::timestamptz", Next: 11},
		"Macro call in arguments is an error":      {Query: "$__timeTo()", InArgs: true, Err: ErrMacroNested},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tok, err := _postgreSQLDialect.token(c.Query, 0)
			require.NoError(t, err)

			e := &sqlExpander{q: c.Query, d: _postgreSQLDialect, tr: _testTimeRange, vars: c.Vars}

			var b strings.Builder

			b.WriteString(c.Written)

			next, err := e.expandReference(&b, tok, c.InArgs)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, b.String())
			assert.Equal(t, c.Next, next)
		})
	}
}

func Test_sqlExpander_expandCall(t *testing.T) {
	const between = " BETWEEN '2024-01-15T10:00:00Z'::timestamptz AND '2024-01-15T11:00:00Z'::timestamptz"

	cc := map[string]struct {
		Query  string
		Name   string
		Vars   map[string]string
		Result string
		Next   int
		Err    error
	}{
		"Call":                    {Query: "$__timeFilter(ts) x", Name: "timeFilter", Result: "(ts)" + between, Next: 17},
		"Nested parentheses":      {Query: "$__timeFilter(f(a, b))", Name: "timeFilter", Result: "(f(a, b))" + between, Next: 22},
		"Several arguments":       {Query: "$__timeGroup(ts, '5m')", Name: "timeGroup", Result: "floor(EXTRACT(EPOCH FROM ts)/300)*300", Next: 22},
		"Quoted parenthesis":      {Query: "$__timeFilter(a = ')')", Name: "timeFilter", Result: "(a = ')')" + between, Next: 22},
		"Variable in an argument": {Query: "$__timeFilter($v)", Name: "timeFilter", Vars: map[string]string{"v": "ts"}, Result: "(ts)" + between, Next: 17},
		"No closing parenthesis":  {Query: "$__timeFilter(ts", Name: "timeFilter", Err: NewInvalidQueryError("macro $__timeFilter has no closing parenthesis")},
		"Nested macro call":       {Query: "$__timeFilter($__timeTo())", Name: "timeFilter", Err: ErrMacroNested},
		"Unknown macro":           {Query: "$__nope(x)", Name: "nope", Err: NewInvalidQueryError("unknown macro $__nope")},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			e := &sqlExpander{q: c.Query, d: _postgreSQLDialect, tr: _testTimeRange, vars: c.Vars}

			var b strings.Builder

			next, err := e.expandCall(&b, c.Name, strings.IndexByte(c.Query, '('))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, b.String())
			assert.Equal(t, c.Next, next)
		})
	}
}

func Test_sqlExpander_render(t *testing.T) {
	const between = " BETWEEN '2024-01-15T10:00:00Z'::timestamptz AND '2024-01-15T11:00:00Z'::timestamptz"

	cc := map[string]struct {
		Name   string
		Args   []string
		Result string
		Fill   *sqlFill
		Err    error
	}{
		"Arguments are trimmed":        {Name: "timeFilter", Args: []string{" ts "}, Result: "(ts)" + between},
		"No arguments":                 {Name: "timeTo", Args: []string{""}, Result: "'2024-01-15T11:00:00Z'::timestamptz"},
		"Fill mode":                    {Name: "timeGroup", Args: []string{"ts", "'5m'", "NULL"}, Result: "floor(EXTRACT(EPOCH FROM ts)/300)*300", Fill: &sqlFill{Interval: 300, From: 1705312800, To: 1705316400}},
		"Unknown macro":                {Name: "nope", Args: []string{"x"}, Err: NewInvalidQueryError("unknown macro $__nope")},
		"Argument where none is taken": {Name: "timeTo", Args: []string{"x"}, Err: NewInvalidQueryError("macro $__timeTo takes 0 arguments")},
		"Missing argument":             {Name: "time", Args: []string{""}, Err: NewInvalidQueryError("macro $__time takes 1 argument")},
		"Too many arguments":           {Name: "time", Args: []string{"a", "b"}, Err: NewInvalidQueryError("macro $__time takes 1 argument")},
		"Empty argument":               {Name: "timeGroup", Args: []string{"ts", " "}, Err: NewInvalidQueryError("macro $__timeGroup takes 2 or 3 arguments")},
		"Argument after the fill mode": {Name: "timeGroup", Args: []string{"ts", "'5m'", "0", "1"}, Err: NewInvalidQueryError("macro $__timeGroup takes 2 or 3 arguments")},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			e := &sqlExpander{d: _postgreSQLDialect, tr: _testTimeRange}

			res, err := e.render(c.Name, c.Args)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
			assert.Equal(t, c.Fill, e.fill)
		})
	}
}

func Test_sqlExpander_lookup(t *testing.T) {
	cc := map[string]struct {
		Ref    string
		Result string
		OK     bool
	}{
		"Global macro":                {Ref: "$__from", Result: "1705312800000", OK: true},
		"Braced global macro":         {Ref: "${__from:date:seconds}", Result: "1705312800", OK: true},
		"Unknown global macro":        {Ref: "$__nope"},
		"Unknown braced global macro": {Ref: "${__nope}"},
		"Unclosed braces":             {Ref: "${__from"},
		"Variable":                    {Ref: "$v", Result: "x", OK: true},
		"Unknown variable":            {Ref: "$w"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			e := &sqlExpander{d: _postgreSQLDialect, tr: _testTimeRange, vars: map[string]string{"v": "x"}}

			res, ok := e.lookup(c.Ref)
			assert.Equal(t, c.Result, res)
			assert.Equal(t, c.OK, ok)
		})
	}
}

func Test_expandSQL(t *testing.T) {
	const filter = "'2024-01-15T10:00:00Z'::timestamptz AND '2024-01-15T11:00:00Z'::timestamptz"

	cc := map[string]struct {
		Dialect sqlDialect
		Query   string
		Vars    map[string]string
		Result  string
		Fill    *sqlFill
		Err     error
	}{
		"PostgreSQL: Time column":                                                 {Dialect: _postgreSQLDialect, Query: "SELECT $__time(created_at), value FROM metrics", Result: "SELECT EXTRACT(EPOCH FROM created_at) AS \"time\", value FROM metrics"},
		"PostgreSQL: Time epoch column":                                           {Dialect: _postgreSQLDialect, Query: "SELECT $__timeEpoch(created_at), value FROM metrics", Result: "SELECT EXTRACT(EPOCH FROM created_at) AS \"time\", value FROM metrics"},
		"PostgreSQL: Time filter":                                                 {Dialect: _postgreSQLDialect, Query: "SELECT * FROM metrics WHERE $__timeFilter(created_at)", Result: "SELECT * FROM metrics WHERE (created_at) BETWEEN '2024-01-15T10:00:00Z'::timestamptz AND '2024-01-15T11:00:00Z'::timestamptz"},
		"PostgreSQL: Range start":                                                 {Dialect: _postgreSQLDialect, Query: "SELECT * FROM metrics WHERE created_at >= $__timeFrom()", Result: "SELECT * FROM metrics WHERE created_at >= '2024-01-15T10:00:00Z'::timestamptz"},
		"PostgreSQL: Range end":                                                   {Dialect: _postgreSQLDialect, Query: "SELECT * FROM metrics WHERE created_at <= $__timeTo()", Result: "SELECT * FROM metrics WHERE created_at <= '2024-01-15T11:00:00Z'::timestamptz"},
		"PostgreSQL: Range start and end together":                                {Dialect: _postgreSQLDialect, Query: "SELECT * FROM metrics WHERE created_at >= $__timeFrom() AND created_at <= $__timeTo()", Result: "SELECT * FROM metrics WHERE created_at >= '2024-01-15T10:00:00Z'::timestamptz AND created_at <= '2024-01-15T11:00:00Z'::timestamptz"},
		"PostgreSQL: Time group by five minutes":                                  {Dialect: _postgreSQLDialect, Query: "SELECT $__timeGroup(created_at,'5m'), avg(value) FROM metrics GROUP BY 1", Result: "SELECT floor(EXTRACT(EPOCH FROM created_at)/300)*300, avg(value) FROM metrics GROUP BY 1"},
		"PostgreSQL: Time group by one hour":                                      {Dialect: _postgreSQLDialect, Query: "SELECT $__timeGroup(created_at,'1h') FROM metrics", Result: "SELECT floor(EXTRACT(EPOCH FROM created_at)/3600)*3600 FROM metrics"},
		"PostgreSQL: Time group with a zero fill mode":                            {Dialect: _postgreSQLDialect, Query: "SELECT $__timeGroup(created_at,'5m', 0), avg(value) FROM metrics GROUP BY 1", Result: "SELECT floor(EXTRACT(EPOCH FROM created_at)/300)*300, avg(value) FROM metrics GROUP BY 1", Fill: &sqlFill{Value: new(0.0), Interval: 300, From: 1705312800, To: 1705316400}},
		"PostgreSQL: Time group with a NULL fill mode":                            {Dialect: _postgreSQLDialect, Query: "SELECT $__timeGroup(created_at,'5m', NULL), avg(value) FROM metrics GROUP BY 1", Result: "SELECT floor(EXTRACT(EPOCH FROM created_at)/300)*300, avg(value) FROM metrics GROUP BY 1", Fill: &sqlFill{Interval: 300, From: 1705312800, To: 1705316400}},
		"PostgreSQL: Time group with a previous fill mode":                        {Dialect: _postgreSQLDialect, Query: "SELECT $__timeGroup(created_at,'5m', previous) FROM metrics", Result: "SELECT floor(EXTRACT(EPOCH FROM created_at)/300)*300 FROM metrics", Fill: &sqlFill{Previous: true, Interval: 300, From: 1705312800, To: 1705316400}},
		"PostgreSQL: Time group alias by five minutes":                            {Dialect: _postgreSQLDialect, Query: "SELECT $__timeGroupAlias(created_at,'5m'), avg(value) FROM metrics GROUP BY 1", Result: "SELECT floor(EXTRACT(EPOCH FROM created_at)/300)*300 AS \"time\", avg(value) FROM metrics GROUP BY 1"},
		"PostgreSQL: Unix epoch filter":                                           {Dialect: _postgreSQLDialect, Query: "SELECT * FROM metrics WHERE $__unixEpochFilter(epoch_col)", Result: "SELECT * FROM metrics WHERE (epoch_col) >= 1705312800 AND (epoch_col) <= 1705316400"},
		"PostgreSQL: Unix epoch range start":                                      {Dialect: _postgreSQLDialect, Query: "SELECT * FROM metrics WHERE epoch >= $__unixEpochFrom()", Result: "SELECT * FROM metrics WHERE epoch >= 1705312800"},
		"PostgreSQL: Unix epoch range end":                                        {Dialect: _postgreSQLDialect, Query: "SELECT * FROM metrics WHERE epoch <= $__unixEpochTo()", Result: "SELECT * FROM metrics WHERE epoch <= 1705316400"},
		"PostgreSQL: Unix epoch nanosecond filter":                                {Dialect: _postgreSQLDialect, Query: "SELECT * FROM metrics WHERE $__unixEpochNanoFilter(ns_col)", Result: "SELECT * FROM metrics WHERE (ns_col) >= 1705312800000000000 AND (ns_col) <= 1705316400000000000"},
		"PostgreSQL: Unix epoch nanosecond range start":                           {Dialect: _postgreSQLDialect, Query: "SELECT * FROM metrics WHERE ns >= $__unixEpochNanoFrom()", Result: "SELECT * FROM metrics WHERE ns >= 1705312800000000000"},
		"PostgreSQL: Unix epoch nanosecond range end":                             {Dialect: _postgreSQLDialect, Query: "SELECT * FROM metrics WHERE ns <= $__unixEpochNanoTo()", Result: "SELECT * FROM metrics WHERE ns <= 1705316400000000000"},
		"PostgreSQL: Unix epoch group by five minutes":                            {Dialect: _postgreSQLDialect, Query: "SELECT $__unixEpochGroup(epoch_col,'5m'), avg(value) FROM metrics GROUP BY 1", Result: "SELECT floor((epoch_col)/300)*300, avg(value) FROM metrics GROUP BY 1"},
		"PostgreSQL: Unix epoch group with a fill mode":                           {Dialect: _postgreSQLDialect, Query: "SELECT $__unixEpochGroup(epoch_col,'5m', 0) FROM metrics", Result: "SELECT floor((epoch_col)/300)*300 FROM metrics", Fill: &sqlFill{Value: new(0.0), Interval: 300, From: 1705312800, To: 1705316400}},
		"PostgreSQL: Unix epoch group alias by five minutes":                      {Dialect: _postgreSQLDialect, Query: "SELECT $__unixEpochGroupAlias(epoch_col,'5m'), avg(value) FROM metrics GROUP BY 1", Result: "SELECT floor((epoch_col)/300)*300 AS \"time\", avg(value) FROM metrics GROUP BY 1"},
		"PostgreSQL: Time column without a column is an error":                    {Dialect: _postgreSQLDialect, Query: "SELECT $__time() FROM metrics", Err: NewInvalidQueryError("macro $__time takes 1 argument")},
		"PostgreSQL: Time filter without a column is an error":                    {Dialect: _postgreSQLDialect, Query: "SELECT * FROM metrics WHERE $__timeFilter()", Err: NewInvalidQueryError("macro $__timeFilter takes 1 argument")},
		"PostgreSQL: Time group without an interval is an error":                  {Dialect: _postgreSQLDialect, Query: "SELECT $__timeGroup(created_at) FROM metrics", Err: NewInvalidQueryError("macro $__timeGroup takes 2 or 3 arguments")},
		"PostgreSQL: Time group with an invalid interval is an error":             {Dialect: _postgreSQLDialect, Query: "SELECT $__timeGroup(created_at,'5x') FROM metrics", Err: NewInvalidQueryError("interval '5x' must be a whole number of s, m, h, d or w, at most 10 years")},
		"PostgreSQL: Time group alias without an interval is an error":            {Dialect: _postgreSQLDialect, Query: "SELECT $__timeGroupAlias(created_at) FROM metrics", Err: NewInvalidQueryError("macro $__timeGroupAlias takes 2 or 3 arguments")},
		"PostgreSQL: Time group alias with an invalid interval is an error":       {Dialect: _postgreSQLDialect, Query: "SELECT $__timeGroupAlias(created_at,'5x') FROM metrics", Err: NewInvalidQueryError("interval '5x' must be a whole number of s, m, h, d or w, at most 10 years")},
		"PostgreSQL: Unix epoch group alias without an interval is an error":      {Dialect: _postgreSQLDialect, Query: "SELECT $__unixEpochGroupAlias(epoch_col) FROM metrics", Err: NewInvalidQueryError("macro $__unixEpochGroupAlias takes 2 or 3 arguments")},
		"PostgreSQL: Unix epoch group alias with an invalid interval is an error": {Dialect: _postgreSQLDialect, Query: "SELECT $__unixEpochGroupAlias(epoch_col,'5x') FROM metrics", Err: NewInvalidQueryError("interval '5x' must be a whole number of s, m, h, d or w, at most 10 years")},
		"PostgreSQL: Unix epoch filter without a column is an error":              {Dialect: _postgreSQLDialect, Query: "SELECT * FROM metrics WHERE $__unixEpochFilter()", Err: NewInvalidQueryError("macro $__unixEpochFilter takes 1 argument")},
		"PostgreSQL: Unix epoch nanosecond filter without a column is an error":   {Dialect: _postgreSQLDialect, Query: "SELECT * FROM metrics WHERE $__unixEpochNanoFilter()", Err: NewInvalidQueryError("macro $__unixEpochNanoFilter takes 1 argument")},
		"PostgreSQL: Unix epoch group without an interval is an error":            {Dialect: _postgreSQLDialect, Query: "SELECT $__unixEpochGroup(epoch_col) FROM metrics", Err: NewInvalidQueryError("macro $__unixEpochGroup takes 2 or 3 arguments")},
		"PostgreSQL: Unix epoch group with an invalid interval is an error":       {Dialect: _postgreSQLDialect, Query: "SELECT $__unixEpochGroup(epoch_col,'5x') FROM metrics", Err: NewInvalidQueryError("interval '5x' must be a whole number of s, m, h, d or w, at most 10 years")},
		"PostgreSQL: Unknown macro is an error":                                   {Dialect: _postgreSQLDialect, Query: "SELECT $__unknown(col) FROM metrics", Err: NewInvalidQueryError("unknown macro $__unknown")},
		"PostgreSQL: No macros leaves query unchanged":                            {Dialect: _postgreSQLDialect, Query: "SELECT time, value FROM metrics WHERE time > now() - interval '1 hour'", Result: "SELECT time, value FROM metrics WHERE time > now() - interval '1 hour'"},
		"PostgreSQL: Multiple macros in one query":                                {Dialect: _postgreSQLDialect, Query: "SELECT $__timeGroupAlias(created_at,'5m'), avg(value) FROM metrics WHERE $__timeFilter(created_at) GROUP BY 1", Result: "SELECT floor(EXTRACT(EPOCH FROM created_at)/300)*300 AS \"time\", avg(value) FROM metrics WHERE (created_at) BETWEEN '2024-01-15T10:00:00Z'::timestamptz AND '2024-01-15T11:00:00Z'::timestamptz GROUP BY 1"},
		"PostgreSQL: Time group alias by the calculated interval":                 {Dialect: _postgreSQLDialect, Query: "SELECT\n    $__timeGroupAlias(\"time\", $__interval),\n    service,\n    COUNT(*)::double precision AS deployments\nFROM deployments\nWHERE $__timeFilter(\"time\")\nGROUP BY 1, 2\nORDER BY 1, 2", Result: "SELECT\n    floor(EXTRACT(EPOCH FROM \"time\")/60)*60 AS \"time\",\n    service,\n    COUNT(*)::double precision AS deployments\nFROM deployments\nWHERE (\"time\") BETWEEN '2024-01-15T10:00:00Z'::timestamptz AND '2024-01-15T11:00:00Z'::timestamptz\nGROUP BY 1, 2\nORDER BY 1, 2"},
		"MySQL: Time column":                                                      {Dialect: _mySQLDialect, Query: "SELECT $__time(created_at), value FROM metrics", Result: "SELECT UNIX_TIMESTAMP(created_at) AS `time`, value FROM metrics"},
		"MySQL: Time epoch column":                                                {Dialect: _mySQLDialect, Query: "SELECT $__timeEpoch(created_at), value FROM metrics", Result: "SELECT UNIX_TIMESTAMP(created_at) AS `time`, value FROM metrics"},
		"MySQL: Time filter":                                                      {Dialect: _mySQLDialect, Query: "SELECT * FROM metrics WHERE $__timeFilter(created_at)", Result: "SELECT * FROM metrics WHERE (created_at) BETWEEN FROM_UNIXTIME(1705312800) AND FROM_UNIXTIME(1705316400)"},
		"MySQL: Range start":                                                      {Dialect: _mySQLDialect, Query: "SELECT * FROM metrics WHERE created_at >= $__timeFrom()", Result: "SELECT * FROM metrics WHERE created_at >= FROM_UNIXTIME(1705312800)"},
		"MySQL: Range end":                                                        {Dialect: _mySQLDialect, Query: "SELECT * FROM metrics WHERE created_at <= $__timeTo()", Result: "SELECT * FROM metrics WHERE created_at <= FROM_UNIXTIME(1705316400)"},
		"MySQL: Range start and end together":                                     {Dialect: _mySQLDialect, Query: "SELECT * FROM metrics WHERE created_at >= $__timeFrom() AND created_at <= $__timeTo()", Result: "SELECT * FROM metrics WHERE created_at >= FROM_UNIXTIME(1705312800) AND created_at <= FROM_UNIXTIME(1705316400)"},
		"MySQL: Time group by five minutes":                                       {Dialect: _mySQLDialect, Query: "SELECT $__timeGroup(created_at,'5m'), avg(value) FROM metrics GROUP BY 1", Result: "SELECT floor(UNIX_TIMESTAMP(created_at)/300)*300, avg(value) FROM metrics GROUP BY 1"},
		"MySQL: Time group by one hour":                                           {Dialect: _mySQLDialect, Query: "SELECT $__timeGroup(created_at,'1h') FROM metrics", Result: "SELECT floor(UNIX_TIMESTAMP(created_at)/3600)*3600 FROM metrics"},
		"MySQL: Time group with a zero fill mode":                                 {Dialect: _mySQLDialect, Query: "SELECT $__timeGroup(created_at,'5m', 0), avg(value) FROM metrics GROUP BY 1", Result: "SELECT floor(UNIX_TIMESTAMP(created_at)/300)*300, avg(value) FROM metrics GROUP BY 1", Fill: &sqlFill{Value: new(0.0), Interval: 300, From: 1705312800, To: 1705316400}},
		"MySQL: Time group with a NULL fill mode":                                 {Dialect: _mySQLDialect, Query: "SELECT $__timeGroup(created_at,'5m', NULL), avg(value) FROM metrics GROUP BY 1", Result: "SELECT floor(UNIX_TIMESTAMP(created_at)/300)*300, avg(value) FROM metrics GROUP BY 1", Fill: &sqlFill{Interval: 300, From: 1705312800, To: 1705316400}},
		"MySQL: Time group with a previous fill mode":                             {Dialect: _mySQLDialect, Query: "SELECT $__timeGroup(created_at,'5m', previous) FROM metrics", Result: "SELECT floor(UNIX_TIMESTAMP(created_at)/300)*300 FROM metrics", Fill: &sqlFill{Previous: true, Interval: 300, From: 1705312800, To: 1705316400}},
		"MySQL: Time group alias by five minutes":                                 {Dialect: _mySQLDialect, Query: "SELECT $__timeGroupAlias(created_at,'5m'), avg(value) FROM metrics GROUP BY 1", Result: "SELECT floor(UNIX_TIMESTAMP(created_at)/300)*300 AS `time`, avg(value) FROM metrics GROUP BY 1"},
		"MySQL: Unix epoch filter":                                                {Dialect: _mySQLDialect, Query: "SELECT * FROM metrics WHERE $__unixEpochFilter(epoch_col)", Result: "SELECT * FROM metrics WHERE (epoch_col) >= 1705312800 AND (epoch_col) <= 1705316400"},
		"MySQL: Unix epoch range start":                                           {Dialect: _mySQLDialect, Query: "SELECT * FROM metrics WHERE epoch >= $__unixEpochFrom()", Result: "SELECT * FROM metrics WHERE epoch >= 1705312800"},
		"MySQL: Unix epoch range end":                                             {Dialect: _mySQLDialect, Query: "SELECT * FROM metrics WHERE epoch <= $__unixEpochTo()", Result: "SELECT * FROM metrics WHERE epoch <= 1705316400"},
		"MySQL: Unix epoch nanosecond filter":                                     {Dialect: _mySQLDialect, Query: "SELECT * FROM metrics WHERE $__unixEpochNanoFilter(ns_col)", Result: "SELECT * FROM metrics WHERE (ns_col) >= 1705312800000000000 AND (ns_col) <= 1705316400000000000"},
		"MySQL: Unix epoch nanosecond range start":                                {Dialect: _mySQLDialect, Query: "SELECT * FROM metrics WHERE ns >= $__unixEpochNanoFrom()", Result: "SELECT * FROM metrics WHERE ns >= 1705312800000000000"},
		"MySQL: Unix epoch nanosecond range end":                                  {Dialect: _mySQLDialect, Query: "SELECT * FROM metrics WHERE ns <= $__unixEpochNanoTo()", Result: "SELECT * FROM metrics WHERE ns <= 1705316400000000000"},
		"MySQL: Unix epoch group by five minutes":                                 {Dialect: _mySQLDialect, Query: "SELECT $__unixEpochGroup(epoch_col,'5m'), avg(value) FROM metrics GROUP BY 1", Result: "SELECT floor((epoch_col)/300)*300, avg(value) FROM metrics GROUP BY 1"},
		"MySQL: Unix epoch group with a fill mode":                                {Dialect: _mySQLDialect, Query: "SELECT $__unixEpochGroup(epoch_col,'5m', 0) FROM metrics", Result: "SELECT floor((epoch_col)/300)*300 FROM metrics", Fill: &sqlFill{Value: new(0.0), Interval: 300, From: 1705312800, To: 1705316400}},
		"MySQL: Unix epoch group alias by five minutes":                           {Dialect: _mySQLDialect, Query: "SELECT $__unixEpochGroupAlias(epoch_col,'5m'), avg(value) FROM metrics GROUP BY 1", Result: "SELECT floor((epoch_col)/300)*300 AS `time`, avg(value) FROM metrics GROUP BY 1"},
		"MySQL: Time column without a column is an error":                         {Dialect: _mySQLDialect, Query: "SELECT $__time() FROM metrics", Err: NewInvalidQueryError("macro $__time takes 1 argument")},
		"MySQL: Time filter without a column is an error":                         {Dialect: _mySQLDialect, Query: "SELECT * FROM metrics WHERE $__timeFilter()", Err: NewInvalidQueryError("macro $__timeFilter takes 1 argument")},
		"MySQL: Time group without an interval is an error":                       {Dialect: _mySQLDialect, Query: "SELECT $__timeGroup(created_at) FROM metrics", Err: NewInvalidQueryError("macro $__timeGroup takes 2 or 3 arguments")},
		"MySQL: Time group with an invalid interval is an error":                  {Dialect: _mySQLDialect, Query: "SELECT $__timeGroup(created_at,'5x') FROM metrics", Err: NewInvalidQueryError("interval '5x' must be a whole number of s, m, h, d or w, at most 10 years")},
		"MySQL: Time group alias without an interval is an error":                 {Dialect: _mySQLDialect, Query: "SELECT $__timeGroupAlias(created_at) FROM metrics", Err: NewInvalidQueryError("macro $__timeGroupAlias takes 2 or 3 arguments")},
		"MySQL: Time group alias with an invalid interval is an error":            {Dialect: _mySQLDialect, Query: "SELECT $__timeGroupAlias(created_at,'5x') FROM metrics", Err: NewInvalidQueryError("interval '5x' must be a whole number of s, m, h, d or w, at most 10 years")},
		"MySQL: Unix epoch group alias without an interval is an error":           {Dialect: _mySQLDialect, Query: "SELECT $__unixEpochGroupAlias(epoch_col) FROM metrics", Err: NewInvalidQueryError("macro $__unixEpochGroupAlias takes 2 or 3 arguments")},
		"MySQL: Unix epoch group alias with an invalid interval is an error":      {Dialect: _mySQLDialect, Query: "SELECT $__unixEpochGroupAlias(epoch_col,'5x') FROM metrics", Err: NewInvalidQueryError("interval '5x' must be a whole number of s, m, h, d or w, at most 10 years")},
		"MySQL: Unknown macro is an error":                                        {Dialect: _mySQLDialect, Query: "SELECT $__unknown(col) FROM metrics", Err: NewInvalidQueryError("unknown macro $__unknown")},
		"MySQL: No macros leaves query unchanged":                                 {Dialect: _mySQLDialect, Query: "SELECT time, value FROM metrics WHERE time > NOW() - INTERVAL 1 HOUR", Result: "SELECT time, value FROM metrics WHERE time > NOW() - INTERVAL 1 HOUR"},
		"MySQL: Multiple macros in one query":                                     {Dialect: _mySQLDialect, Query: "SELECT $__timeGroupAlias(created_at,'5m'), avg(value) FROM metrics WHERE $__timeFilter(created_at) GROUP BY 1", Result: "SELECT floor(UNIX_TIMESTAMP(created_at)/300)*300 AS `time`, avg(value) FROM metrics WHERE (created_at) BETWEEN FROM_UNIXTIME(1705312800) AND FROM_UNIXTIME(1705316400) GROUP BY 1"},

		// quoting.
		"PostgreSQL: Quoted column keeps its quotes in a filter": {Dialect: _postgreSQLDialect, Query: `WHERE $__timeFilter("createdAt")`, Result: `WHERE ("createdAt") BETWEEN ` + filter},
		"PostgreSQL: Quoted column keeps its quotes in a group":  {Dialect: _postgreSQLDialect, Query: `SELECT $__timeGroup("createdAt", '5m')`, Result: `SELECT floor(EXTRACT(EPOCH FROM "createdAt")/300)*300`},
		"MySQL: Backtick column keeps its quotes":                {Dialect: _mySQLDialect, Query: "SELECT $__time(`createdAt`)", Result: "SELECT UNIX_TIMESTAMP(`createdAt`) AS `time`"},
		"PostgreSQL: Expression is grouped as a whole":           {Dialect: _postgreSQLDialect, Query: "SELECT $__unixEpochGroup(a+b, '5m')", Result: "SELECT floor((a+b)/300)*300"},
		"PostgreSQL: Call inside an argument stays whole":        {Dialect: _postgreSQLDialect, Query: "WHERE $__timeFilter(COALESCE(a, b))", Result: "WHERE (COALESCE(a, b)) BETWEEN " + filter},
		"PostgreSQL: Parenthesis in a string argument":           {Dialect: _postgreSQLDialect, Query: "WHERE $__timeFilter(CASE WHEN host = ')' THEN ts END)", Result: "WHERE (CASE WHEN host = ')' THEN ts END) BETWEEN " + filter},
		"PostgreSQL: Parenthesis in a comment argument":          {Dialect: _postgreSQLDialect, Query: "WHERE $__timeFilter(ts /* ) */)", Result: "WHERE (ts /* ) */) BETWEEN " + filter},

		// regions a macro does not expand in.
		"PostgreSQL: Macro in a string is left as written":               {Dialect: _postgreSQLDialect, Query: "SELECT '$__timeFilter(ts)', 'it''s'", Result: "SELECT '$__timeFilter(ts)', 'it''s'"},
		"PostgreSQL: Macro in a line comment is left as written":         {Dialect: _postgreSQLDialect, Query: "SELECT 1 -- $__timeFilter(\nFROM t", Result: "SELECT 1 -- $__timeFilter(\nFROM t"},
		"PostgreSQL: Macro in a nested block comment is left as written": {Dialect: _postgreSQLDialect, Query: "SELECT /* a /* $__timeFrom() */ $__timeTo() */ 1", Result: "SELECT /* a /* $__timeFrom() */ $__timeTo() */ 1"},
		"PostgreSQL: Macro in a dollar-quoted string is left as written": {Dialect: _postgreSQLDialect, Query: "SELECT $q$ $__timeFrom() $q$, $$x$$", Result: "SELECT $q$ $__timeFrom() $q$, $$x$$"},
		"PostgreSQL: Positional parameter is left as written":            {Dialect: _postgreSQLDialect, Query: "SELECT $1", Result: "SELECT $1"},
		"MySQL: Macro in a hash comment is left as written":              {Dialect: _mySQLDialect, Query: "SELECT 1 # $__timeFrom(", Result: "SELECT 1 # $__timeFrom("},
		"MySQL: Macro in a double-quoted string is left as written":      {Dialect: _mySQLDialect, Query: `SELECT "$__timeFrom()", 'it\'s'`, Result: `SELECT "$__timeFrom()", 'it\'s'`},
		"MySQL: Dashes without a space are not a comment":                {Dialect: _mySQLDialect, Query: "SELECT 1--$__unixEpochFrom()", Result: "SELECT 1--1705312800"},
		"MySQL: Executable comment expands as code":                      {Dialect: _mySQLDialect, Query: "SELECT /*! $__unixEpochFrom() */ 1", Result: "SELECT /*! 1705312800 */ 1"},

		// global macros.
		"PostgreSQL: Global macros":                                       {Dialect: _postgreSQLDialect, Query: "SELECT $__from, $__to, '$__interval', $__interval_ms, '$__range', $__range_s, $__range_ms", Result: "SELECT 1705312800000, 1705316400000, '1m', 60000, '3600s', 3600, 3600000"},
		"PostgreSQL: Braced global macros":                                {Dialect: _postgreSQLDialect, Query: "SELECT ${__from}, '${__interval}', ${__range_s}, '${__to:date}', ${__from:date:seconds}", Result: "SELECT 1705312800000, '1m', 3600, '2024-01-15T11:00:00.000Z', 1705312800"},
		"PostgreSQL: Global macro as a group interval":                    {Dialect: _postgreSQLDialect, Query: "SELECT $__timeGroup(ts, $__interval)", Result: "SELECT floor(EXTRACT(EPOCH FROM ts)/60)*60"},
		"PostgreSQL: Unknown global macro in code is an error":            {Dialect: _postgreSQLDialect, Query: "SELECT $__nope", Err: NewInvalidQueryError("unknown macro $__nope")},
		"PostgreSQL: Unknown braced macro in code is an error":            {Dialect: _postgreSQLDialect, Query: "SELECT ${__from:nope}", Err: NewInvalidQueryError("unknown macro ${__from:nope}")},
		"PostgreSQL: Unknown global macro in a string is left as written": {Dialect: _postgreSQLDialect, Query: "SELECT '$__nope'", Result: "SELECT '$__nope'"},
		"PostgreSQL: Braces that are no macro are left as written":        {Dialect: _postgreSQLDialect, Query: "SELECT '${env}', ${x", Result: "SELECT '${env}', ${x"},

		// errors.
		"PostgreSQL: Interval that overflows is an error":           {Dialect: _postgreSQLDialect, Query: "SELECT $__timeGroup(ts, '144115188075855872w')", Err: NewInvalidQueryError("interval '144115188075855872w' must be a whole number of s, m, h, d or w, at most 10 years")},
		"PostgreSQL: Interval of ten years":                         {Dialect: _postgreSQLDialect, Query: "SELECT $__unixEpochGroup(ts, 3650d)", Result: "SELECT floor((ts)/315360000)*315360000"},
		"PostgreSQL: Interval over ten years is an error":           {Dialect: _postgreSQLDialect, Query: "SELECT $__unixEpochGroup(ts, 3651d)", Err: NewInvalidQueryError("interval 3651d must be a whole number of s, m, h, d or w, at most 10 years")},
		"PostgreSQL: Unclosed call is an error":                     {Dialect: _postgreSQLDialect, Query: "SELECT $__timeFilter(a, b", Err: NewInvalidQueryError("macro $__timeFilter has no closing parenthesis")},
		"PostgreSQL: Nested call is an error":                       {Dialect: _postgreSQLDialect, Query: strings.Repeat("$__timeFilter(", 3), Err: ErrMacroNested},
		"PostgreSQL: Unterminated string is an error":               {Dialect: _postgreSQLDialect, Query: "SELECT 'a", Err: ErrQueryUnterminated},
		"PostgreSQL: Unterminated quoted name is an error":          {Dialect: _postgreSQLDialect, Query: `SELECT "a`, Err: ErrQueryUnterminated},
		"PostgreSQL: Unterminated comment is an error":              {Dialect: _postgreSQLDialect, Query: "SELECT /* /* */", Err: ErrQueryUnterminated},
		"PostgreSQL: Unterminated dollar-quoted string is an error": {Dialect: _postgreSQLDialect, Query: "SELECT $a$ b", Err: ErrQueryUnterminated},
		"MySQL: Backslash before the closing quote is an error":     {Dialect: _mySQLDialect, Query: `SELECT 'a\'`, Err: ErrQueryUnterminated},

		"PostgreSQL: Dollar after a name is part of the name": {Dialect: _postgreSQLDialect, Query: "SELECT a$x$ FROM t WHERE n = '$v' -- $x$", Vars: map[string]string{"v": "' OR 1=1 --"}, Result: "SELECT a$x$ FROM t WHERE n = ''' OR 1=1 --' -- $x$"},
		"PostgreSQL: Dollar inside a name":                    {Dialect: _postgreSQLDialect, Query: "SELECT a$x$b FROM t", Result: "SELECT a$x$b FROM t"},
		"PostgreSQL: Block comment ending an argument":        {Dialect: _postgreSQLDialect, Query: "WHERE $__timeFilter(ts /* note */)", Result: "WHERE (ts /* note */) BETWEEN " + filter},
		"MySQL: Double-quoted interval":                       {Dialect: _mySQLDialect, Query: `SELECT $__timeGroup(ts, "5m")`, Result: "SELECT floor(UNIX_TIMESTAMP(ts)/300)*300"},
		"PostgreSQL: Double-quoted interval is an error":      {Dialect: _postgreSQLDialect, Query: `SELECT $__timeGroup(ts, "5m")`, Err: invalidIntervalError(`"5m"`)},

		// variables.
		"PostgreSQL: Variable in a string is escaped":                                 {Dialect: _postgreSQLDialect, Query: "WHERE host = '$name'", Vars: map[string]string{"name": `o'brien \`}, Result: `WHERE host = 'o''brien \'`},
		"PostgreSQL: Variable in an E string is escaped":                              {Dialect: _postgreSQLDialect, Query: "WHERE host = E'$name'", Vars: map[string]string{"name": `o'b\`}, Result: `WHERE host = E'o''b\\'`},
		"PostgreSQL: Name that ends in E is no E string":                              {Dialect: _postgreSQLDialect, Query: "SELECT typE'$name'", Vars: map[string]string{"name": `\`}, Result: `SELECT typE'\'`},
		"PostgreSQL: Variable in a quoted name is escaped":                            {Dialect: _postgreSQLDialect, Query: `SELECT "$col"`, Vars: map[string]string{"col": `a"b`}, Result: `SELECT "a""b"`},
		"PostgreSQL: Variable in a dollar-quoted string":                              {Dialect: _postgreSQLDialect, Query: "SELECT $$ $v $$", Vars: map[string]string{"v": "it's"}, Result: "SELECT $$ it's $$"},
		"PostgreSQL: Variable holding a dollar in a dollar-quoted string is an error": {Dialect: _postgreSQLDialect, Query: "SELECT $$ $v $$", Vars: map[string]string{"v": "a$$"}, Err: unsafeValueError("$v")},
		"PostgreSQL: Variable holding a NUL is an error":                              {Dialect: _postgreSQLDialect, Query: "SELECT '$v'", Vars: map[string]string{"v": "a\x00"}, Err: unsafeValueError("$v")},
		"PostgreSQL: Bare variables in code":                                          {Dialect: _postgreSQLDialect, Query: "SELECT * FROM t_$env WHERE v > $limit AND w > -$limit LIMIT $limit", Vars: map[string]string{"env": "prod", "limit": "1.5"}, Result: "SELECT * FROM t_prod WHERE v > 1.5 AND w > -1.5 LIMIT 1.5"},
		"PostgreSQL: Variable as a group interval":                                    {Dialect: _postgreSQLDialect, Query: "SELECT $__timeGroup(ts, $step), $__timeGroup(ts, '$step')", Vars: map[string]string{"step": "5m"}, Result: "SELECT floor(EXTRACT(EPOCH FROM ts)/300)*300, floor(EXTRACT(EPOCH FROM ts)/300)*300"},
		"PostgreSQL: Variable that is no bare value in code is an error":              {Dialect: _postgreSQLDialect, Query: "WHERE host = $name", Vars: map[string]string{"name": "a b"}, Err: unsafeValueError("$name")},
		"PostgreSQL: Empty variable in code is an error":                              {Dialect: _postgreSQLDialect, Query: "SELECT 1 $v", Vars: map[string]string{"v": ""}, Err: unsafeValueError("$v")},
		"PostgreSQL: Negative variable after a minus is an error":                     {Dialect: _postgreSQLDialect, Query: "SELECT 1 -$v", Vars: map[string]string{"v": "-1"}, Err: unsafeValueError("$v")},
		"PostgreSQL: Variable right before a quote is an error":                       {Dialect: _postgreSQLDialect, Query: `SELECT $v'\'`, Vars: map[string]string{"v": "E"}, Err: unsafeValueError("$v")},
		"PostgreSQL: Variable in a comment is left as written":                        {Dialect: _postgreSQLDialect, Query: "SELECT 1 -- $v", Vars: map[string]string{"v": "x"}, Result: "SELECT 1 -- $v"},
		"PostgreSQL: Unknown variable is left as written":                             {Dialect: _postgreSQLDialect, Query: "SELECT '$other', $other", Vars: map[string]string{"v": "x"}, Result: "SELECT '$other', $other"},
		"MySQL: Variable in a string is escaped":                                      {Dialect: _mySQLDialect, Query: `WHERE a = '$v' AND b = "$v"`, Vars: map[string]string{"v": `o'b"\`}, Result: `WHERE a = 'o''b"\\' AND b = "o'b""\\"`},
		"MySQL: Variable in a quoted name is escaped":                                 {Dialect: _mySQLDialect, Query: "SELECT `$v`", Vars: map[string]string{"v": "a`b"}, Result: "SELECT `a``b`"},
		"MySQL: Empty variable cannot make a comment":                                 {Dialect: _mySQLDialect, Query: "SELECT 1 --$v \n, 2", Vars: map[string]string{"v": ""}, Err: unsafeValueError("$v")},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, fill, err := expandSQL(c.Query, c.Dialect, _testTimeRange, c.Vars)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
			assert.Equal(t, c.Fill, fill)
		})
	}
}

func Test_isBareValue(t *testing.T) {
	cc := map[string]struct {
		Value  string
		Before string
		Result bool
	}{
		"Number":                        {Value: "1.5", Result: true},
		"Name":                          {Value: "prod_eu", Result: true},
		"Negative number":               {Value: "-1", Before: "x > ", Result: true},
		"Negative after a minus":        {Value: "-1", Before: "x -", Result: false},
		"Exponent":                      {Value: "1e-5", Result: true},
		"Negative exponent with a plus": {Value: "-2.5E+3", Before: "x > ", Result: true},
		"Exponent sign without a digit": {Value: "1e-", Result: false},
		"Exponent sign after a letter":  {Value: "ae-5", Result: false},
		"Exponent sign twice":           {Value: "1e--5", Result: false},
		"Empty":                         {Value: "", Result: false},
		"Lone minus":                    {Value: "-", Result: false},
		"Space":                         {Value: "a b", Result: false},
		"Quote":                         {Value: "a'", Result: false},
		"Comment opener":                {Value: "a/*", Result: false},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, isBareValue(c.Value, c.Before))
		})
	}
}

func Test_unsafeValueError(t *testing.T) {
	t.Parallel()

	assert.Equal(t, NewInvalidQueryError("the value of $env cannot be used here: outside quotes it must be a number, a name or an interval"), unsafeValueError("$env"))
}

// regions returns the kinds of the quoted tokens and comments of q, in
// order.
func regions(q string, d sqlDialect) ([]sqlTokenKind, error) {
	var kinds []sqlTokenKind

	for i := 0; i < len(q); {
		tok, err := d.token(q, i)
		if err != nil {
			return nil, err
		}

		if tok.kind != _sqlTokenCode && tok.kind != _sqlTokenReference {
			kinds = append(kinds, tok.kind)
		}

		i = tok.end
	}

	return kinds, nil
}

func FuzzExpandSQL(f *testing.F) {
	seeds := []string{
		"", "$", "$$", "$__", "$__(", "${__", "'", `"`, "`", "--", "--$v", "/*", "/* /* */", "#", `E'\''`, `'\'`, "$a$ $a",
		"SELECT $__timeGroup(ts, '5m') FROM m WHERE $__timeFilter(\"createdAt\") AND host = '$v' -- $v",
		"SELECT `$v`, \"$v\", $$ $v $$, E'$v', $v, -$v, ${__from:date}",
		"$__timeFilter(CASE WHEN a = ')' THEN ts END) /*! $v */ # $v",
	}

	for _, s := range seeds {
		f.Add(s, "x")
		f.Add(s, `o'b"r\i$e*/n--`)
	}

	f.Fuzz(func(t *testing.T, q, v string) {
		for _, d := range []sqlDialect{_postgreSQLDialect, _mySQLDialect} {
			out, _, err := expandSQL(q, d, _testTimeRange, map[string]string{"v": v})
			if err != nil {
				continue
			}

			// a value must stay inside the region it is written in, so the
			// expansion has the regions of one with a harmless value.
			ref, _, err := expandSQL(q, d, _testTimeRange, map[string]string{"v": "x"})
			if err != nil {
				t.Fatalf("a harmless value failed where %q passed: %v", v, err)
			}

			got, gerr := regions(out, d)
			want, werr := regions(ref, d)

			if (gerr == nil) != (werr == nil) || !slices.Equal(got, want) {
				t.Fatalf("value %q changed the regions of %q:\n%q\n%q", v, q, out, ref)
			}
		}
	})
}
