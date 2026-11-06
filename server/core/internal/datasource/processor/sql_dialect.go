package processor

import (
	"strings"
	"time"
)

const (
	// _maxBracedReferenceLength bounds the search for the closing brace of
	// a ${...} reference, so a query full of unclosed ones stays linear.
	_maxBracedReferenceLength = 128

	// _macroPrefix opens a global macro or a macro call.
	_macroPrefix = "$__"

	// _bracedMacroPrefix opens a global macro written in braces.
	_bracedMacroPrefix = "${__"
)

const (
	// _sqlTokenCode is one byte of SQL code.
	_sqlTokenCode sqlTokenKind = iota + 1

	// _sqlTokenString is a string literal.
	_sqlTokenString

	// _sqlTokenName is a quoted name.
	_sqlTokenName

	// _sqlTokenDollarString is a Postgres dollar-quoted string.
	_sqlTokenDollarString

	// _sqlTokenComment is a comment.
	_sqlTokenComment

	// _sqlTokenReference is a $ reference written in code: a macro, a
	// variable or a parameter such as $1.
	_sqlTokenReference
)

var (
	// ErrMacroUnclosedBrace is returned when a ${__ macro has no closing
	// brace.
	ErrMacroUnclosedBrace = NewInvalidQueryError("a ${__ macro has no closing brace")

	// ErrQueryUnterminated is returned when a string, quoted name or
	// comment in a query never closes.
	ErrQueryUnterminated = NewInvalidQueryError("query has an unterminated string, quoted name or comment")
)

// sqlTokenKind tells what a token of a query is.
type sqlTokenKind int

// sqlToken is one lexical unit of a query, q[start:end].
type sqlToken struct {
	// kind specifies what the token is.
	kind sqlTokenKind

	// start specifies the offset of the token's first byte.
	start int

	// end specifies the offset just past the token's last byte.
	end int

	// delim specifies the length of a quoted token's opening delimiter,
	// which is also the length of its closing one.
	delim int

	// quote specifies the byte a quoted token doubles to hold itself.
	quote byte

	// backslash indicates that a backslash escapes the next byte.
	backslash bool
}

// sqlDialect holds what differs between SQL dialects: how a query is lexed
// and how the time macros render.
type sqlDialect struct {
	// stringQuotes specifies the bytes that open a string literal.
	stringQuotes string

	// nameQuote specifies the byte that opens a quoted name.
	nameQuote byte

	// backslashStrings indicates that a backslash escapes inside every
	// string literal.
	backslashStrings bool

	// eStrings indicates that an E'...' string takes backslash escapes.
	eStrings bool

	// dollarStrings indicates that $tag$...$tag$ is a string literal.
	dollarStrings bool

	// nestedComments indicates that block comments nest.
	nestedComments bool

	// spacedDashComments indicates that -- opens a comment only before
	// whitespace or the end of the query.
	spacedDashComments bool

	// hashComments indicates that # opens a line comment.
	hashComments bool

	// executableComments indicates that the body of /*! ... */ is code.
	executableComments bool

	// epoch renders the unix seconds of a time expression.
	epoch func(expr string) string

	// between renders expr lying between two times.
	between func(expr string, from, to time.Time) string

	// timestamp renders a time.
	timestamp func(t time.Time) string

	// timeAlias specifies the clause that names the time column.
	timeAlias string
}

// token returns the token of q that starts at q[i].
func (d sqlDialect) token(q string, i int) (sqlToken, error) {
	rest := q[i:]
	c := q[i]

	switch {
	case strings.IndexByte(d.stringQuotes, c) >= 0:
		// the E of an E'' string must start its own word, or it is the
		// last letter of a name.
		eString := d.eStrings && i > 0 && (q[i-1] == 'E' || q[i-1] == 'e') &&
			(i < 2 || !isWordByte(q[i-2]))

		return quotedToken(q, i, _sqlTokenString, d.backslashStrings || eString)
	case c == d.nameQuote:
		return quotedToken(q, i, _sqlTokenName, false)
	case strings.HasPrefix(rest, "--") && (!d.spacedDashComments || len(rest) == 2 || isSpace(rest[2])),
		c == '#' && d.hashComments:
		end := strings.IndexByte(rest, '\n')
		if end < 0 {
			end = len(rest)
		}

		return sqlToken{kind: _sqlTokenComment, start: i, end: i + end}, nil
	case strings.HasPrefix(rest, "/*!") && d.executableComments:
		return sqlToken{kind: _sqlTokenCode, start: i, end: i + len("/*!")}, nil
	case strings.HasPrefix(rest, "/*"):
		return d.blockComment(q, i)
	case c == '$':
		return d.dollarToken(q, i)
	default:
		return sqlToken{kind: _sqlTokenCode, start: i, end: i + 1}, nil
	}
}

// dollarToken returns the token that opens with the $ at q[i]: a
// dollar-quoted string or a reference.
func (d sqlDialect) dollarToken(q string, i int) (sqlToken, error) {
	rest := q[i:]

	// a $ right after a name byte is part of the name, so it opens no
	// dollar-quoted string there.
	inName := i > 0 && (isWordByte(q[i-1]) || q[i-1] == '$')

	if tag := dollarTag(rest); d.dollarStrings && tag != "" && !inName && !strings.HasPrefix(rest, _macroPrefix) {
		return dollarString(q, i, tag)
	}

	n, err := d.referenceLength(rest)
	if err != nil {
		return sqlToken{}, err
	}

	return sqlToken{kind: _sqlTokenReference, start: i, end: i + n}, nil
}

// blockComment returns the block comment that opens at q[i].
func (d sqlDialect) blockComment(q string, i int) (sqlToken, error) {
	depth := 0

	for j := i; j+1 < len(q); {
		switch {
		case q[j] == '/' && q[j+1] == '*' && (depth == 0 || d.nestedComments):
			depth++
			j += 2
		case q[j] == '*' && q[j+1] == '/':
			depth--
			j += 2

			if depth == 0 {
				return sqlToken{kind: _sqlTokenComment, start: i, end: j}, nil
			}
		default:
			j++
		}
	}

	return sqlToken{}, ErrQueryUnterminated
}

// referenceLength returns the length of the $ reference at the start of s:
// ${...}, $word, or a lone $. A ${__ macro whose braces do not close is an
// error.
func (d sqlDialect) referenceLength(s string) (int, error) {
	if strings.HasPrefix(s, "${") {
		if n := d.bracedLength(s); n > 0 {
			return n, nil
		}

		if strings.HasPrefix(s, _bracedMacroPrefix) {
			return 0, ErrMacroUnclosedBrace
		}

		return 1, nil
	}

	n := 1
	for n < len(s) && isWordByte(s[n]) {
		n++
	}

	return n, nil
}

// bracedLength returns the length of the ${...} reference at the start of
// s, or 0 when its braces do not close first. Braces hold no quote, $,
// brace, control byte or comment opener, so a reference never reaches into
// a quote or a comment the query goes on with.
func (d sqlDialect) bracedLength(s string) int {
	for k := 2; k < min(len(s), _maxBracedReferenceLength); k++ {
		switch c := s[k]; {
		case c == '}':
			return k + 1
		case c < ' ', c == '\x7f', c == '$', c == '{', c == d.nameQuote,
			strings.IndexByte(d.stringQuotes, c) >= 0,
			c == '#' && d.hashComments,
			strings.HasPrefix(s[k:], "--"), strings.HasPrefix(s[k:], "/*"):
			return 0
		}
	}

	return 0
}

// quotedToken returns the string or quoted name that opens at q[i].
func quotedToken(q string, i int, kind sqlTokenKind, backslash bool) (sqlToken, error) {
	quote := q[i]

	for j := i + 1; j < len(q); j++ {
		switch {
		case backslash && q[j] == '\\':
			j++
		case q[j] == quote && j+1 < len(q) && q[j+1] == quote:
			j++
		case q[j] == quote:
			return sqlToken{
				kind:      kind,
				start:     i,
				end:       j + 1,
				delim:     1,
				quote:     quote,
				backslash: backslash,
			}, nil
		}
	}

	return sqlToken{}, ErrQueryUnterminated
}

// dollarString returns the dollar-quoted string that opens at q[i] with
// tag.
func dollarString(q string, i int, tag string) (sqlToken, error) {
	end := strings.Index(q[i+len(tag):], tag)
	if end < 0 {
		return sqlToken{}, ErrQueryUnterminated
	}

	return sqlToken{
		kind:  _sqlTokenDollarString,
		start: i,
		end:   i + len(tag) + end + len(tag),
		delim: len(tag),
	}, nil
}

// dollarTag returns the dollar-quote tag at the start of s, such as $$ or
// $body$, or "" when s does not open one.
func dollarTag(s string) string {
	if len(s) < 2 || s[0] != '$' {
		return ""
	}

	if s[1] == '$' {
		return "$$"
	}

	if s[1] >= '0' && s[1] <= '9' {
		return ""
	}

	for k := 1; k < len(s); k++ {
		if s[k] == '$' {
			return s[:k+1]
		}

		if !isWordByte(s[k]) {
			return ""
		}
	}

	return ""
}

// isWordByte reports whether c can appear in a name.
func isWordByte(c byte) bool {
	return c == '_' ||
		(c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z')
}

// isSpace reports whether c is whitespace that closes MySQL's "-- " opener.
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}
