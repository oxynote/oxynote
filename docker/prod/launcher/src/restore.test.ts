import { createHash } from "node:crypto"
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs"
import { join } from "node:path"
import { Readable } from "node:stream"
import { pack } from "tar-stream"
import { describe, it } from "vitest"
import type { Manifest } from "./archive.js"
import { backup } from "./backup.js"
import { client } from "./psql.js"
import { restore, type RestoreOptions } from "./restore.js"
import { commit, finishRestore } from "./staging.js"
import {
	collector,
	emptyVolume,
	fakeBin,
	fakeDatabasePassword,
	fakeVolume,
	recordingLog,
	volumeState,
} from "./test-helpers.js"

const external = { args: ["--dbname", "postgresql://db"], env: {} }

const manifest: Manifest = {
	format: 1,
	imageVersion: "0.12.0",
	migrationVersion: 13,
	database: "embedded",
	skipped: [],
	secretsFromEnv: [],
}

const schemaReset =
	"DROP SCHEMA IF EXISTS public CASCADE;\nDROP SCHEMA IF EXISTS dbos CASCADE;\nCREATE SCHEMA public;\n"
const schemaComment = "\nCOMMENT ON SCHEMA public IS 'oxynote-restore:id-1';\n"

describe("restore", () => {
	it("restores a backup into an empty volume", async ({ expect }) => {
		const dataDir = emptyVolume()
		const t = harness(await backupOf(), dataDir)

		await restore(t.options)

		expect(read(dataDir, "secrets/auth-secret")).toBe("auth")
		expect(
			read(
				dataDir,
				"object-storage/organizations/o/documents/d/files/f",
			),
		).toBe("png")
		expect(read(dataDir, "postgres/PG_VERSION")).toBe("18")
		expect(existsSync(join(dataDir, "restore"))).toBe(false)
		expect(t.clusters).toEqual([fakeDatabasePassword])
		expect(t.bin.loaded()).toBe(
			"SET transaction_timeout = 0;\nCREATE TABLE t ();\n",
		)
		expect(t.bin.log().map((line) => line.split(" ")[0])).toEqual([
			"pg_ctl",
			"psql",
			"pg_ctl",
		])
		expect(t.bin.log()[1]).toContain(
			"-v ON_ERROR_STOP=1 --single-transaction",
		)
		expect(t.log.lines.at(-1)).toBe(
			"info restore complete. Start the instance",
		)
	})

	it("replaces an instance only with --replace, and drops its search index", async ({
		expect,
	}) => {
		const dataDir = fakeVolume()

		writeFileSync(join(dataDir, "secrets/auth-secret"), "old")

		const before = volumeState(dataDir)
		const refused = harness(await backupOf(), dataDir)

		await expect(restore(refused.options)).rejects.toThrow(
			"this volume already holds an instance. Run restore --replace to replace it",
		)
		expect(volumeState(dataDir)).toEqual(before)

		const t = harness(await backupOf(), dataDir, { replace: true })

		await restore(t.options)

		expect(read(dataDir, "secrets/auth-secret")).toBe("auth")
		expect(existsSync(join(dataDir, "search"))).toBe(false)
	})

	it("replaces the old instance's objects even when the backup holds none", async ({
		expect,
	}) => {
		const dataDir = fakeVolume()
		const t = harness(
			await backupOf({ remoteObjects: true }),
			dataDir,
			{
				replace: true,
			},
		)

		await restore(t.options)

		expect(volumeState(join(dataDir, "object-storage"))).toEqual({})
	})

	it("counts a volume with a cluster but no secrets as an instance", async ({
		expect,
	}) => {
		const dataDir = emptyVolume()

		mkdirSync(join(dataDir, "postgres"))
		writeFileSync(join(dataDir, "postgres/PG_VERSION"), "18")

		await expect(
			restore(harness(await backupOf(), dataDir).options),
		).rejects.toThrow("already holds an instance")
	})

	it("refuses while the instance runs, even with --replace", async ({
		expect,
	}) => {
		const dataDir = fakeVolume()

		writeFileSync(join(dataDir, "postgres/postmaster.pid"), "1")

		const before = volumeState(dataDir)
		const t = harness(await backupOf(), dataDir, { replace: true })

		await expect(restore(t.options)).rejects.toThrow(
			"the instance is running in another container, or it did not stop cleanly",
		)
		expect(volumeState(dataDir)).toEqual(before)
	})

	it("leaves a restore staged by the running instance alone", async ({
		expect,
	}) => {
		const dataDir = fakeVolume()

		writeFileSync(join(dataDir, "postgres/postmaster.pid"), "1")
		mkdirSync(join(dataDir, "restore/secrets"), { recursive: true })
		writeFileSync(
			join(dataDir, "restore/secrets/auth-secret"),
			"new",
		)
		commit(dataDir, { database: "embedded" })

		const before = volumeState(dataDir)
		const t = harness(await backupOf(), dataDir, { replace: true })

		await expect(restore(t.options)).rejects.toThrow(
			"the instance is running in another container",
		)
		expect(volumeState(dataDir)).toEqual(before)
	})

	it("refuses a terminal as its input", async ({ expect }) => {
		const t = harness(Buffer.alloc(0), emptyVolume(), {
			inputIsTerminal: true,
		})

		await expect(restore(t.options)).rejects.toThrow(
			"restore reads a backup from standard input: restore < oxynote-backup.tar",
		)
	})

	it("refuses a newer backup before it writes anything", async ({
		expect,
	}) => {
		const dataDir = fakeVolume()
		const before = volumeState(dataDir)
		const t = harness(await backupOf(), dataDir, {
			replace: true,
			image: { version: "0.11.0", migrationVersion: 13 },
		})

		await expect(restore(t.options)).rejects.toThrow(
			"the backup comes from image 0.12.0, newer than this image's 0.11.0",
		)
		expect(volumeState(dataDir)).toEqual(before)
		expect(t.bin.log()).toEqual([])
	})

	it("leaves the volume as it was when the backup is damaged", async ({
		expect,
	}) => {
		const dataDir = fakeVolume()
		const before = volumeState(dataDir)
		const tar = await backupOf()

		// the entry's body starts in the block after its header.
		tar[tar.indexOf("secrets/auth-secret") + 512] = "X".charCodeAt(
			0,
		)

		const t = harness(tar, dataDir, { replace: true })

		await expect(restore(t.options)).rejects.toThrow(
			"cannot read the backup: secrets/auth-secret does not match its checksum",
		)
		expect(volumeState(dataDir)).toEqual(before)
	})

	const malformed = [
		{
			name: "an entry restore does not know",
			input: () =>
				archive([
					[
						"manifest.json",
						JSON.stringify(manifest),
					],
					["etc/passwd", "x"],
				]),
			expected: "cannot read the backup: it holds an unknown entry: etc/passwd",
		},
		{
			name: "a backup without a database dump",
			input: () =>
				archive([
					[
						"manifest.json",
						JSON.stringify(manifest),
					],
				]),
			expected: "cannot read the backup: it holds no database dump",
		},
		{
			name: "a backup without a database password",
			input: () =>
				archive([
					[
						"manifest.json",
						JSON.stringify(manifest),
					],
					["database.sql.gz", "x"],
				]),
			expected: "cannot read the backup: it holds no database password this image generates",
		},
		// the password reaches SQL as the superuser
		{
			name: "a database password the image never generates",
			input: () =>
				archive([
					[
						"manifest.json",
						JSON.stringify(manifest),
					],
					["database.sql.gz", "x"],
					[
						"secrets/database-password",
						"x'; ALTER ROLE oxynote SUPERUSER; --",
					],
				]),
			expected: "cannot read the backup: it holds no database password this image generates",
		},
	]

	for (const c of malformed) {
		it(`refuses ${c.name}`, async ({ expect }) => {
			const dataDir = emptyVolume()
			const t = harness(await c.input(), dataDir)

			await expect(restore(t.options)).rejects.toThrow(
				c.expected,
			)
			expect(volumeState(dataDir)).toEqual({})
			expect(t.clusters).toEqual([])
		})
	}

	it("leaves the volume as it was when the dump fails to load", async ({
		expect,
	}) => {
		const dataDir = fakeVolume()
		const before = volumeState(dataDir)
		const t = harness(await backupOf(), dataDir, { replace: true })

		t.bin.set("fail-load")

		await expect(restore(t.options)).rejects.toThrow(
			"psql exited with code 3: ERROR: boom",
		)
		expect(volumeState(dataDir)).toEqual(before)
		expect(t.bin.log().at(-1)).toBe("pg_ctl stop")
	})

	it("leaves the volume as it was when the server does not start", async ({
		expect,
	}) => {
		const dataDir = fakeVolume()
		const before = volumeState(dataDir)
		const t = harness(await backupOf(), dataDir, { replace: true })

		t.bin.set("fail-start")

		await expect(restore(t.options)).rejects.toThrow(
			"pg_ctl exited with code 1: could not start",
		)
		expect(volumeState(dataDir)).toEqual(before)
	})

	it("skips the objects when this instance keeps them in a remote store", async ({
		expect,
	}) => {
		const dataDir = emptyVolume()
		const t = harness(await backupOf(), dataDir, {
			remoteObjects: true,
		})

		await restore(t.options)

		expect(existsSync(join(dataDir, "object-storage"))).toBe(false)
		expect(t.log.lines).toContain(
			"info skipping object-storage/: the objects are in a remote store",
		)
	})

	it("warns about what the backup could not hold", async ({ expect }) => {
		const t = harness(
			await backupOf({
				remoteObjects: true,
				secretsFromEnv: ["OXYNOTE_AUTH_SECRET"],
			}),
			emptyVolume(),
		)

		await restore(t.options)

		expect(t.log.lines).toContain(
			"warn the backup holds no objects, because its instance kept them in a remote store. Uploaded files are missing until they are copied in",
		)
		expect(t.log.lines).toContain(
			"warn the backed-up instance took OXYNOTE_AUTH_SECRET from its environment. Set the same variables on this one",
		)
	})

	describe("inside the running instance", () => {
		it("builds the new instance aside and restarts the running one", async ({
			expect,
		}) => {
			const dataDir = fakeVolume()
			const before = volumeState(dataDir)
			const t = harness(await backupOf(), dataDir, {
				instanceRunning: () => Promise.resolve(true),
			})

			await restore(t.options)

			expect(t.restarts).toEqual(["restart"])
			expect(t.clusters).toEqual([fakeDatabasePassword])
			expect(
				Object.fromEntries(
					Object.entries(
						volumeState(dataDir),
					).filter(
						([path]) =>
							!path.startsWith(
								"restore/",
							),
					),
				),
			).toEqual(before)
			expect(
				read(dataDir, "restore/postgres/PG_VERSION"),
			).toBe("18")
			expect(t.log.lines.at(-1)).toBe(
				"info the backup is staged. The instance restarts now and finishes the restore",
			)

			await finishRestore(dataDir, undefined, recordingLog())

			expect(read(dataDir, "secrets/database-password")).toBe(
				fakeDatabasePassword,
			)
			expect(existsSync(join(dataDir, "restore"))).toBe(false)
		})

		it("leaves an external load to the restart", async ({
			expect,
		}) => {
			const dataDir = fakeVolume({ cluster: false })
			const t = harness(await backupOf(), dataDir, {
				external,
				instanceRunning: () => Promise.resolve(true),
			})

			await restore(t.options)

			expect(t.restarts).toEqual(["restart"])
			expect(
				t.bin
					.log()
					.some((line) =>
						line.includes(
							"--single-transaction",
						),
					),
			).toBe(false)

			await finishRestore(
				dataDir,
				client(t.bin.dir, external),
				recordingLog(),
			)

			expect(t.bin.loaded()).toBe(
				`${schemaReset}SET transaction_timeout = 0;\nCREATE TABLE t ();\n${schemaComment}`,
			)
			expect(read(dataDir, "secrets/auth-secret")).toBe(
				"auth",
			)
		})

		it("refuses a second restore while one waits for the restart", async ({
			expect,
		}) => {
			const dataDir = fakeVolume()

			mkdirSync(join(dataDir, "restore"))
			commit(dataDir, { database: "embedded" })

			const t = harness(await backupOf(), dataDir, {
				instanceRunning: () => Promise.resolve(true),
			})

			await expect(restore(t.options)).rejects.toThrow(
				"a restore is already waiting for the instance to restart. Restart it first",
			)
			expect(t.restarts).toEqual([])
		})

		// a second restore, or one killed while it was staging
		it("refuses while another restore is staging", async ({
			expect,
		}) => {
			const dataDir = fakeVolume()

			mkdirSync(join(dataDir, "restore"))
			writeFileSync(
				join(dataDir, "restore/database.sql.gz"),
				"x",
			)

			const t = harness(await backupOf(), dataDir, {
				instanceRunning: () => Promise.resolve(true),
			})

			await expect(restore(t.options)).rejects.toThrow(
				"another restore is running, or an interrupted one left restore/ behind",
			)
			expect(read(dataDir, "restore/database.sql.gz")).toBe(
				"x",
			)
		})

		it("leaves the running instance alone when the backup is damaged", async ({
			expect,
		}) => {
			const dataDir = fakeVolume()
			const before = volumeState(dataDir)
			const t = harness(
				(await backupOf()).subarray(0, 1500),
				dataDir,
				{
					instanceRunning: () =>
						Promise.resolve(true),
				},
			)

			await expect(restore(t.options)).rejects.toThrow(
				"cannot read the backup",
			)
			expect(volumeState(dataDir)).toEqual(before)
			expect(t.restarts).toEqual([])
		})
	})

	describe("into an external database", () => {
		it("replaces both schemas in one transaction and marks it", async ({
			expect,
		}) => {
			const dataDir = emptyVolume()
			const t = harness(await backupOf(), dataDir, {
				external,
			})

			await restore(t.options)

			expect(t.bin.loaded()).toBe(
				`${schemaReset}SET transaction_timeout = 0;\nCREATE TABLE t ();\n${schemaComment}`,
			)
			expect(t.bin.log().at(-1)).toBe(
				"psql -X -q -v ON_ERROR_STOP=1 --single-transaction --dbname postgresql://db",
			)
			expect(t.clusters).toEqual([])
			expect(read(dataDir, "secrets/auth-secret")).toBe(
				"auth",
			)
			expect(existsSync(join(dataDir, "postgres"))).toBe(
				false,
			)
		})

		// pg_dump 17 and later write a setting PostgreSQL 16 rejects
		it("leaves out transaction_timeout for a server older than 17", async ({
			expect,
		}) => {
			const t = harness(await backupOf(), emptyVolume(), {
				external,
			})

			t.bin.set("version", "160004")

			await restore(t.options)

			expect(t.bin.loaded()).toBe(
				`${schemaReset}CREATE TABLE t ();\n${schemaComment}`,
			)
		})

		it("leaves the rest of a long dump as it was", async ({
			expect,
		}) => {
			const t = harness(
				await backupOf({ bigDump: true }),
				emptyVolume(),
				{
					external,
				},
			)

			t.bin.set("version", "160004")

			await restore(t.options)

			expect(t.bin.loaded()).toBe(
				`${schemaReset}CREATE TABLE t ();\n${"x".repeat(100000)}${schemaComment}`,
			)
		})

		it("refuses while another client is connected", async ({
			expect,
		}) => {
			const t = harness(await backupOf(), emptyVolume(), {
				external,
			})

			t.bin.set("others", "2")

			await expect(restore(t.options)).rejects.toThrow(
				"the database has 2 other connections. Run restore inside the running instance, or stop it first",
			)
		})

		it("counts a database with the schema as an instance", async ({
			expect,
		}) => {
			const t = harness(await backupOf(), emptyVolume(), {
				external,
			})

			t.bin.set("tables", "t")

			await expect(restore(t.options)).rejects.toThrow(
				"already holds an instance",
			)
		})

		it("leaves the files as they were when the load fails", async ({
			expect,
		}) => {
			const dataDir = fakeVolume({ cluster: false })
			const before = volumeState(dataDir)
			const t = harness(await backupOf(), dataDir, {
				external,
				replace: true,
			})

			t.bin.set("fail-load")

			await expect(restore(t.options)).rejects.toThrow(
				"ERROR: boom",
			)
			expect(volumeState(dataDir)).toEqual(before)
		})
	})
})

// backupOf takes a real backup of a fake volume.
async function backupOf(
	overrides: {
		remoteObjects?: boolean
		secretsFromEnv?: string[]
		bigDump?: boolean
	} = {},
): Promise<Buffer> {
	const out = collector()
	const bin = fakeBin()

	if (overrides.bigDump) {
		bin.set("big-dump")
	}

	await backup({
		dataDir: fakeVolume(),
		binDir: bin.dir,
		connection: { args: [], env: {} },
		external: false,
		remoteObjects: overrides.remoteObjects ?? false,
		secretsFromEnv: overrides.secretsFromEnv ?? [],
		imageVersion: "0.12.0",
		out,
		outIsTerminal: false,
		log: recordingLog(),
	})

	return out.bytes()
}

// archive builds a backup by hand, for the shapes backup never writes.
async function archive(entries: [string, string][]): Promise<Buffer> {
	const p = pack()
	const out = collector()
	const done = new Promise((resolve) => out.on("finish", resolve))
	const sums: Record<string, string> = {}

	p.pipe(out)

	for (const [name, body] of entries) {
		sums[name] = createHash("sha256").update(body).digest("hex")
		p.entry({ name }, body)
	}

	p.entry({ name: "checksums.json" }, JSON.stringify(sums))
	p.finalize()
	await done

	return out.bytes()
}

function harness(
	tar: Buffer,
	dataDir: string,
	overrides: Partial<RestoreOptions> = {},
) {
	const bin = fakeBin()
	const log = recordingLog()
	const clusters: string[] = []
	const restarts: string[] = []

	const options: RestoreOptions = {
		dataDir,
		binDir: bin.dir,
		external: undefined,
		env: {},
		log,
		socketDir: join(dataDir, "..", "sock"),
		hbaFile: "/hba",
		remoteObjects: false,
		image: { version: "0.12.0", migrationVersion: 13 },
		replace: false,
		input: Readable.from([tar]),
		inputIsTerminal: false,
		createCluster: (dir, password) => {
			clusters.push(password)
			mkdirSync(dir, { recursive: true })
			writeFileSync(join(dir, "PG_VERSION"), "18")
		},
		instanceRunning: () => Promise.resolve(false),
		restartInstance: () => {
			restarts.push("restart")
		},
		restoreId: () => "id-1",
		...overrides,
	}

	return { bin, log, clusters, restarts, options }
}

function read(dataDir: string, path: string): string {
	return readFileSync(join(dataDir, path), "utf8")
}
