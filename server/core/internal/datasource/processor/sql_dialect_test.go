package processor

import (
	"strings"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
)

func Test_sqlDialect_token(t *testing.T) {
	cc := map[string]struct {
		Dialect sqlDialect
		Query   string
		Index   int
		Result  sqlToken
		Err     error
	}{
		"Code byte":                                {Dialect: _postgreSQLDialect, Query: "SELECT", Result: sqlToken{kind: _sqlTokenCode, end: 1}},
		"String":                                   {Dialect: _postgreSQLDialect, Query: "'a''b' x", Result: sqlToken{kind: _sqlTokenString, end: 6, delim: 1, quote: '\''}},
		"MySQL string takes backslashes":           {Dialect: _mySQLDialect, Query: `'a\'b'`, Result: sqlToken{kind: _sqlTokenString, end: 6, delim: 1, quote: '\'', backslash: true}},
		"MySQL double quote opens a string":        {Dialect: _mySQLDialect, Query: `"a"`, Result: sqlToken{kind: _sqlTokenString, end: 3, delim: 1, quote: '"', backslash: true}},
		"E string takes backslashes":               {Dialect: _postgreSQLDialect, Query: `E'\''`, Index: 1, Result: sqlToken{kind: _sqlTokenString, start: 1, end: 5, delim: 1, quote: '\'', backslash: true}},
		"E at the end of a name opens no E string": {Dialect: _postgreSQLDialect, Query: `xE'\'`, Index: 2, Result: sqlToken{kind: _sqlTokenString, start: 2, end: 5, delim: 1, quote: '\''}},
		"Quoted name":                              {Dialect: _postgreSQLDialect, Query: `"a""b"`, Result: sqlToken{kind: _sqlTokenName, end: 6, delim: 1, quote: '"'}},
		"Dash comment":                             {Dialect: _postgreSQLDialect, Query: "-- x\nSELECT", Result: sqlToken{kind: _sqlTokenComment, end: 4}},
		"MySQL dash without a space is code":       {Dialect: _mySQLDialect, Query: "--x", Result: sqlToken{kind: _sqlTokenCode, end: 1}},
		"MySQL dash at the end opens a comment":    {Dialect: _mySQLDialect, Query: "--", Result: sqlToken{kind: _sqlTokenComment, end: 2}},
		"Hash comment":                             {Dialect: _mySQLDialect, Query: "# x", Result: sqlToken{kind: _sqlTokenComment, end: 3}},
		"Hash in Postgres is code":                 {Dialect: _postgreSQLDialect, Query: "#", Result: sqlToken{kind: _sqlTokenCode, end: 1}},
		"Executable comment opener is code":        {Dialect: _mySQLDialect, Query: "/*! x */", Result: sqlToken{kind: _sqlTokenCode, end: 3}},
		"Block comment":                            {Dialect: _postgreSQLDialect, Query: "/* x */ y", Result: sqlToken{kind: _sqlTokenComment, end: 7}},
		"Dollar string":                            {Dialect: _postgreSQLDialect, Query: "$a$ x $a$", Result: sqlToken{kind: _sqlTokenDollarString, end: 9, delim: 3}},
		"Reference":                                {Dialect: _postgreSQLDialect, Query: "$env x", Result: sqlToken{kind: _sqlTokenReference, end: 4}},
		"Unterminated string":                      {Dialect: _postgreSQLDialect, Query: "'a", Err: ErrQueryUnterminated},
		"Unterminated block comment":               {Dialect: _postgreSQLDialect, Query: "/* a", Err: ErrQueryUnterminated},
		"Unclosed braced macro":                    {Dialect: _postgreSQLDialect, Query: "${__from", Err: ErrMacroUnclosedBrace},
		"Token at an offset":                       {Dialect: _postgreSQLDialect, Query: "a 'b'", Index: 2, Result: sqlToken{kind: _sqlTokenString, start: 2, end: 5, delim: 1, quote: '\''}},
		"Postgres double quote opens no string":    {Dialect: _postgreSQLDialect, Query: `"a"`, Result: sqlToken{kind: _sqlTokenName, end: 3, delim: 1, quote: '"'}},
		"MySQL backtick opens a quoted name":       {Dialect: _mySQLDialect, Query: "`a`", Result: sqlToken{kind: _sqlTokenName, end: 3, delim: 1, quote: '`'}},
		"Dash comment at the end of the query":     {Dialect: _postgreSQLDialect, Query: "-- x", Result: sqlToken{kind: _sqlTokenComment, end: 4}},
		"Executable comment opener in Postgres":    {Dialect: _postgreSQLDialect, Query: "/*! x */", Result: sqlToken{kind: _sqlTokenComment, end: 8}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.Dialect.token(c.Query, c.Index)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_sqlDialect_dollarToken(t *testing.T) {
	cc := map[string]struct {
		Dialect sqlDialect
		Query   string
		Index   int
		Result  sqlToken
		Err     error
	}{
		"Dollar string":                        {Dialect: _postgreSQLDialect, Query: "$$x$$", Result: sqlToken{kind: _sqlTokenDollarString, end: 5, delim: 2}},
		"Dollar after a name is a reference":   {Dialect: _postgreSQLDialect, Query: "a$x$", Index: 1, Result: sqlToken{kind: _sqlTokenReference, start: 1, end: 3}},
		"Dollar after a dollar is a reference": {Dialect: _postgreSQLDialect, Query: "x$$a$", Index: 2, Result: sqlToken{kind: _sqlTokenReference, start: 2, end: 4}},
		"Macro opens no dollar string":         {Dialect: _postgreSQLDialect, Query: "$__x$ ", Result: sqlToken{kind: _sqlTokenReference, end: 4}},
		"MySQL has no dollar strings":          {Dialect: _mySQLDialect, Query: "$a$", Result: sqlToken{kind: _sqlTokenReference, end: 2}},
		"Unterminated dollar string":           {Dialect: _postgreSQLDialect, Query: "$a$ x", Err: ErrQueryUnterminated},
		"Unclosed braced macro":                {Dialect: _postgreSQLDialect, Query: "${__from", Err: ErrMacroUnclosedBrace},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.Dialect.dollarToken(c.Query, c.Index)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_sqlDialect_blockComment(t *testing.T) {
	cc := map[string]struct {
		Dialect sqlDialect
		Query   string
		Index   int
		Result  sqlToken
		Err     error
	}{
		"Comment":                      {Dialect: _postgreSQLDialect, Query: "/* a */ b", Result: sqlToken{kind: _sqlTokenComment, end: 7}},
		"Comment at an offset":         {Dialect: _postgreSQLDialect, Query: "a /**/", Index: 2, Result: sqlToken{kind: _sqlTokenComment, start: 2, end: 6}},
		"Nested comment":               {Dialect: _postgreSQLDialect, Query: "/* /* */ */ x", Result: sqlToken{kind: _sqlTokenComment, end: 11}},
		"MySQL comments do not nest":   {Dialect: _mySQLDialect, Query: "/* /* */ */", Result: sqlToken{kind: _sqlTokenComment, end: 8}},
		"Unterminated comment":         {Dialect: _postgreSQLDialect, Query: "/* a", Err: ErrQueryUnterminated},
		"Unterminated nested comment":  {Dialect: _postgreSQLDialect, Query: "/* /* */", Err: ErrQueryUnterminated},
		"Opener that is also a closer": {Dialect: _postgreSQLDialect, Query: "/*/", Err: ErrQueryUnterminated},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.Dialect.blockComment(c.Query, c.Index)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_sqlDialect_referenceLength(t *testing.T) {
	cc := map[string]struct {
		Dialect sqlDialect
		Input   string
		Result  int
		Err     error
	}{
		"Word":                           {Dialect: _postgreSQLDialect, Input: "$env + 1", Result: 4},
		"Lone dollar":                    {Dialect: _postgreSQLDialect, Input: "$ 1", Result: 1},
		"Braces":                         {Dialect: _postgreSQLDialect, Input: "${__from:date} x", Result: 14},
		"Braces with a non-ASCII format": {Dialect: _postgreSQLDialect, Input: "${__from:date:€}", Result: 18},
		"Braces with a format":           {Dialect: _postgreSQLDialect, Input: "${__from:date:MMM D, YYYY (HH)}", Result: 31},
		"Braces that are no macro":       {Dialect: _postgreSQLDialect, Input: "${env", Result: 1},
		"Unclosed macro":                 {Dialect: _postgreSQLDialect, Input: "${__from", Err: ErrMacroUnclosedBrace},
		"Macro closed too far":           {Dialect: _postgreSQLDialect, Input: "${__" + strings.Repeat("a", 124) + "}", Err: ErrMacroUnclosedBrace},
		"Longest macro":                  {Dialect: _postgreSQLDialect, Input: "${__" + strings.Repeat("a", 123) + "}", Result: 128},
		"Braces around a quote":          {Dialect: _postgreSQLDialect, Input: "${'}'", Result: 1},
		"Braces around a quoted name":    {Dialect: _mySQLDialect, Input: "${`}`", Result: 1},
		"Braces around a newline":        {Dialect: _postgreSQLDialect, Input: "${a\n}", Result: 1},
		"Braces around a dash comment":   {Dialect: _postgreSQLDialect, Input: "${a -- }", Result: 1},
		"Braces around a block comment":  {Dialect: _postgreSQLDialect, Input: "${a /* } */", Result: 1},
		"Braces around a hash comment":   {Dialect: _mySQLDialect, Input: "${a # }", Result: 1},
		"Hash in Postgres braces":        {Dialect: _postgreSQLDialect, Input: "${a # }", Result: 7},
		"Braces around a dollar":         {Dialect: _postgreSQLDialect, Input: "${a $v}", Result: 1},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.Dialect.referenceLength(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_sqlDialect_bracedLength(t *testing.T) {
	cc := map[string]struct {
		Dialect sqlDialect
		Input   string
		Result  int
	}{
		"Closed":                    {Dialect: _postgreSQLDialect, Input: "${a} x", Result: 4},
		"Unclosed":                  {Dialect: _postgreSQLDialect, Input: "${a", Result: 0},
		"Closed at the window edge": {Dialect: _postgreSQLDialect, Input: "${" + strings.Repeat("a", 125) + "}", Result: 128},
		"Closed past the window":    {Dialect: _postgreSQLDialect, Input: "${" + strings.Repeat("a", 126) + "}", Result: 0},
		"String quote":              {Dialect: _postgreSQLDialect, Input: "${'}", Result: 0},
		"MySQL double quote":        {Dialect: _mySQLDialect, Input: `${"}`, Result: 0},
		"Name quote":                {Dialect: _postgreSQLDialect, Input: `${"}`, Result: 0},
		"Control byte":              {Dialect: _postgreSQLDialect, Input: "${a\x01}", Result: 0},
		"Delete byte":               {Dialect: _postgreSQLDialect, Input: "${a\x7f}", Result: 0},
		"Dollar":                    {Dialect: _postgreSQLDialect, Input: "${a$}", Result: 0},
		"Brace":                     {Dialect: _postgreSQLDialect, Input: "${a{}", Result: 0},
		"MySQL hash":                {Dialect: _mySQLDialect, Input: "${#}", Result: 0},
		"Postgres hash":             {Dialect: _postgreSQLDialect, Input: "${#}", Result: 4},
		"Dash comment opener":       {Dialect: _postgreSQLDialect, Input: "${--}", Result: 0},
		"Single dash":               {Dialect: _postgreSQLDialect, Input: "${-}", Result: 4},
		"Block comment opener":      {Dialect: _postgreSQLDialect, Input: "${/*}", Result: 0},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Dialect.bracedLength(c.Input))
		})
	}
}

func Test_quotedToken(t *testing.T) {
	cc := map[string]struct {
		Query     string
		Index     int
		Kind      sqlTokenKind
		Backslash bool
		Result    sqlToken
		Err       error
	}{
		"String":                          {Query: "'a' b", Kind: _sqlTokenString, Result: sqlToken{kind: _sqlTokenString, end: 3, delim: 1, quote: '\''}},
		"String at an offset":             {Query: "x 'a'", Index: 2, Kind: _sqlTokenString, Result: sqlToken{kind: _sqlTokenString, start: 2, end: 5, delim: 1, quote: '\''}},
		"Doubled quote":                   {Query: "'a''b'", Kind: _sqlTokenString, Result: sqlToken{kind: _sqlTokenString, end: 6, delim: 1, quote: '\''}},
		"Backslash escapes the quote":     {Query: `'a\'b'`, Kind: _sqlTokenString, Backslash: true, Result: sqlToken{kind: _sqlTokenString, end: 6, delim: 1, quote: '\'', backslash: true}},
		"Backslash without escapes":       {Query: `'a\'b'`, Kind: _sqlTokenString, Result: sqlToken{kind: _sqlTokenString, end: 4, delim: 1, quote: '\''}},
		"Quoted name":                     {Query: "`a`", Kind: _sqlTokenName, Result: sqlToken{kind: _sqlTokenName, end: 3, delim: 1, quote: '`'}},
		"Unterminated":                    {Query: "'a", Kind: _sqlTokenString, Err: ErrQueryUnterminated},
		"Doubled quote at the end":        {Query: "'a''", Kind: _sqlTokenString, Err: ErrQueryUnterminated},
		"Backslash before the last quote": {Query: `'a\'`, Kind: _sqlTokenString, Backslash: true, Err: ErrQueryUnterminated},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := quotedToken(c.Query, c.Index, c.Kind, c.Backslash)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_dollarString(t *testing.T) {
	cc := map[string]struct {
		Query  string
		Index  int
		Tag    string
		Result sqlToken
		Err    error
	}{
		"Empty tag":                  {Query: "$$a$$", Tag: "$$", Result: sqlToken{kind: _sqlTokenDollarString, end: 5, delim: 2}},
		"Named tag":                  {Query: "$t$ a $t$ b", Tag: "$t$", Result: sqlToken{kind: _sqlTokenDollarString, end: 9, delim: 3}},
		"String at an offset":        {Query: "x $$a$$", Index: 2, Tag: "$$", Result: sqlToken{kind: _sqlTokenDollarString, start: 2, end: 7, delim: 2}},
		"Unterminated":               {Query: "$t$ a", Tag: "$t$", Err: ErrQueryUnterminated},
		"Another tag does not close": {Query: "$t$ a $u$", Tag: "$t$", Err: ErrQueryUnterminated},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := dollarString(c.Query, c.Index, c.Tag)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_dollarTag(t *testing.T) {
	cc := map[string]struct {
		Input  string
		Result string
	}{
		"Empty tag":    {Input: "$$ x $$", Result: "$$"},
		"Named tag":    {Input: "$body$ x", Result: "$body$"},
		"Parameter":    {Input: "$1$", Result: ""},
		"Variable":     {Input: "$env ", Result: ""},
		"Lone dollar":  {Input: "$", Result: ""},
		"Unclosed tag": {Input: "$abc", Result: ""},
		"Not a dollar": {Input: "abc$", Result: ""},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, dollarTag(c.Input))
		})
	}
}

func Test_isWordByte(t *testing.T) {
	cc := map[string]struct {
		Input  byte
		Result bool
	}{
		"Underscore":       {Input: '_', Result: true},
		"Digit":            {Input: '7', Result: true},
		"Lowercase letter": {Input: 'q', Result: true},
		"Uppercase letter": {Input: 'Q', Result: true},
		"Dollar":           {Input: '$', Result: false},
		"Dot":              {Input: '.', Result: false},
		"Non-ASCII byte":   {Input: 0xc3, Result: false},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, isWordByte(c.Input))
		})
	}
}

func Test_isSpace(t *testing.T) {
	cc := map[string]struct {
		Input  byte
		Result bool
	}{
		"Space":           {Input: ' ', Result: true},
		"Tab":             {Input: '\t', Result: true},
		"Newline":         {Input: '\n', Result: true},
		"Carriage return": {Input: '\r', Result: true},
		"Form feed":       {Input: '\f', Result: true},
		"Vertical tab":    {Input: '\v', Result: true},
		"Letter":          {Input: 'x', Result: false},
		"Dash":            {Input: '-', Result: false},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, isSpace(c.Input))
		})
	}
}
