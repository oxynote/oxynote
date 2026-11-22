import { randomBytes } from "node:crypto"
import { mkdirSync, readFileSync, writeFileSync } from "node:fs"
import { join } from "node:path"

export interface Secrets {
	authSecret: string
	dataSourceEncryptionKeys: string
	githubInstallationSigningSecret: string
	slackInstallationSigningSecret: string
	databasePassword: string
}

export interface SecretOverrides {
	authSecret: string | undefined
	dataSourceEncryptionKeys: string | undefined
}

export interface SecretReport {
	generated: string[]
	fromVolume: string[]
	fromEnv: string[]
}

export function isNoEntry(err: unknown): boolean {
	return err instanceof Error && "code" in err && err.code === "ENOENT"
}

function isExists(err: unknown): boolean {
	return err instanceof Error && "code" in err && err.code === "EEXIST"
}

function readSecret(path: string): string | undefined {
	try {
		return readFileSync(path, "utf8").trim() || undefined
	} catch (err) {
		if (isNoEntry(err)) {
			return undefined
		}

		throw err
	}
}

// the shape of every secret generateAscii draws.
export const generatedAsciiPattern = /^[A-Za-z0-9_-]{32}$/

// generateAscii draws 24 random bytes as base64url, which is exactly the 32
// ASCII bytes core's installation signing secrets must be.
function generateAscii(): string {
	return randomBytes(24).toString("base64url")
}

// generateDataSourceKey draws the 32 random bytes of one AES-256 key, in
// the base64 form core's keyring reads.
function generateDataSourceKey(): string {
	return randomBytes(32).toString("base64")
}

// ensureSecrets resolves every internal secret with the precedence the
// established all-in-one images use: an explicit override wins and is never
// written to disk, an existing volume file is reused, and only a secret
// with neither is generated — from the CSPRNG — and persisted with
// owner-only permissions.
export function ensureSecrets(
	dir: string,
	overrides: SecretOverrides,
): { secrets: Secrets; report: SecretReport } {
	const report: SecretReport = {
		generated: [],
		fromVolume: [],
		fromEnv: [],
	}

	function resolve(
		file: string,
		generate: () => string,
		override?: string,
	): string {
		if (override !== undefined) {
			report.fromEnv.push(file)

			return override
		}

		const path = join(dir, file)
		const existing = readSecret(path)

		if (existing !== undefined) {
			report.fromVolume.push(file)

			return existing
		}

		const value = generate()

		try {
			mkdirSync(dir, { recursive: true, mode: 0o700 })
			writeFileSync(path, value, { flag: "wx", mode: 0o600 })
		} catch (err) {
			// a concurrent boot on the same volume may have written
			// the file between the read and the write; its value
			// wins.
			if (isExists(err)) {
				const written = readSecret(path)

				if (written !== undefined) {
					// NOCOV: requires a concurrent writer
					// between the read and the write.
					report.fromVolume.push(file)

					return written
				}
			}

			throw new Error(
				`cannot persist the generated secret "${file}" under ${dir} — the image needs a writable data volume there (${err instanceof Error ? err.message : String(err)})`,
				{ cause: err },
			)
		}

		report.generated.push(file)

		return value
	}

	const secrets: Secrets = {
		authSecret: resolve(
			"auth-secret",
			generateAscii,
			overrides.authSecret,
		),
		dataSourceEncryptionKeys: resolve(
			"data-source-encryption-key",
			generateDataSourceKey,
			overrides.dataSourceEncryptionKeys,
		),
		githubInstallationSigningSecret: resolve(
			"github-installation-signing-secret",
			generateAscii,
		),
		slackInstallationSigningSecret: resolve(
			"slack-installation-signing-secret",
			generateAscii,
		),
		databasePassword: resolve("database-password", generateAscii),
	}

	return { secrets, report }
}
