import { lowlight } from "./index"
import { defaultExtendedCodeBlockLanguage } from "./languages"

// one matching pattern counts as much as this many keyword matches
const PATTERN_WEIGHT = 5

// the best language needs this score and this lead over the next one. A
// block that reaches neither stays plaintext.
const MIN_SCORE = 5
const MIN_LEAD = 3

// constructs only one of the languages writes. The highlighter scores a
// language by its keywords alone, and many languages share those.
const LANGUAGE_PATTERNS: Record<string, RegExp[]> = {
	bash: [
		/^#!.*\b(ba|z)?sh\b/,
		/^\s*(sudo |cd |export \w+=|echo |source )/m,
		/^\s*(git|docker|kubectl|helm|npm|pnpm|yarn|npx|make|brew|apt(-get)?|pip3?|cargo|go|terraform|aws|gcloud|ssh|scp|wget|tar|mkdir|chmod|cat|grep|ls|rm|cp|mv) [\w.\-/]/m,
		/\b(then|fi|done|esac)\s*$/m,
		/\$\{?\w+\}?"|\$\(/,
	],
	c: [
		/^#include <\w+\.h>/m,
		/\b(printf|malloc|sizeof)\(/,
		/^(int|void|char) \w+\([^)]*\)\s*\{/m,
	],
	cpp: [
		/^#include <[a-z_]+>/m,
		/\bstd::/,
		/^(template|namespace|class) .*[<{]/m,
		/\b(cout|cin|endl)\b/,
	],
	csharp: [
		/^using [\w.]+;/m,
		/^namespace [\w.]+\s*[{;]?$/m,
		/\{ get; (set|init); \}/,
		/\b(Console\.Write|string\[\] args|async Task|foreach \(|IEnumerable<)/,
		/\.(Where|Select|OrderBy|ToList)\(/,
		/\b(Task<|IActionResult|public record |private readonly )/,
		/^\s*\[(Http\w+|ApiController|Route|Fact|Test)\b/m,
	],
	css: [
		/^[.#][\w-]+[^\n{};=(]*\{\s*$/m,
		/^(html|body|a|p|div|span|h[1-6]|ul|li|table|input|button|img)\b[^\n{};=(]*\{\s*$/m,
		/^\s*[a-z-]+:\s*[^;{}\n]+;\s*$/m,
		/(^|\s)(@media|@import|:root|!important)\b/m,
	],
	curl: [/^\s*curl\s/],
	diff: [
		/^diff --git /m,
		/^@@ -\d+(,\d+)? \+\d+(,\d+)? @@/m,
		/^--- .*\n\+\+\+ /m,
	],
	go: [
		/^package \w+\s*$/m,
		/^func (\(\w+ \*?[\w.]+\) )?\w+(\[[^\]]+\])?\(/m,
		/\w+(, \w+)* := /,
		/^type \w+ (struct|interface) \{/m,
		/\berr [!=]= nil\b|\bfunc\(|\bdefer |\bchan |\brange /,
		/^import \(\s*$|^import "[\w./-]+"$/m,
	],
	graphql: [
		/^\s*(query|mutation|subscription|fragment)\b[^=;\n]*\{\s*$/m,
		/^\s*\w+(\([^)]*\))?:\s*\[?\w+!?\]?!?\s*$/m,
		/\$\w+:\s*\[?\w+!?\]?/,
	],
	ini: [/^\[[\w.\- ]+\]\s*$/m, /^[\w.-]+ = \S/m],
	java: [
		/^import (static )?(java|javax|org|com)\.[\w.*]+;/m,
		/\bSystem\.(out|err)\.|String\[\] args/,
		/^\s*@(Override|Autowired|RestController|GetMapping|PostMapping|Test|Entity)\b/m,
		/\b(public|private|protected) (static )?(final )?[\w<>[\]]+ \w+\([^)]*\)\s*(throws \w+\s*)?\{/,
	],
	javascript: [
		/\b(const|let) \w+ = /,
		/=>\s*[{(\w]/,
		/\b(require\(|console\.\w+\(|document\.|module\.exports|window\.)/,
		/\b(async )?function\*? \w*\(/,
		/^(import|export) .* from ["']/m,
	],
	json: [
		/^\s*\{\s*("[^"\n]*"\s*:|\}\s*$)/,
		/^\s*\[\s*([{["\d\]-]|true|false|null)/,
		/^\s*"[^"\n]*"\s*:\s*("|\d|true|false|null|\{|\[)/m,
	],
	kotlin: [
		/\bfun \w+\(/,
		/\b(val|var) \w+(: \w+)? = /,
		/\b(println\(|listOf\(|data class |companion object)/,
	],
	makefile: [/^\.PHONY:/m, /^[\w.\-/%]+:[^=\n]*\n\t\S/m, /\$\((\w+)\)|\$@|\$</],
	markdown: [
		/^#{1,6} \S.*\n\s*\n/m,
		/\[[^\]\n]+\]\([^)\n]+\)/,
		/(\*\*|__)[^*_\n]+(\*\*|__)/,
		/^```/m,
	],
	php: [/^\s*<\?php/, /\$\w+->\w+|\$this\b/, /\bfunction \w+\([^)]*\$\w+/],
	python: [
		/^\s*(def|class) \w+[^\n{;]*:\s*(#.*)?$/m,
		/^from [\w.]+ import [\w*, ]+$/m,
		/^import \w+(\.\w+)*( as \w+)?$/m,
		/\b(self\.\w+|__\w+__|print\(|elif |None\b|True\b|False\b)/,
		/^\s*(if|for|while|with|try|except)\b[^\n{;]*:\s*$/m,
	],
	ruby: [
		/^\s*def \w+[^\n:{]*$/m,
		/^\s*end\s*$/m,
		/\b(attr_(reader|writer|accessor)|puts |require ')/,
		/\bdo \|[\w, ]+\|/,
		/@\w+ = /,
	],
	rust: [
		/^use \w+(::[\w{}*, ]+)+;/m,
		/\bfn \w+(<[^>]+>)?\(/,
		/\blet (mut )?\w+/,
		/^\s*#\[\w+/m,
		/\b(impl|pub (fn|struct|enum)|println!|Some\(|Ok\(|&str\b)/,
	],
	shell: [/^\$ \S/m],
	sql: [
		/^\s*(select)\b[\s\S]*\bfrom\b/im,
		/^\s*(insert into|delete from|create (table|index|view)|alter table|drop table)\b/im,
		/^\s*update \w+ set\b/im,
		/\b(where|group by|order by|left join|inner join|primary key|not null)\b/i,
	],
	swift: [
		/^import (Foundation|UIKit|SwiftUI)\b/m,
		/\bfunc \w+\([^)]*\) -> /,
		/\b(guard let|if let|let \w+: \w+)/,
		/\\\(\w+/,
	],
	typescript: [
		/\b(interface|type) \w+(<[^>]+>)? (=|\{|extends)/,
		/[\w)\]]\??: (string|number|boolean|unknown|void|any|Promise<|Record<|\w+\[\])/,
		/\b(as (const|unknown|string|number|\w+)\b|import type |readonly |enum \w+ \{)/,
		/\bexport (default |async )?(function|const|class|interface|type)\b/,
	],
	xml: [
		/^\s*<(!DOCTYPE|\?xml|[a-zA-Z][\w:-]*)(\s[^<>]*)?\/?>/,
		/<\/[a-zA-Z][\w:-]*>\s*$/m,
		/<[a-zA-Z][\w:-]*(\s+[\w:-]+="[^"]*")+\s*\/?>/,
	],
	yaml: [/^[\w.-]+:\s*$/m, /^\s+- [\w"'{[]/m, /^\s+[\w.-]+: \S/m, /^---\s*$/m],
}

export function detectLanguage(code: string): string {
	if (!code.trim()) {
		return defaultExtendedCodeBlockLanguage
	}

	let bestLanguage = defaultExtendedCodeBlockLanguage
	let bestScore = 0
	let nextScore = 0

	for (const [language, patterns] of Object.entries(LANGUAGE_PATTERNS)) {
		const score = languageScore(language, patterns, code)

		if (score > bestScore) {
			nextScore = bestScore
			bestScore = score
			bestLanguage = language
		} else if (score > nextScore) {
			nextScore = score
		}
	}

	if (bestScore < MIN_SCORE || bestScore - nextScore < MIN_LEAD) {
		return defaultExtendedCodeBlockLanguage
	}

	return bestLanguage
}

function languageScore(
	language: string,
	patterns: RegExp[],
	code: string,
): number {
	const relevance = lowlight.highlight(language, code).data?.relevance ?? 0
	const matches = patterns.filter((pattern) => pattern.test(code)).length

	return relevance + matches * PATTERN_WEIGHT
}
