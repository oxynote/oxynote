// compare the chart's env names with the variables the launcher declares.
// Every declared name must be either in the schema's env list or in
// secretNames below, which only reach the pod through envFromSecret. A
// name that looks like a secret must be in secretNames. Print what differs
// and exit 1 when they drift apart.
//
// usage: node scripts/check-chart-env.mjs
import { readFileSync } from "node:fs"

const envPath = "docker/prod/launcher/src/env.ts"
const schemaPath = "charts/oxynote/values.schema.json"
const secretNames = [
	"OXYNOTE_AI_ASSISTANT_API_KEY",
	"OXYNOTE_AI_ASSISTANT_BEDROCK_ACCESS_KEY",
	"OXYNOTE_AI_ASSISTANT_BEDROCK_SECRET_ACCESS_KEY",
	"OXYNOTE_AI_ASSISTANT_BEDROCK_SESSION_TOKEN",
	"OXYNOTE_AI_ASSISTANT_VERTEX_SERVICE_ACCOUNT_JSON",
	"OXYNOTE_AUTH_SECRET",
	"OXYNOTE_CHANGE_DETECTION_API_KEY",
	"OXYNOTE_DATA_SOURCE_ENCRYPTION_KEYS",
	"OXYNOTE_DB_DSN",
	"OXYNOTE_GITHUB_APP_SIGNATURE_SECRET",
	"OXYNOTE_OBJECT_STORAGE_DSN",
	"OXYNOTE_SLACK_APP_CLIENT_SECRET",
	"OXYNOTE_SLACK_APP_SIGNATURE_SECRET",
	"OXYNOTE_SMTP_DSN",
	"OXYNOTE_SOCIAL_LOGIN_GITHUB_CLIENT_SECRET",
	"OXYNOTE_SOCIAL_LOGIN_GOOGLE_CLIENT_SECRET",
	"OXYNOTE_SOCIAL_LOGIN_OIDC_CLIENT_SECRET",
	"OXYNOTE_SOCIAL_LOGIN_SLACK_CLIENT_SECRET",
	"OXYNOTE_VALKEY_DSN",
]

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
].map((m) => m[1])

const schema = JSON.parse(
	readFileSync(new URL(`../${schemaPath}`, import.meta.url), "utf8"),
)
const plain = Object.keys(schema.properties.env.properties)
const looksSecret = /_(DSN|SECRET|KEY|KEYS|TOKEN|JSON)$/
// the chart sets OXYNOTE_PUBLIC_URL from publicUrl.
const listed = [...plain, ...secretNames, "OXYNOTE_PUBLIC_URL"]
const problems = []

for (const name of declared) {
	if (!listed.includes(name)) {
		problems.push(`${name} is declared in ${envPath} but not in ${schemaPath}`)
	}
}

for (const name of listed) {
	if (!declared.includes(name)) {
		problems.push(`${name} is in ${schemaPath} but not declared in ${envPath}`)
	}
}

for (const name of plain) {
	if (secretNames.includes(name)) {
		problems.push(`${name} is in both env and secretNames`)
	}

	if (looksSecret.test(name)) {
		problems.push(`${name} looks like a secret, so it belongs in secretNames`)
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
