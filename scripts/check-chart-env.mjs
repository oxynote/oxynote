// compare the chart's env names with the variables the launcher declares.
// extraEnv accepts every declared name except OXYNOTE_PUBLIC_URL, which the
// chart sets from publicUrl. env accepts exactly the ones that do not look
// like a secret. Print what differs and exit 1 when
// they drift apart.
//
// usage: node scripts/check-chart-env.mjs
import { readFileSync } from "node:fs"

const envPath = "docker/prod/launcher/src/env.ts"
const schemaPath = "charts/oxynote/values.schema.json"

// the launcher's names are the keys of baseSchema, one per line.
const source = readFileSync(new URL(`../${envPath}`, import.meta.url), "utf8")
const start = source.indexOf("const baseSchema = z.object({")
const end = source.indexOf("\n})", start)

if (start === -1 || end === -1) {
	console.error(`${envPath}: baseSchema not found.`)
	process.exit(1)
}

const declared = [
	...source.slice(start, end).matchAll(/^\t(OXYNOTE_[A-Z0-9_]+):/gm),
]
	.map((m) => m[1])
	.filter((name) => name !== "OXYNOTE_PUBLIC_URL")

const schema = JSON.parse(
	readFileSync(new URL(`../${schemaPath}`, import.meta.url), "utf8"),
)
const all = schema.properties.extraEnv.items.properties.name.enum
const plain = Object.keys(schema.properties.env.properties)
const looksSecret = /_(DSN|SECRET|KEY|KEYS|TOKEN|JSON)$/
const problems = []

for (const name of declared) {
	if (!all.includes(name)) {
		problems.push(`${name} is declared in ${envPath} but missing from extraEnv`)
	}

	if (!looksSecret.test(name) && !plain.includes(name)) {
		problems.push(`${name} is not a secret, so it belongs in env too`)
	}
}

for (const name of all) {
	if (!declared.includes(name)) {
		problems.push(`${name} is in extraEnv but not declared in ${envPath}`)
	}
}

for (const name of plain) {
	if (!all.includes(name)) {
		problems.push(`${name} is in env but not in extraEnv`)
	}

	if (looksSecret.test(name)) {
		problems.push(`${name} looks like a secret, so it does not belong in env`)
	}
}

if (problems.length === 0) {
	process.exit(0)
}

console.error(`${schemaPath} and ${envPath} differ.`)

for (const problem of problems) {
	console.error(`  ${problem}`)
}

process.exit(1)
