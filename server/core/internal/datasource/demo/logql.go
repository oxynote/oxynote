package demo

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/prometheus/prometheus/model/labels"
)

// All available line filter operators.
const (
	_lineContains    = "|="
	_lineNotContains = "!="
	_lineMatches     = "|~"
	_lineNotMatches  = "!~"
)

// _errUnsupportedLogQuery is returned when a log query uses more of LogQL
// than the demo answers.
var _errUnsupportedLogQuery = processor.NewInvalidQueryError(
	`the demo data source supports stream selectors, line filters (|=, !=, |~, !~) and label filters such as | detected_level="error"`,
)

// _errEmptyCompatibleSelector is returned when no matcher of a selector
// rules out the empty string, which Loki refuses, with this message.
var _errEmptyCompatibleSelector = processor.NewInvalidQueryError(
	`queries require at least one regexp or equality matcher that does not have an empty-compatible value. ` +
		`For instance, app=~".*" does not meet this requirement, but app=~".+" will`,
)

// _labelFilterOps maps a label filter operator to the matcher it builds.
// Two-character operators come first, so "!=" is not read as "!".
var _labelFilterOps = []struct {
	op string
	mt labels.MatchType
}{
	{op: "=~", mt: labels.MatchRegexp},
	{op: "!~", mt: labels.MatchNotRegexp},
	{op: "!=", mt: labels.MatchNotEqual},
	{op: "==", mt: labels.MatchEqual},
	{op: "=", mt: labels.MatchEqual},
}

// logQuery is a LogQL log query of the subset the demo answers.
type logQuery struct {
	// selector picks the streams.
	selector []*labels.Matcher

	// filters are the pipeline's stages, in order.
	filters []logFilter
}

// matchesStream reports whether the selector picks a stream with the
// given labels.
func (lq logQuery) matchesStream(lbls map[string]string) bool {
	for _, m := range lq.selector {
		if !m.Matches(lbls[m.Name]) {
			return false
		}
	}

	return true
}

// matches reports whether every stage of the pipeline keeps the line.
func (lq logQuery) matches(l processor.LogLine) bool {
	for _, f := range lq.filters {
		if !f.matches(l) {
			return false
		}
	}

	return true
}

// logFilter is one pipeline stage: a line filter, or a label filter
// when matcher is set.
type logFilter struct {
	// op is a line filter's operator.
	op string

	// value is the text a substring line filter looks for.
	value string

	// re is the expression a regexp line filter looks for, unanchored.
	re *regexp.Regexp

	// matcher matches a label filter's label or structured metadata.
	matcher *labels.Matcher
}

// matches reports whether the stage keeps the line.
func (lf logFilter) matches(l processor.LogLine) bool {
	if lf.matcher != nil {
		v, ok := l.Labels[lf.matcher.Name]
		if !ok {
			v = l.Fields[lf.matcher.Name]
		}

		return lf.matcher.Matches(v)
	}

	switch lf.op {
	case _lineContains:
		return strings.Contains(l.Line, lf.value)
	case _lineNotContains:
		return !strings.Contains(l.Line, lf.value)
	case _lineMatches:
		return lf.re.MatchString(l.Line)
	default:
		return !lf.re.MatchString(l.Line)
	}
}

// parseLogQuery parses a log query of the subset the demo answers. The
// selector goes through the PromQL parser, whose matcher syntax LogQL
// shares.
func (c *Client) parseLogQuery(q string) (logQuery, error) {
	q = strings.TrimSpace(q)

	if !strings.HasPrefix(q, "{") {
		return logQuery{}, _errUnsupportedLogQuery
	}

	sel := processor.LokiStreamSelector(q)
	if sel == "" {
		return logQuery{}, processor.NewInvalidQueryError("unterminated stream selector")
	}

	selector, err := c.parser.ParseMetricSelector(sel)
	if err != nil {
		return logQuery{}, processor.NewInvalidQueryError(err.Error())
	}

	if !slices.ContainsFunc(selector, func(m *labels.Matcher) bool { return !m.Matches("") }) {
		return logQuery{}, _errEmptyCompatibleSelector
	}

	filters, err := parseLogFilters(q[len(sel):])
	if err != nil {
		return logQuery{}, err
	}

	return logQuery{
		selector: selector,
		filters:  filters,
	}, nil
}

// parseLogFilters parses the pipeline that follows a stream selector.
func parseLogFilters(s string) ([]logFilter, error) {
	var filters []logFilter

	for s = trimSpace(s); s != ""; s = trimSpace(s) {
		var (
			f   logFilter
			err error
		)

		if op, ok := lineFilterOp(s); ok {
			f, s, err = parseLineFilter(op, trimSpace(s[len(op):]))
		} else if strings.HasPrefix(s, "|") {
			f, s, err = parseLabelFilter(trimSpace(s[1:]))
		} else {
			err = _errUnsupportedLogQuery
		}

		if err != nil {
			return nil, err
		}

		filters = append(filters, f)
	}

	return filters, nil
}

// lineFilterOp returns the line filter operator s starts with, if any.
func lineFilterOp(s string) (string, bool) {
	for _, op := range []string{_lineContains, _lineNotContains, _lineMatches, _lineNotMatches} {
		if strings.HasPrefix(s, op) {
			return op, true
		}
	}

	return "", false
}

// parseLineFilter parses the string a line filter looks for and returns
// the filter and what follows it.
func parseLineFilter(op, s string) (logFilter, string, error) {
	value, rest, err := readString(s)
	if err != nil {
		return logFilter{}, "", err
	}

	f := logFilter{
		op:    op,
		value: value,
	}

	if op == _lineMatches || op == _lineNotMatches {
		f.re, err = regexp.Compile(value)
		if err != nil {
			return logFilter{}, "", processor.NewInvalidQueryError(err.Error())
		}
	}

	return f, rest, nil
}

// parseLabelFilter parses "name op string" and returns the filter and
// what follows it.
func parseLabelFilter(s string) (logFilter, string, error) {
	end := strings.IndexFunc(s, func(r rune) bool {
		return r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if end < 0 {
		end = len(s)
	}

	name := s[:end]
	if name == "" || unicode.IsDigit(rune(name[0])) {
		return logFilter{}, "", _errUnsupportedLogQuery
	}

	s = trimSpace(s[end:])

	for _, lo := range _labelFilterOps {
		if !strings.HasPrefix(s, lo.op) {
			continue
		}

		value, rest, err := readString(trimSpace(s[len(lo.op):]))
		if err != nil {
			return logFilter{}, "", err
		}

		m, err := labels.NewMatcher(lo.mt, name, value)
		if err != nil {
			return logFilter{}, "", processor.NewInvalidQueryError(err.Error())
		}

		return logFilter{matcher: m}, rest, nil
	}

	// a parser such as "| json", or a comparison such as "| status > 500".
	return logFilter{}, "", _errUnsupportedLogQuery
}

// readString reads the double-quoted or backtick-quoted string s starts
// with, and returns its value and what follows it.
func readString(s string) (string, string, error) {
	if s == "" || (s[0] != '"' && s[0] != '`') {
		return "", "", _errUnsupportedLogQuery
	}

	if s[0] == '`' {
		end := strings.IndexByte(s[1:], '`')
		if end < 0 {
			return "", "", processor.NewInvalidQueryError("unterminated raw string")
		}

		return s[1 : end+1], s[end+2:], nil
	}

	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			value, err := strconv.Unquote(s[:i+1])
			if err != nil {
				return "", "", processor.NewInvalidQueryError(err.Error())
			}

			return value, s[i+1:], nil
		}
	}

	return "", "", processor.NewInvalidQueryError("unterminated string")
}

// trimSpace returns s without leading white space.
func trimSpace(s string) string {
	return strings.TrimLeftFunc(s, unicode.IsSpace)
}
