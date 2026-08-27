import { randomBytes } from "node:crypto"
import { mkdirSync, readFileSync, writeFileSync } from "node:fs"
import { join } from "node:path"

export interface Secrets {
	authSecret: string
	dataSourceEncryptionKey: string
	githubInstallationSigningSecret: string
	slackInstallationSigningSecret: string
}

export interface SecretOverrides {
	authSecret: string | undefined
	dataSourceEncryptionKey: string | undefined
}

export interface SecretReport {
	generated: string[]
	fromVolume: string[]
	fromEnv: string[]
}

function isNoEntry(err: unknown): boolean {
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

// ensureSecrets resolves every internal secret with the precedence the
// established all-in-one images use: an explicit override wins and is never
// written to disk, an existing volume file is reused, and only a secret
// with neither is generated — from the CSPRNG — and persisted with
// owner-only permissions. Everything generated is 32 ASCII bytes, which
// satisfies both the AES-256 key length the data-source encryption needs
// and core's exactly-32-byte installation signing secrets.
export function ensureSecrets(
	dir: string,
	overrides: SecretOverrides,
): { secrets: Secrets; report: SecretReport } {
	const report: SecretReport = {
		generated: [],
		fromVolume: [],
		fromEnv: [],
	}

	function resolve(file: string, override?: string): string {
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

		// base64url of 24 random bytes is exactly 32 ASCII bytes.
		const value = randomBytes(24).toString("base64url")

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
		authSecret: resolve("auth-secret", overrides.authSecret),
		dataSourceEncryptionKey: resolve(
			"data-source-encryption-key",
			overrides.dataSourceEncryptionKey,
		),
		githubInstallationSigningSecret: resolve(
			"github-installation-signing-secret",
		),
		slackInstallationSigningSecret: resolve(
			"slack-installation-signing-secret",
		),
	}

	return { secrets, report }
}
