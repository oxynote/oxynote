import { parser } from "@oxynote/lezer-logql"
import { LanguageSupport, LRLanguage } from "@codemirror/language"
import { styleTags, tags } from "@lezer/highlight"
import {
	snippetCompletion,
	type Completion,
	type CompletionContext,
	type CompletionResult,
} from "@codemirror/autocomplete"
import type { EditorState } from "@codemirror/state"
import { resolveTimeRange, TimeRangePreset } from "~/utils/time-range"
import type { VariableTreeResponse } from "~/utils"
import { referencedVariableValues } from "~/utils/variable"
import {
	TYPED_VARIABLE_PATTERN,
	variableCompletions,
} from "./variable-completion"

const logqlLanguage = LRLanguage.define({
	name: "logql",
	parser: parser.configure({
		props: [
			styleTags({
				LineComment: tags.comment,
				String: tags.string,
				"Number Duration Bytes": tags.number,
				"Identifier LabelName": tags.labelName,
				"VariableReference DynamicDurationPlaceholder": tags.special(
					tags.variableName,
				),
				ParserFlag: tags.modifier,
				"Json Logfmt Unpack Pattern Regexp Unwrap LabelFormat LineFormat Decolorize Drop Keep":
					tags.keyword,
				"CountOverTime Rate RateCounter BytesOverTime BytesRate AvgOverTime SumOverTime MinOverTime MaxOverTime StdvarOverTime StddevOverTime QuantileOverTime FirstOverTime LastOverTime AbsentOverTime BytesConv DurationConv DurationSecondsConv Ip ApproxCountDistinct LabelReplace Vector":
					tags.function(tags.variableName),
				"Sum Avg Count Max Min Stddev Stdvar Bottomk Topk Sort Sort_Desc Approx_Topk":
					tags.operatorKeyword,
				"By Without Bool On Ignoring GroupLeft GroupRight Offset":
					tags.modifier,
				"And Or Unless": tags.logicOperator,
				"Eq Neq Re Nre Pipe Lte Lss Gte Gtr Eql PipeExact PipeMatch PipePattern Npa Add Mul Mod Div Sub Pow":
					tags.operator,
				"( )": tags.paren,
				"[ ]": tags.squareBracket,
				"{ }": tags.brace,
				"⚠": tags.invalid,
			}),
		],
	}),
	languageData: { commentTokens: { line: "#" } },
})

// the stages a pipe can start, other than a label filter
function stageCompletions(t: (key: string) => string): Completion[] {
	// NOTE: i18n here is static
	return [
		{
			label: "json",
			type: "keyword",
			info: t("editor.config-modal-utils.query-info.loki.json"),
		},
		{
			label: "logfmt",
			type: "keyword",
			info: t("editor.config-modal-utils.query-info.loki.logfmt"),
		},
		snippetCompletion('regexp "${}"', {
			label: "regexp",
			type: "keyword",
			info: t("editor.config-modal-utils.query-info.loki.regexp"),
		}),
		snippetCompletion('pattern "${}"', {
			label: "pattern",
			type: "keyword",
			info: t("editor.config-modal-utils.query-info.loki.pattern"),
		}),
		{
			label: "unpack",
			type: "keyword",
			info: t("editor.config-modal-utils.query-info.loki.unpack"),
		},
		snippetCompletion('line_format "${}"', {
			label: "line_format",
			type: "keyword",
			info: t("editor.config-modal-utils.query-info.loki.line-format"),
		}),
		{
			label: "label_format",
			type: "keyword",
			info: t("editor.config-modal-utils.query-info.loki.label-format"),
		},
		{
			label: "drop",
			type: "keyword",
			info: t("editor.config-modal-utils.query-info.loki.drop"),
		},
		{
			label: "keep",
			type: "keyword",
			info: t("editor.config-modal-utils.query-info.loki.keep"),
		},
		{
			label: "decolorize",
			type: "keyword",
			info: t("editor.config-modal-utils.query-info.loki.decolorize"),
		},
	]
}

function lineFilterCompletions(t: (key: string) => string): Completion[] {
	// NOTE: i18n here is static
	return [
		snippetCompletion('|= "${}"', {
			label: "|=",
			type: "keyword",
			info: t("editor.config-modal-utils.query-info.loki.contains"),
		}),
		snippetCompletion('!= "${}"', {
			label: "!=",
			type: "keyword",
			info: t("editor.config-modal-utils.query-info.loki.not-contains"),
		}),
		snippetCompletion('|~ "${}"', {
			label: "|~",
			type: "keyword",
			info: t("editor.config-modal-utils.query-info.loki.matches"),
		}),
		snippetCompletion('!~ "${}"', {
			label: "!~",
			type: "keyword",
			info: t("editor.config-modal-utils.query-info.loki.not-matches"),
		}),
	]
}

// a stage that starts with one of these is not a label filter
const STAGE_KEYWORD =
	/^\s*(json|logfmt|regexp|pattern|unpack|line_format|label_format|decolorize|unwrap)\b/

const WORD = /^\s*(\$?\w*)$/

// valueMatcher reads the name and operator of a matcher that text ends
// with. Only these operators compare with a string.
function valueMatcher(text: string) {
	const match = /([A-Za-z_]\w*)\s*(=~|!~|!=|=)\s*$/.exec(text)

	if (!match?.[1] || !match[2]) {
		return null
	}

	return { field: match[1], regex: match[2].endsWith("~") }
}

// what the query holds before the cursor, read character by character.
// The parse tree cannot say this alone. An unclosed string has no node,
// but a value is typed in an unclosed string. string and selector are
// where the string and the selector around the cursor open. stagePipe
// starts the cursor's stage, and it is null once a line filter follows.
interface Scan {
	string: { from: number; quote: string } | null
	selector: number | null
	selectorEnd: number | null
	stagePipe: number | null
}

function scanQuery(text: string): Scan {
	const scan: Scan = {
		string: null,
		selector: null,
		selectorEnd: null,
		stagePipe: null,
	}

	for (let i = 0; i < text.length; i++) {
		const c = text.charAt(i)

		if (scan.string) {
			if (c === "\\" && scan.string.quote === '"') {
				i++
			} else if (c === scan.string.quote) {
				scan.string = null
			}

			continue
		}

		switch (c) {
			case '"':
			case "`":
				scan.string = { from: i, quote: c }

				break
			case "#":
				i = text.indexOf("\n", i)

				if (i < 0) {
					i = text.length
				}

				break
			case "{":
				scan.selector = i

				break
			case "}":
				if (scan.selector !== null) {
					scan.selector = null
					scan.selectorEnd ??= i + 1
				}

				break
			case "|":
				if (scan.selector === null) {
					scan.stagePipe = "=~>".includes(text.charAt(i + 1)) ? null : i
				}

				break
		}
	}

	return scan
}

// the matchers of the selector around pos, other than the one pos is in.
// They narrow the values offered for the matcher being written.
function otherMatchers(doc: string, open: number, pos: number) {
	const close = doc.indexOf("}", pos)
	const selector = doc.slice(open + 1, close < 0 ? doc.length : close)
	const at = pos - open - 1
	const matchers = selector.matchAll(
		/(?:[A-Za-z_]\w*|\$[a-z][a-z0-9_]*)\s*(?:=~|!~|!=|=)\s*(?:"(?:[^"\\]|\\.)*"|`[^`]*`)/g,
	)
	const kept = [...matchers]
		.filter((m) => at < m.index || at > m.index + m[0].length)
		.map((m) => m[0])

	return kept.length ? `{${kept.join(", ")}}` : undefined
}

// quoteValue writes a value as the inside of a string with the given
// quote. A regex matcher matches the value literally, so its special
// characters are escaped first.
function quoteValue(value: string, quote: string, regex: boolean) {
	const literal = regex ? value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&") : value

	if (quote === "`") {
		return literal
	}

	return literal.replace(/\\/g, "\\\\").replace(/"/g, '\\"')
}

// acceptsVariable tells whether the grammar takes a variable in place of
// the text between from and to. It parses the query with a stand-in
// variable there. A slot that takes a value, a label name or a number
// parses it. A keyword or an operator position turns it into an error.
function acceptsVariable(state: EditorState, from: number, to: number) {
	const probe = `${state.sliceDoc(0, from)}$x${state.sliceDoc(to)}`
	const node = parser.parse(probe).resolveInner(from + 1, 1)

	return node.name === "VariableReference" && !node.parent?.type.isError
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
	if (!inString && !acceptsVariable(ctx.state, from, ctx.pos)) {
		return null
	}

	return { from: from, typed: !!typed }
}

export function useLogQLQueryExtension(
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

	// without a query, the fields are the stream labels. With one, they
	// also hold the fields its parsers find in the lines.
	async function fieldNames(q?: string): Promise<Completion[]> {
		fieldsParams.value = lookupParams(q)
		const res = await fetchFields.refresh()

		return (res.data?.fields ?? []).map((field) =>
			field.kind === LogFieldKind.Label
				? {
						label: field.name,
						type: "constant",
						info: t("editor.config-modal-utils.query-info.loki.label"),
					}
				: {
						label: field.name,
						type: "property",
						info: t("editor.config-modal-utils.query-info.loki.field"),
					},
		)
	}

	async function fieldValues(
		field: string,
		q: string | undefined,
		apply: (value: string) => string | null,
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

	// contextCompletions offers what fits at the cursor, apart from the
	// variables: label and field names, their values, and the stages.
	async function contextCompletions(
		ctx: CompletionContext,
		scan: Scan,
	): Promise<CompletionResult | null> {
		const doc = ctx.state.doc.toString()
		const before = doc.slice(0, ctx.pos)

		if (scan.string) {
			const { from, quote } = scan.string
			const matcher = valueMatcher(doc.slice(0, from))

			if (!matcher) {
				return null
			}

			const apply = (value: string) =>
				quote === "`" && value.includes("`")
					? null
					: quoteValue(value, quote, matcher.regex)

			if (scan.selector !== null) {
				return {
					from: from + 1,
					options: await fieldValues(
						matcher.field,
						otherMatchers(doc, scan.selector, ctx.pos),
						apply,
					),
				}
			}

			if (scan.stagePipe === null) {
				return null
			}

			return {
				from: from + 1,
				options: await fieldValues(
					matcher.field,
					doc.slice(0, scan.stagePipe).trim(),
					apply,
				),
			}
		}

		if (scan.selector !== null) {
			const segment = before.slice(
				Math.max(scan.selector, before.lastIndexOf(",")) + 1,
			)
			const word = WORD.exec(segment)

			if (word) {
				return {
					from: ctx.pos - (word[1]?.length ?? 0),
					options: await fieldNames(),
					validFor: /^\w*$/,
				}
			}

			const matcher = valueMatcher(segment)
			if (!matcher) {
				return null
			}

			return {
				from: ctx.pos,
				options: await fieldValues(
					matcher.field,
					otherMatchers(doc, scan.selector, ctx.pos),
					(value) => `"${quoteValue(value, '"', matcher.regex)}"`,
				),
			}
		}

		if (scan.selectorEnd === null) {
			return null
		}

		if (scan.stagePipe === null) {
			const afterSelector = before.slice(scan.selectorEnd)

			if (!ctx.explicit || !/^\s+$/.test(afterSelector)) {
				return null
			}

			return { from: ctx.pos, options: lineFilterCompletions(t) }
		}

		const q = doc.slice(0, scan.stagePipe).trim()
		const stage = before.slice(scan.stagePipe + 1)
		const word = WORD.exec(stage)

		if (word) {
			return {
				from: ctx.pos - (word[1]?.length ?? 0),
				options: [...stageCompletions(t), ...(await fieldNames(q))],
				validFor: /^\w*$/,
			}
		}

		if (STAGE_KEYWORD.test(stage)) {
			return null
		}

		// drop and keep take a list of names. A label filter joins its
		// filters with a comma, and, or, or a bracket.
		const list = /^\s*(?:drop|keep)\s+/.exec(stage)
		const rest = list ? stage.slice(list[0].length) : stage
		const segment = rest.split(/,|\(|\band\b|\bor\b/).pop() ?? ""
		const name = list || segment !== rest ? WORD.exec(segment) : null

		if (name) {
			return {
				from: ctx.pos - (name[1]?.length ?? 0),
				options: await fieldNames(q),
				validFor: /^\w*$/,
			}
		}

		const matcher = list ? null : valueMatcher(segment)
		if (!matcher) {
			return null
		}

		return {
			from: ctx.pos,
			options: await fieldValues(
				matcher.field,
				q,
				(value) => `"${quoteValue(value, '"', matcher.regex)}"`,
			),
		}
	}

	async function complete(
		ctx: CompletionContext,
	): Promise<CompletionResult | null> {
		const scan = scanQuery(ctx.state.sliceDoc(0, ctx.pos))
		const slot = variableSlot(ctx, !!scan.string)
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

		const res = await contextCompletions(ctx, scan)

		if (!res) {
			return slot && ctx.explicit
				? { from: slot.from, options: variableOptions }
				: null
		}

		return { ...res, options: [...res.options, ...variableOptions] }
	}

	return {
		extensions: computed(() => [
			new LanguageSupport(logqlLanguage, [
				logqlLanguage.data.of({ autocomplete: complete }),
			]),
		]),
	}
}
