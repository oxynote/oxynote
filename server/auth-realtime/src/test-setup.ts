// yjs answers a read from a type that was never added to a document with a
// console warning rather than an error, so a fixture that forgets to attach
// its type still passes and the warning scrolls past — and vitest only
// prints intercepted stderr under the verbose reporter, so it is invisible
// in a piped or CI run. Turn it into a failure attributed to the test that
// caused it.
const PREMATURE_ACCESS = "Invalid access: Add Yjs type to a document"

const warn: typeof console.warn = console.warn.bind(console)

console.warn = (...args: unknown[]) => {
	const [first] = args

	if (typeof first === "string" && first.includes(PREMATURE_ACCESS)) {
		throw new Error(
			`${first} — the yjs type under test was never added to a document, so it cannot be read from. Attach it first.`,
		)
	}

	warn(...args)
}
