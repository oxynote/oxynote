import { createHash } from "node:crypto"
import {
	chmodSync,
	existsSync,
	mkdirSync,
	mkdtempSync,
	readdirSync,
	readFileSync,
	writeFileSync,
} from "node:fs"
import { tmpdir } from "node:os"
import { join, relative } from "node:path"
import { Writable } from "node:stream"
import type { Config } from "./env.js"
import type { Logger } from "./logging.js"
import type { Secrets } from "./secrets.js"

// a complete, valid environment. Each test overrides only the variables it
// is about, so the rest can never be the reason a case passes or fails.
export function completeEnv(
	overrides: Record<string, string | undefined> = {},
): Record<string, string | undefined> {
	return {
		OXYNOTE_PUBLIC_URL: "https://notes.example.com",
		OXYNOTE_DB_DSN: "postgresql://user:pass@postgres/oxynote",
		OXYNOTE_VALKEY_DSN: "redis://valkey:6379",
		OXYNOTE_OBJECT_STORAGE_DSN:
			"http://access:secret@s3.example.com/documents",
		...overrides,
	}
}

// the configuration completeEnv() parses into, for tests that start after
// loading.
export function testConfig(overrides: Partial<Config> = {}): Config {
	return {
		publicOrigin: "https://notes.example.com",
		publicHostPort: "notes.example.com",
		cookieDomain: "notes.example.com",
		publicWebSocketOrigin: "wss://notes.example.com",
		databaseDsn: "postgresql://user:pass@postgres/oxynote",
		valkeyDsn: "redis://valkey:6379",
		objectStorage: {
			url: "http://s3.example.com",
			accessKey: "access",
			secretKey: "secret",
			bucket: "documents",
			region: "",
		},
		smtp: undefined,
		emailFromAddress: "",
		changeDetection: undefined,
		githubApp: undefined,
		slackApp: undefined,
		socialLogin: {},
		oidcLogin: undefined,
		passwordLoginDisabled: false,
		aiAssistant: {
			PROVIDER: "",
			MODEL: "",
			API_KEY: "",
			BASE_URL: "",
			MAX_TOKENS: "",
			REQUEST_TIMEOUT: "",
			SUMMARY_MODEL: "",
			AZURE_API_VERSION: "",
			BEDROCK_REGION: "",
			BEDROCK_ACCESS_KEY: "",
			BEDROCK_SECRET_ACCESS_KEY: "",
			BEDROCK_SESSION_TOKEN: "",
			VERTEX_PROJECT_ID: "",
			VERTEX_REGION: "",
			VERTEX_SERVICE_ACCOUNT_JSON: "",
		},
		maxOrganizations: "",
		maxOrganizationMembers: "",
		maxDocumentHistoryEntries: "",
		documentHistoryRetention: "",
		logLevel: "WARN",
		termsOfServiceUrl: "",
		privacyPolicyUrl: "",
		crashReportingDisabled: false,
		authSecret: undefined,
		dataSourceEncryptionKeys: undefined,
		...overrides,
	}
}

export function testSecrets(): Secrets {
	return {
		authSecret: "test-auth-secret",
		dataSourceEncryptionKeys:
			"MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		githubInstallationSigningSecret: "github-installation-secret",
		slackInstallationSigningSecret: "slack-installation-secret",
		databasePassword: "test-database-password",
	}
}

// fakes of the PostgreSQL client tools, as shell scripts in a temporary bin
// directory. Each logs its arguments to bin/log. A file next to them steers
// it: fail-dump, fail-load and fail-start make that step fail, big-dump
// makes the dump longer than its preamble, lost-reply fails a load after
// its commit; migration, version, others, tables and comment hold what a
// query answers. A load writes its input to bin/loaded, and a schema
// comment in it to bin/comment once the input reaches its COMMIT, the way
// the database would keep it.
const scripts = {
	pg_dump: `#!/bin/sh
dir=$(dirname "$0")
echo "pg_dump $*" >> "$dir/log"
if [ -f "$dir/fail-dump" ]; then echo "connection refused" >&2; exit 1; fi
printf 'SET transaction_timeout = 0;\\nCREATE TABLE t ();\\n'
if [ -f "$dir/big-dump" ]; then head -c 100000 /dev/zero | tr '\\0' x; fi
`,
	psql: `#!/bin/sh
dir=$(dirname "$0")
echo "psql $*" >> "$dir/log"
case "$*" in
*" -c "*)
	case "$*" in
	*"goose_db_version WHERE"*) cat "$dir/migration" 2>/dev/null || echo 13 ;;
	*server_version_num*) cat "$dir/version" 2>/dev/null || echo 180007 ;;
	*pg_stat_activity*) cat "$dir/others" 2>/dev/null || echo 0 ;;
	*to_regclass*) cat "$dir/tables" 2>/dev/null || echo f ;;
	*obj_description*) cat "$dir/comment" 2>/dev/null || echo "" ;;
	esac ;;
*)
	cat > "$dir/loaded"
	if [ -f "$dir/fail-load" ]; then echo "ERROR: boom" >&2; exit 3; fi
	if grep -qx "COMMIT;" "$dir/loaded"; then
		sed -n "s/^COMMENT ON SCHEMA public IS '\\(.*\\)';$/\\1/p" "$dir/loaded" > "$dir/comment"
	fi
	if [ -f "$dir/lost-reply" ]; then echo "server closed the connection unexpectedly" >&2; exit 2; fi ;;
esac
`,
	pg_ctl: `#!/bin/sh
dir=$(dirname "$0")
echo "pg_ctl $1" >> "$dir/log"
if [ "$1" = start ] && [ -f "$dir/fail-start" ]; then echo "could not start" >&2; exit 1; fi
`,
}

export interface FakeBin {
	dir: string
	set(name: string, value?: string): void
	log(): string[]
	loaded(): string
}

export function fakeBin(): FakeBin {
	const dir = mkdtempSync(join(tmpdir(), "oxynote-bin-"))

	for (const [name, body] of Object.entries(scripts)) {
		writeFileSync(join(dir, name), body)
		chmodSync(join(dir, name), 0o755)
	}

	return {
		dir,
		set(name, value = "") {
			writeFileSync(join(dir, name), value)
		},
		log() {
			return existsSync(join(dir, "log"))
				? readFileSync(join(dir, "log"), "utf8")
						.trim()
						.split("\n")
				: []
		},
		loaded() {
			return readFileSync(join(dir, "loaded"), "utf8")
		},
	}
}

// a password in the shape the image generates.
export const fakeDatabasePassword = "fake-database-password-32-chars_"

// a data volume as a running instance leaves it: secrets, a cluster, a
// search index and one uploaded object.
export function fakeVolume(options: { cluster?: boolean } = {}): string {
	const dir = mkdtempSync(join(tmpdir(), "oxynote-data-"))
	const files: Record<string, string> = {
		"secrets/auth-secret": "auth",
		"secrets/database-password": fakeDatabasePassword,
		"object-storage/organizations/o/documents/d/files/f": "png",
		"search/index_meta.json": "{}",
	}

	if (options.cluster !== false) {
		files["postgres/PG_VERSION"] = "18"
	}

	for (const [path, body] of Object.entries(files)) {
		mkdirSync(join(dir, path, ".."), { recursive: true })
		writeFileSync(join(dir, path), body)
	}

	return dir
}

export function emptyVolume(): string {
	return mkdtempSync(join(tmpdir(), "oxynote-data-"))
}

// the volume's files and their contents, for comparing a volume before and
// after.
export function volumeState(dir: string): Record<string, string> {
	const state: Record<string, string> = {}

	for (const entry of readdirSync(dir, {
		recursive: true,
		withFileTypes: true,
	})) {
		if (entry.isFile()) {
			const path = join(entry.parentPath, entry.name)

			state[relative(dir, path)] = createHash("sha256")
				.update(readFileSync(path))
				.digest("hex")
		}
	}

	return state
}

export function recordingLog(): Logger & { lines: string[] } {
	const lines: string[] = []

	return {
		lines,
		info: (message) => lines.push(`info ${message}`),
		warn: (message) => lines.push(`warn ${message}`),
		error: (message) => lines.push(`error ${message}`),
	}
}

export function collector(): Writable & { bytes(): Buffer } {
	const chunks: Buffer[] = []
	const stream = new Writable({
		write(chunk: Buffer, _encoding, done) {
			chunks.push(chunk)
			done()
		},
	})

	return Object.assign(stream, { bytes: () => Buffer.concat(chunks) })
}
