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

	for j < len(q) && (isWordByte(q[j])) {
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
