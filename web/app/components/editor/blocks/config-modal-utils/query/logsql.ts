import {
	filterFunctions,
	mathFunctions,
	parser,
	pipeNames,
	runningStatsFunctions,
	statsFunctions,
} from "@oxynote/lezer-logsql"
import { LanguageSupport, LRLanguage } from "@codemirror/language"
import {
	insertCompletionText,
	pickedCompletion,
	snippetCompletion,
	type Completion,
	type CompletionContext,
	type CompletionResult,
} from "@codemirror/autocomplete"
import type { SyntaxNode } from "@lezer/common"
import { resolveTimeRange, TimeRangePreset } from "~/utils/time-range"
import type { VariableTreeResponse } from "~/utils"
import { referencedVariableValues } from "~/utils/variable"
import { unquoteLogsQL } from "~/components/editor/blocks/logs/victorialogs"
import {
	TYPED_VARIABLE_PATTERN,
	variableCompletions,
} from "./variable-completion"

const logsqlLanguage = LRLanguage.define({
	name: "logsql",
	parser,
	languageData: { commentTokens: { line: "#" } },
})

const INFO_KEY = "query-languages.victorialogs"

// other names of a pipe. They share its description.
const PIPE_ALIASES: Record<string, string> = {
	cp: "copy",
	del: "delete",
	drop: "delete",
	eval: "math",
	head: "limit",
	keep: "fields",
	mv: "rename",
	order: "sort",
	rm: "delete",
	skip: "offset",
	where: "filter",
}

// the pipes that start with quoted text
const PIPE_SNIPPETS: Record<string, string> = {
	extract: 'extract "${}"',
	extract_regexp: 'extract_regexp "${}"',
	format: 'format "${}"',
	replace: 'replace ("${1}", "${2}")',
	replace_regexp: 'replace_regexp ("${1}", "${2}")',
}

// the fields every log has. The server leaves them out of its field list.
const BUILTIN_FIELDS = ["_msg", "_stream", "_stream_id", "_time"]

// the filter functions whose arguments are values of the field
const VALUE_FUNCTIONS = [
	"contains_all",
	"contains_any",
	"contains_common_case",
	"equals_common_case",
	"exact",
	"i",
	"in",
]

const WORD_BEFORE_CURSOR = /[\p{L}\p{Nd}_][\p{L}\p{Nd}_.-]*$/u
const WORD_CHARS = /^[\p{L}\p{Nd}_.-]*$/u
const WORD_CHAR = /[\p{L}\p{Nd}_]/u

const BARE_FIELD = /^[\p{L}_][\p{L}\p{Nd}_.-]*$/u

// math reads a - as minus, so a name with one needs quotes there
const BARE_MATH_FIELD = /^[\p{L}_][\p{L}\p{Nd}_.]*$/u

// the words VictoriaLogs reads as syntax, so a field with such a name needs
// quotes
const RESERVED_WORDS = new Set([
	...filterFunctions,
	...mathFunctions,
	...pipeNames,
	...runningStatsFunctions,
	...statsFunctions,
	"and",
	"as",
	"by",
	"from",
	"if",
	"not",
	"options",
	"or",
])

// what the query holds before the cursor, read character by character.
// The parse tree cannot say this alone, because an unclosed string has no
// node, but a value is typed in an unclosed string.
function scanQuery(text: string) {
	let string: { from: number; quote: string } | null = null

	for (let i = 0; i < text.length; i++) {
		const c = text.charAt(i)

		if (string) {
			if (c === "\\" && string.quote !== "`") {
				i++
			} else if (c === string.quote) {
				string = null
			}

			continue
		}

		if (c === "#") {
			i = text.indexOf("\n", i)

			if (i < 0) {
				return { string: null, inComment: true }
			}
		} else if (c === '"' || c === "'" || c === "`") {
			string = { from: i, quote: c }
		}
	}

	return { string, inComment: false }
}

// stringEnd finds where the string around pos closes. It gives pos when
// the string is still open.
function stringEnd(doc: string, pos: number, quote: string) {
	for (let i = pos; i < doc.length; i++) {
		if (doc.charAt(i) === "\\" && quote !== "`") {
			i++
		} else if (doc.charAt(i) === quote) {
			return i + 1
		}
	}

	return pos
}

// stringChar reads the character of a string's text at i. Outside
// backticks an escape is one character, and the string takes two
// backslashes to write one for the regex. Any other escape gives an empty
// char, because it never means anything to the regex.
function stringChar(doc: string, i: number, quote: string) {
	if (quote !== "`" && doc.charAt(i) === "\\") {
		return { char: doc.charAt(i + 1) === "\\" ? "\\" : "", length: 2 }
	}

	return { char: doc.charAt(i), length: 1 }
}

// a regex lists its alternatives with |, so a value completes only the
// one around pos. alternativeRange finds where that one starts and ends,
// in the string text between from and to. A group's parenthesis ends an
// alternative too. Inside a character class such as [|)] each character is
// literal, so it gives null when pos is inside one.
function alternativeRange(
	doc: string,
	from: number,
	to: number,
	pos: number,
	quote: string,
) {
	let start = from
	let end = to
	let inClass = false
	let classStart = from
	let cursorInClass: boolean | null = null
	let i = from

	while (i < to && end === to) {
		if (cursorInClass === null && i >= pos) {
			cursorInClass = inClass
		}

		const { char, length } = stringChar(doc, i, quote)
		let size = length

		if (char === "\\") {
			size += stringChar(doc, i + length, quote).length
		} else if (inClass) {
			// a ] right after the opening [ or [^ is literal. A named class
			// such as [:alpha:] holds its own ].
			const named = doc.indexOf(":]", i + 2)

			if (char === "]" && i > classStart) {
				inClass = false
			} else if (char === "[" && doc.charAt(i + 1) === ":" && named >= 0) {
				size = named + 2 - i
			}
		} else if (char === "[") {
			inClass = true
			classStart = i + (doc.charAt(i + 1) === "^" ? 2 : 1)
		} else if ("|()".includes(char)) {
			if (i < pos) {
				start = i + 1
			} else {
				end = i
			}
		}

		i += size
	}

	if (cursorInClass ?? inClass) {
		return null
	}

	return { start, end }
}

// probe parses the query with word in place of the text between from and
// to, and gives the node the word became. It gives null when the grammar
// refuses the word there. The parser recovers from such an error by
// inventing tokens, so the node alone does not show it. The tail is text
// that a word needs after it, such as the ( of a function.
function probe(doc: string, from: number, to: number, word: string, tail = "") {
	const text = doc.slice(0, from) + word + tail + doc.slice(to)
	const tree = parser.parse(text)
	const end = from + word.length
	const cursor = tree.cursor()

	do {
		if (cursor.type.isError && cursor.from >= from && cursor.from < end) {
			return null
		}
	} while (cursor.next())

	return { text, node: tree.resolveInner(from, 1) }
}

function closest(node: SyntaxNode, ...names: string[]) {
	for (let n: SyntaxNode | null = node; n; n = n.parent) {
		if (names.includes(n.name)) {
			return n
		}
	}

	return null
}

function fieldNameText(node: SyntaxNode, text: string) {
	return unquoteLogsQL(text.slice(node.from, node.to))
}

// the query whose logs a lookup at node should read. In a pipe, that is
// everything before the pipe. Before the first pipe, it is the stream
// filter, when every log has to match it.
function lookupScope(node: SyntaxNode, text: string) {
	const root = closest(node, "Subquery", "Query")
	if (!root) {
		return undefined
	}

	let pipe: number | null = null

	for (let c = root.firstChild; c && c.from < node.from; c = c.nextSibling) {
		if (c.name === "|") {
			pipe = c.from
		}
	}

	if (pipe !== null) {
		const start = root.name === "Subquery" ? root.from + 1 : root.from

		return text.slice(start, pipe).trim()
	}

	const stream = root.getChild("AndFilter")?.getChild("StreamFilter")

	return stream ? text.slice(stream.from, stream.to) : undefined
}

// the other complete matchers of a stream filter. They narrow the values
// offered for the matcher being written. With an or they may not apply.
function otherMatchers(matcher: SyntaxNode, text: string) {
	const filter = matcher.parent
	if (!filter || filter.getChild("LogicOperator")) {
		return undefined
	}

	const kept = filter
		.getChildren("StreamMatcher")
		.filter(
			(m) =>
				m.from !== matcher.from &&
				(m.getChild("Value") ?? m.getChild("ArgList")),
		)
		.map((m) => text.slice(m.from, m.to))

	return kept.length ? `{${kept.join(", ")}}` : undefined
}

// valueTarget finds the field whose values fit where the probed variable
// stands, and the query that narrows them. Null when no field value fits.
function valueTarget(node: SyntaxNode, text: string) {
	if (node.name !== "VariableReference") {
		return null
	}

	const holder = node.parent
	const owner = holder?.parent

	// a stream matcher holds its value directly, or in the list after in
	const matcher = owner?.name === "ArgList" ? owner.parent : owner

	if (holder?.name === "Value" && matcher?.name === "StreamMatcher") {
		const name = matcher.getChild("FieldName")

		return name
			? {
					field: fieldNameText(name, text),
					q: otherMatchers(matcher, text),
					regex: !!(matcher.getChild("=~") ?? matcher.getChild("!~")),
				}
			: null
	}

	const fits =
		holder?.name === "Phrase" ||
		(holder?.name === "Value" &&
			(owner?.name === "Exact" ||
				owner?.name === "Regexp" ||
				(owner?.name === "ArgList" &&
					VALUE_FUNCTIONS.includes(
						text
							.slice(owner.prevSibling?.from, owner.prevSibling?.to)
							.toLowerCase(),
					))))
	const filter = closest(node, "FieldFilter", "Subquery")
	const name = filter?.getChild("FieldName")

	if (!fits || filter?.name !== "FieldFilter" || !name) {
		return null
	}

	return {
		field: fieldNameText(name, text),
		q: lookupScope(filter, text),
		regex: owner?.name === "Regexp",
	}
}

// a name given to a result, such as "as total". It is new, so the names
// that exist are no help there.
function isResultName(node: SyntaxNode, text: string) {
	const before = node.prevSibling

	return (
		node.parent?.name === "StatsEntry" ||
		(before?.name === "Keyword" &&
			text.slice(before.from, before.to).toLowerCase() === "as")
	)
}

// quoteValue writes a value as the inside of a string with the given
// quote. A regex matches the value literally, so its special characters
// are escaped first.
function quoteValue(value: string, quote: string, regex: boolean) {
	const literal = regex ? value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&") : value
	if (quote === "`") {
		return literal
	}

	return literal
		.replace(/\\/g, "\\\\")
		.replaceAll(quote, `\\${quote}`)
		.replace(/\n/g, "\\n")
}

// replaceUpTo gives a completion that also replaces the text between the
// cursor and end. A plain completion ends at the cursor, so it leaves the
// rest of a value in place.
function replaceUpTo(text: string, end: number): Completion["apply"] {
	return (view, completion, from) => {
		view.dispatch({
			...insertCompletionText(view.state, text, from, end),
			annotations: pickedCompletion.of(completion),
		})
	}
}

function descriptionKey(group: string, name: string) {
	return `${INFO_KEY}.${group}.${name.replaceAll("_", "-")}`
}

function pipeCompletions(t: (key: string) => string): Completion[] {
	return pipeNames.map((name) => {
		const completion = {
			label: name,
			type: "keyword",
			info: t(descriptionKey("pipes", PIPE_ALIASES[name] ?? name)),
		}
		const snippet = PIPE_SNIPPETS[name]

		return snippet ? snippetCompletion(snippet, completion) : completion
	})
}

function functionCompletions(
	t: (key: string) => string,
	names: readonly string[],
	group: string,
): Completion[] {
	return names.map((name) =>
		snippetCompletion(`${name}(\${})`, {
			label: name,
			type: "function",
			info: t(descriptionKey(group, name)),
		}),
	)
}

function builtinFieldCompletions(
	t: (key: string) => string,
	suffix: string,
): Completion[] {
	return BUILTIN_FIELDS.map((name) => ({
		label: name,
		apply: `${name}${suffix}`,
		type: "constant",
		info: t(descriptionKey("fields", name.slice(1))),
	}))
}

// variableSlot is where a variable would go at the cursor: over the $key
// typed so far, or at the cursor before anything is typed. Null when the
// cursor is in the middle of another word or no variable fits there. Any
// text fits in a string, so a variable does too.
function variableSlot(ctx: CompletionContext, inString: boolean) {
	const typed = ctx.matchBefore(TYPED_VARIABLE_PATTERN)
	if (!typed && ctx.matchBefore(/\w+$/)) {
		return null
	}

	const from = typed?.from ?? ctx.pos
	const doc = ctx.state.doc.toString()
	const fits =
		inString ||
		probe(doc, from, ctx.pos, "$x")?.node.name === "VariableReference"

	return fits ? { from: from, typed: !!typed } : null
}

export function useLogsQLQueryExtension(
	t: (key: string) => string,
	dataSourceId: MaybeRefOrGetter<string | null | undefined>,
	timeRange: MaybeRefOrGetter<TimeRangePreset | null | undefined>,
	enable: MaybeRefOrGetter<boolean>,
	variables: MaybeRefOrGetter<VariableTreeResponse>,
) {
	const { useLogFields, useLogFieldValues } = useLogDataSourceAPI()

	const fieldsParams = ref<LogFieldsParams | null>(null)
	const fetchFields = useLogFields(dataSourceId, fieldsParams, enable)

	const fieldValuesParams = ref<LogFieldValuesParams | null>(null)
	const fetchFieldValues = useLogFieldValues(
		dataSourceId,
		fieldValuesParams,
		enable,
	)

	// a lookup reads the block's time range. It sends the values of the
	// variables its query references, for the server to substitute.
	function lookupParams(q: string | undefined) {
		const preset = toValue(timeRange) || TimeRangePreset.Last5Minutes
		const { from, to } = resolveTimeRange(preset)

		return {
			q: q,
			from: from,
			to: to,
			timeRangeKey: preset,
			variableValues: referencedVariableValues(
				toValue(variables),
				q ? [q] : [],
			),
		}
	}

	async function fieldNames(
		q: string | undefined,
		suffix: string,
		labelsOnly = false,
		bare = BARE_FIELD,
	): Promise<Completion[]> {
		fieldsParams.value = lookupParams(q)
		const res = await fetchFields.refresh()

		return (res.data?.fields ?? [])
			.filter((field) => !labelsOnly || field.kind === LogFieldKind.Label)
			.map((field) => {
				const name =
					bare.test(field.name) && !RESERVED_WORDS.has(field.name.toLowerCase())
						? field.name
						: JSON.stringify(field.name)
				const isLabel = field.kind === LogFieldKind.Label

				return {
					label: field.name,
					apply: `${name}${suffix}`,
					type: isLabel ? "constant" : "property",
					info: t(`${INFO_KEY}.${isLabel ? "label" : "field"}`),
				}
			})
	}

	async function fieldValues(
		field: string,
		q: string | undefined,
		apply: (value: string) => Completion["apply"] | null,
	): Promise<Completion[]> {
		fieldValuesParams.value = { ...lookupParams(q), field: field }
		const res = await fetchFieldValues.refresh()

		return (res.data?.values ?? []).flatMap(({ value }) => {
			const applied = apply(value)

			return applied === null
				? []
				: [{ label: value, apply: applied, type: "text" }]
		})
	}

	async function stringCompletions(
		ctx: CompletionContext,
		string: { from: number; quote: string },
	): Promise<CompletionResult | null> {
		const doc = ctx.state.doc.toString()
		const { from, quote } = string
		const end = stringEnd(doc, ctx.pos, quote)
		const probed = probe(doc, from, end, "$x")
		const target = probed && valueTarget(probed.node, probed.text)

		if (!target) {
			return null
		}

		// the string's text ends before its closing quote, or at the cursor
		// while the string is still open
		const textEnd = end > ctx.pos ? end - 1 : ctx.pos
		const range = target.regex
			? alternativeRange(doc, from + 1, textEnd, ctx.pos, quote)
			: { start: from + 1, end: textEnd }

		if (!range) {
			return null
		}

		return {
			from: range.start,
			options: await fieldValues(target.field, target.q, (value) => {
				if (quote === "`" && value.includes("`")) {
					return null
				}

				const text = quoteValue(value, quote, target.regex)

				return range.end > ctx.pos ? replaceUpTo(text, range.end) : text
			}),
		}
	}

	// wordCompletions offers what the grammar takes at the cursor. It asks
	// by probing one word of each kind, because every name of a kind is
	// valid in the same places.
	async function wordCompletions(
		ctx: CompletionContext,
	): Promise<CompletionResult | null> {
		const doc = ctx.state.doc.toString()
		if (WORD_CHAR.test(doc.charAt(ctx.pos))) {
			return null
		}

		const from = ctx.matchBefore(WORD_BEFORE_CURSOR)?.from ?? ctx.pos
		const at = (word: string, tail = "") =>
			probe(doc, from, ctx.pos, word, tail)
		const result = (options: Completion[]) => ({
			from: from,
			options: options,
			validFor: WORD_CHARS,
		})

		const name = at("x")
		if (
			name?.node.name === "FieldName" &&
			name.node.parent?.name === "StreamMatcher"
		) {
			return result(await fieldNames(undefined, "", true))
		}

		const variable = at("$x")
		const target = variable && valueTarget(variable.node, variable.text)
		const takesFilterFunction =
			at("seq", "(")?.node.parent?.name === "FunctionFilter"

		if (target) {
			const values = await fieldValues(
				target.field,
				target.q,
				(value) => `"${quoteValue(value, '"', target.regex)}"`,
			)

			return result(
				takesFilterFunction
					? [...values, ...functionCompletions(t, filterFunctions, "filters")]
					: values,
			)
		}

		const options: Completion[] = []
		const isPipe = at("limit")?.node.name === "PipeName"
		const filterField = at("x", ":")
		const takesFilterField =
			filterField?.node.name === "FieldName" &&
			filterField.node.parent?.name === "FieldFilter"

		// a bare word is a search, so a menu there only gets in the way
		if (!isPipe && takesFilterField && !ctx.explicit) {
			return null
		}

		if (isPipe) {
			options.push(...pipeCompletions(t))
		} else if (takesFilterFunction) {
			options.push(...functionCompletions(t, filterFunctions, "filters"))
		}

		const running = at("first", "(")?.node
		const statsPipe =
			running && closest(running, "RunningStatsPipe", "TotalStatsPipe")

		if (statsPipe) {
			options.push(
				...functionCompletions(
					t,
					runningStatsFunctions,
					statsPipe.name === "RunningStatsPipe"
						? "running-stats"
						: "total-stats",
				),
			)
		} else if (
			!isPipe &&
			at("median", "(")?.node.parent?.name === "StatsFunction"
		) {
			options.push(...functionCompletions(t, statsFunctions, "stats"))
		}

		if (at("abs", "(")?.node.parent?.name === "MathFunction") {
			options.push(...functionCompletions(t, mathFunctions, "math"))
		}

		if (takesFilterField) {
			options.push(
				...(await fieldNames(
					lookupScope(filterField.node, filterField.text),
					":",
				)),
				...builtinFieldCompletions(t, ":"),
			)
		} else if (
			name?.node.name === "FieldName" &&
			!isResultName(name.node, name.text)
		) {
			options.push(
				...(await fieldNames(
					lookupScope(name.node, name.text),
					"",
					false,
					closest(name.node, "MathPipe") ? BARE_MATH_FIELD : BARE_FIELD,
				)),
				...builtinFieldCompletions(t, ""),
			)
		}

		return options.length ? result(options) : null
	}

	async function complete(
		ctx: CompletionContext,
	): Promise<CompletionResult | null> {
		const { string, inComment } = scanQuery(ctx.state.sliceDoc(0, ctx.pos))
		if (inComment) {
			return null
		}

		const slot = variableSlot(ctx, !!string)
		const variableOptions = slot
			? variableCompletions(toValue(variables), t)
			: []

		// a typed $key only ever matches a variable
		if (slot?.typed) {
			return {
				from: slot.from,
				options: variableOptions,
				validFor: TYPED_VARIABLE_PATTERN,
			}
		}

		if (ctx.matchBefore(TYPED_VARIABLE_PATTERN)) {
			return null
		}

		const res = string
			? await stringCompletions(ctx, string)
			: await wordCompletions(ctx)

		if (!res) {
			return slot && ctx.explicit
				? { from: slot.from, options: variableOptions }
				: null
		}

		return { ...res, options: [...res.options, ...variableOptions] }
	}

	return {
		extensions: computed(() => [
			new LanguageSupport(logsqlLanguage, [
				logsqlLanguage.data.of({ autocomplete: complete }),
			]),
		]),
	}
}
