// a filter a reader added from a line's fields. It narrows only the view
// it was added in and is never saved with the block.
export interface LogFilter {
	key: string
	value: string
	// a stream label, which the source answers from its index
	indexed: boolean
	exclude: boolean
}

// RE2 metacharacters, which a search has to match literally
const REGEX_SPECIAL = /[\\.+*?()|[\]{}^$]/g

// withViewFilters returns the query narrowed by the view's filters and
// search. An indexed filter joins the stream selector. Any other filter,
// and an indexed one when the query has no selector to join, follows the
// query as a label filter. The search follows as a case-insensitive line
// filter.
export function withViewFilters(
	query: string,
	filters: LogFilter[],
	search: string,
): string {
	const selector = streamSelector(query)
	const joined = selector ? filters.filter((f) => f.indexed) : []
	let result = query

	if (selector && joined.length) {
		const inner = query.slice(selector.from + 1, selector.to).trim()
		const matchers = [inner, ...joined.map(matcher)].filter(Boolean)

		result = `${query.slice(0, selector.from)}{${matchers.join(", ")}}${query.slice(selector.to + 1)}`
	}

	for (const filter of filters) {
		if (!joined.includes(filter)) {
			result += ` | ${matcher(filter)}`
		}
	}

	const term = search.trim()
	if (term) {
		result += ` |~ ${quote(`(?i)${term.replace(REGEX_SPECIAL, "\\$&")}`)}`
	}

	return result
}

function matcher(filter: LogFilter): string {
	return `${filter.key}${filter.exclude ? "!=" : "="}${quote(filter.value)}`
}

// quote writes a double-quoted LogQL string. Loki reads it the way Go's
// strconv.Unquote does, which refuses a raw newline or other control
// character. JSON escapes those, and every JSON escape is one Go accepts.
function quote(value: string): string {
	return JSON.stringify(value)
}

// streamSelector finds the braces of the query's stream selector, the
// first brace outside a string. Braces inside quoted values do not count.
function streamSelector(query: string): { from: number; to: number } | null {
	let from = -1
	let quoteChar: string | null = null

	for (let i = 0; i < query.length; i++) {
		const char = query.charAt(i)

		if (quoteChar) {
			if (char === "\\" && quoteChar === '"') {
				i++
			} else if (char === quoteChar) {
				quoteChar = null
			}

			continue
		}

		if (char === '"' || char === "`") {
			quoteChar = char
		} else if (char === "{" && from < 0) {
			from = i
		} else if (char === "}" && from >= 0) {
			return { from, to: i }
		}
	}

	return null
}
