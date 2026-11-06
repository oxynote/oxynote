package processor

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// ErrMacroNested is returned when a macro call is the argument of another.
var ErrMacroNested = NewInvalidQueryError("a macro call cannot be the argument of another macro")

// sqlExpander expands the macros and variables of one query.
type sqlExpander struct {
	q    string
	d    sqlDialect
	tr   TimeRange
	vars map[string]string

	// fill specifies the fill mode of the last group macro that has one.
	fill *sqlFill
}

// expand writes the token at q[i] to b, expanded, and returns the offset
// after it. inArgs tells that the token is inside a macro call.
func (e *sqlExpander) expand(b *strings.Builder, i int, inArgs bool) (int, error) {
	t, err := e.d.token(e.q, i)
	if err != nil {
		return 0, err
	}

	switch t.kind {
	case _sqlTokenString, _sqlTokenName, _sqlTokenDollarString:
		return t.end, e.expandQuoted(b, t)
	case _sqlTokenReference:
		return e.expandReference(b, t, inArgs)
	default:
		b.WriteString(e.q[t.start:t.end])

		return t.end, nil
	}
}

// expandQuoted writes the quoted token t to b with the variables and
// global macros inside it escaped for it. A macro call inside it stays as
// written, since it is text the author meant.
func (e *sqlExpander) expandQuoted(b *strings.Builder, t sqlToken) error {
	body := t.end - t.delim

	b.WriteString(e.q[t.start : t.start+t.delim])

	for j := t.start + t.delim; j < body; {
		switch {
		case t.backslash && e.q[j] == '\\':
			b.WriteString(e.q[j : j+2])
			j += 2
		case e.q[j] == '$':
			length, err := e.d.referenceLength(e.q[j:body])
			if err != nil {
				return err
			}

			n := j + length

			if err := e.writeQuotedValue(b, e.q[j:n], t); err != nil {
				return err
			}

			j = n
		default:
			b.WriteByte(e.q[j])
			j++
		}
	}

	b.WriteString(e.q[body:t.end])

	return nil
}

// writeQuotedValue writes the value of reference r, written inside the
// quoted token t, escaped for t. A reference to nothing stays as written.
func (e *sqlExpander) writeQuotedValue(b *strings.Builder, r string, t sqlToken) error {
	v, ok := e.lookup(r)
	if !ok {
		b.WriteString(r)

		return nil
	}

	switch {
	case strings.ContainsRune(v, 0):
		return unsafeValueError(r)
	case t.kind == _sqlTokenDollarString:
		// a dollar-quoted string has no escape, so a value holding a $
		// could close it.
		if strings.ContainsRune(v, '$') {
			return unsafeValueError(r)
		}
	default:
		if t.backslash {
			v = strings.ReplaceAll(v, `\`, `\\`)
		}

		quote := string(t.quote)
		v = strings.ReplaceAll(v, quote, quote+quote)
	}

	b.WriteString(v)

	return nil
}

// expandReference writes the reference token t, written in code, to b and
// returns the offset after it. A macro call takes its arguments with it.
func (e *sqlExpander) expandReference(b *strings.Builder, t sqlToken, inArgs bool) (int, error) {
	r := e.q[t.start:t.end]
	global := strings.HasPrefix(r, _macroPrefix) || strings.HasPrefix(r, _bracedMacroPrefix)

	if strings.HasPrefix(r, _macroPrefix) && t.end < len(e.q) && e.q[t.end] == '(' {
		if inArgs {
			return 0, ErrMacroNested
		}

		return e.expandCall(b, r[len(_macroPrefix):], t.end)
	}

	v, ok := e.lookup(r)

	switch {
	case ok && global:
		// a global macro's value comes from the author, not a viewer.
		b.WriteString(v)
	case ok:
		// a value right before a quote or a $ would change how that lexes,
		// as the E of an E'' string or a $ it takes out of a name.
		next := byte(0)
		if t.end < len(e.q) {
			next = e.q[t.end]
		}

		if !isBareValue(v, b.String()) || (next != 0 && strings.IndexByte(e.d.stringQuotes+string(e.d.nameQuote)+"$", next) >= 0) {
			return 0, unsafeValueError(r)
		}

		b.WriteString(v)
	case global:
		return 0, NewInvalidQueryError("unknown macro " + r)
	default:
		b.WriteString(r)
	}

	return t.end, nil
}

// expandCall writes the expansion of the macro call named name, whose
// argument list opens at q[open], and returns the offset after its closing
// parenthesis.
func (e *sqlExpander) expandCall(b *strings.Builder, name string, open int) (int, error) {
	var (
		args  []string
		arg   strings.Builder
		depth int
	)

	for i := open + 1; i < len(e.q); {
		switch c := e.q[i]; {
		case c == '(':
			depth++

			arg.WriteByte(c)

			i++
		case c == ')' && depth > 0:
			depth--

			arg.WriteByte(c)

			i++
		case c == ')':
			out, err := e.render(name, append(args, arg.String()))
			if err != nil {
				return 0, err
			}

			b.WriteString(out)

			return i + 1, nil
		case c == ',' && depth == 0:
			args = append(args, arg.String())
			arg.Reset()

			i++
		default:
			n, err := e.expand(&arg, i, true)
			if err != nil {
				return 0, err
			}

			i = n
		}
	}

	return 0, NewInvalidQueryError(fmt.Sprintf("macro $__%s has no closing parenthesis", name))
}

// render returns the SQL of the macro call named name.
func (e *sqlExpander) render(name string, args []string) (string, error) {
	m, ok := _sqlMacros[name]
	if !ok {
		return "", NewInvalidQueryError("unknown macro $__" + name)
	}

	for i := range args {
		args[i] = strings.TrimSpace(args[i])
	}

	if len(args) == 1 && args[0] == "" {
		args = nil
	}

	if (len(args) != m.args && (!m.fill || len(args) != m.args+1)) || slices.Contains(args, "") {
		count, unit := strconv.Itoa(m.args), "arguments"

		switch {
		case m.fill:
			count += " or " + strconv.Itoa(m.args+1)
		case m.args == 1:
			unit = "argument"
		}

		return "", NewInvalidQueryError(fmt.Sprintf("macro $__%s takes %s %s", name, count, unit))
	}

	return m.render(e, name, args)
}

// lookup returns the value of reference r: a global macro such as $__from
// or ${__from:date}, or a variable.
func (e *sqlExpander) lookup(r string) (string, bool) {
	switch {
	case strings.HasPrefix(r, _bracedMacroPrefix) && strings.HasSuffix(r, "}"):
		return e.tr.bracedMacro(r[len(_bracedMacroPrefix) : len(r)-1])
	case strings.HasPrefix(r, _macroPrefix):
		return e.tr.globalMacro(r[len(_macroPrefix):])
	default:
		v, ok := e.vars[r[1:]]

		return v, ok
	}
}

// expandSQL returns q with its macros and variables expanded for dialect d
// over tr, and the fill mode of its group macro, if any.
//
// A macro call expands only in code. A variable or a global macro is
// escaped for the string or quoted name it is written in; in code a
// variable only takes a value that cannot change the query's structure.
// Whatever cannot be expanded that way is an error, never left as written.
func expandSQL(q string, d sqlDialect, tr TimeRange, vars map[string]string) (string, *sqlFill, error) {
	e := &sqlExpander{
		q:    q,
		d:    d,
		tr:   tr,
		vars: vars,
	}

	var b strings.Builder

	for i := 0; i < len(q); {
		n, err := e.expand(&b, i, false)
		if err != nil {
			return "", nil, err
		}

		i = n
	}

	return b.String(), e.fill, nil
}

// isBareValue reports whether a variable's value v can stand in code as it
// is: a number such as 1.5 or 1e-5, a name or an interval, which cannot
// open a string or a comment. An empty value is refused, as it can join the
// code around it into a comment, such as MySQL's "--$v ", and so is a
// leading minus right after another.
func isBareValue(v, before string) bool {
	s, negative := strings.CutPrefix(v, "-")
	if s == "" || (negative && strings.HasSuffix(before, "-")) {
		return false
	}

	for i := range len(s) {
		// the sign of an exponent sits between digits, so it never makes
		// a -- comment.
		exponent := (s[i] == '-' || s[i] == '+') && i >= 2 && i+1 < len(s) &&
			(s[i-1] == 'e' || s[i-1] == 'E') &&
			s[i-2] >= '0' && s[i-2] <= '9' && s[i+1] >= '0' && s[i+1] <= '9'

		if !isWordByte(s[i]) && s[i] != '.' && !exponent {
			return false
		}
	}

	return true
}

// unsafeValueError returns the error for a variable whose value cannot go
// where reference r is written.
func unsafeValueError(r string) error {
	return NewInvalidQueryError(fmt.Sprintf("the value of %s cannot be used here: outside quotes it must be a number, a name or an interval", r))
}
