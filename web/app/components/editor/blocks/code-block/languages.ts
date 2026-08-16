import { common } from "lowlight"
import type { LanguageFn } from "highlight.js"
import curl from "highlightjs-curl"

const languageNames: Record<string, string> = {
	arduino: "Arduino",
	bash: "Bash",
	curl: "cURL",
	c: "C",
	cpp: "C++",
	csharp: "C#",
	css: "CSS",
	diff: "Diff",
	go: "Go",
	graphql: "GraphQL",
	ini: "INI",
	java: "Java",
	javascript: "JavaScript",
	json: "JSON",
	kotlin: "Kotlin",
	less: "Less",
	lua: "Lua",
	makefile: "Makefile",
	markdown: "Markdown",
	objectivec: "Objective-C",
	perl: "Perl",
	php: "PHP",
	"php-template": "PHP Template",
	plaintext: "Plaintext",
	python: "Python",
	"python-repl": "Python REPL",
	r: "R",
	ruby: "Ruby",
	rust: "Rust",
	scss: "SCSS",
	shell: "Shell",
	sql: "SQL",
	swift: "Swift",
	typescript: "TypeScript",
	vbnet: "VB.Net",
	wasm: "WebAssembly",
	xml: "XML / HTML",
	yaml: "YAML",
}
export const extendedCodeBlockLanguages = common
extendedCodeBlockLanguages.curl = curl as LanguageFn

export const extendedCodeBlockLanguageOptions = Object.fromEntries(
	Object.keys(extendedCodeBlockLanguages)
		.sort((a, b) => (a < b ? -1 : 1))
		.map((key) => {
			return [key, languageNames[key]]
		}),
)
export const defaultExtendedCodeBlockLanguage = "plaintext"
