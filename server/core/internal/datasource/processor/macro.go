package processor

import "strings"

// _macroPrefix opens a macro invocation: $__name(args).
const _macroPrefix = "$__"

// expandMacros rewrites every $__name(args) invocation in q through expand,
// which returns the replacement and whether it handled the macro; an
// unhandled macro is left as it was.
//
// The closing parenthesis is matched by depth rather than by the first ")",
// so an argument that is itself a call — $__timeFilter(COALESCE(a, b)) —
// survives instead of being cut in half.
func expandMacros(q string, expand func(name string, args []string) (string, bool)) string {
	var b strings.Builder

	for i := 0; i < len(q); {
		if !strings.HasPrefix(q[i:], _macroPrefix) {
			b.WriteByte(q[i])

			i++

			continue
		}

		name, args, end, ok := scanMacro(q, i)
		if !ok {
			b.WriteByte(q[i])

			i++

			continue
		}

		replacement, handled := expand(name, args)
		if !handled {
			b.WriteString(q[i:end])

			i = end

			continue
		}

		b.WriteString(replacement)

		i = end
	}

	return b.String()
}

// scanMacro reads the macro invocation starting at q[i] and returns its name,
// its arguments, and the offset just past its closing parenthesis.
func scanMacro(q string, i int) (name string, args []string, end int, ok bool) {
	j := i + len(_macroPrefix)

	for j < len(q) && isWordByte(q[j]) {
		j++
	}

	if j == i+len(_macroPrefix) || j >= len(q) || q[j] != '(' {
		return "", nil, 0, false
	}

	name = q[i+len(_macroPrefix) : j]

	depth := 0

	for k := j; k < len(q); k++ {
		switch q[k] {
		case '(':
			depth++
		case ')':
			depth--

			if depth == 0 {
				return name, parseMacroArgs(q[j+1 : k]), k + 1, true
			}
		}
	}

	return "", nil, 0, false
}

// isWordByte reports whether the byte can appear in a macro name.
func isWordByte(c byte) bool {
	return c == '_' ||
		(c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z')
}

// parseMacroArgs splits a macro's argument list on the commas that separate
// arguments, leaving the ones nested inside a call alone.
func parseMacroArgs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	var (
		args  []string
		depth int
		start int
	)

	for i := range len(raw) {
		switch raw[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, trimMacroArg(raw[start:i]))
				start = i + 1
			}
		}
	}

	return append(args, trimMacroArg(raw[start:]))
}

// trimMacroArg strips the whitespace and quoting around one macro argument.
func trimMacroArg(arg string) string {
	return strings.Trim(strings.TrimSpace(arg), "'\"")
}

// processSQLQuery resolves the generic macros first, so they can be used as
// macro arguments, and then the SQL ones, taking every expansion that depends
// on the dialect from mm.
func (tr TimeRange) processSQLQuery(q string, mm sqlMacros) string {
	q = tr.ProcessQuery(q)

	return expandMacros(q, func(name string, args []string) (string, bool) {
		// the macro helpers fall back to the invocation itself when the
		// arguments do not fit, so the raw form is rebuilt for them.
		match := _macroPrefix + name + "(" + strings.Join(args, ",") + ")"

		switch name {
		case _timeColumn, "timeEpoch":
			return mm.time(args, match), true
		case "timeFilter":
			return mm.timeFilter(tr, args, match), true
		case "timeFrom":
			return mm.timeFrom(tr), true
		case "timeTo":
			return mm.timeTo(tr), true
		case "timeGroup":
			return mm.timeGroup(args, match), true
		case "timeGroupAlias":
			return mm.timeGroupAlias(args, match), true
		case "unixEpochGroupAlias":
			return mm.unixEpochGroupAlias(args, match), true
		case "unixEpochFilter":
			return pgMacroUnixEpochFilter(tr, args, match), true
		case "unixEpochFrom":
			return pgMacroUnixEpochFrom(tr), true
		case "unixEpochTo":
			return pgMacroUnixEpochTo(tr), true
		case "unixEpochNanoFilter":
			return pgMacroUnixEpochNanoFilter(tr, args, match), true
		case "unixEpochNanoFrom":
			return pgMacroUnixEpochNanoFrom(tr), true
		case "unixEpochNanoTo":
			return pgMacroUnixEpochNanoTo(tr), true
		case "unixEpochGroup":
			return pgMacroUnixEpochGroup(args, match), true
		default:
			return "", false
		}
	})
}

// sqlMacros carries the macro expansions that differ between the SQL
// dialects. The unix-epoch family is absent because it renders the same
// either way, and lives in the dispatcher itself.
type sqlMacros struct {
	// time expands $__time and $__timeEpoch.
	time func(args []string, original string) string

	// timeFilter expands $__timeFilter.
	timeFilter func(tr TimeRange, args []string, original string) string

	// timeFrom expands $__timeFrom.
	timeFrom func(tr TimeRange) string

	// timeTo expands $__timeTo.
	timeTo func(tr TimeRange) string

	// timeGroup expands $__timeGroup.
	timeGroup func(args []string, original string) string

	// timeGroupAlias expands $__timeGroupAlias.
	timeGroupAlias func(args []string, original string) string

	// unixEpochGroupAlias expands $__unixEpochGroupAlias, which differs only
	// in how the dialect quotes the time alias.
	unixEpochGroupAlias func(args []string, original string) string
}
