import { createHash } from "node:crypto"
import { mkdirSync, readdirSync, rmSync, writeFileSync } from "node:fs"
import { join } from "node:path"
import { Readable } from "node:stream"
import { gunzipSync } from "node:zlib"
import { extract } from "tar-stream"
import { describe, it } from "vitest"
import { backup, removePartialBackups, type BackupOptions } from "./backup.js"
import {
	collector,
	fakeBin,
	fakeVolume,
	recordingLog,
	type FakeBin,
} from "./test-helpers.js"

interface Entry {
	name: string
	body: Buffer
}

async function entries(tar: Buffer): Promise<Entry[]> {
	const found: Entry[] = []
	const ex = extract()

	Readable.from([tar]).pipe(ex)

	for await (const entry of ex) {
		const chunks: Buffer[] = []

		for await (const chunk of entry) {
			chunks.push(chunk as Buffer)
		}

		found.push({
			name: entry.header.name,
			body: Buffer.concat(chunks),
		})
	}

	return found
}

function options(
	bin: FakeBin,
	dataDir: string,
	overrides: Partial<BackupOptions> = {},
) {
	const out = collector()
	const log = recordingLog()

	return {
		out,
		log,
		options: {
			dataDir,
			binDir: bin.dir,
			connection: { args: ["-h", "/sock"], env: {} },
			external: false,
			remoteObjects: false,
			secretsFromEnv: [],
			imageVersion: "0.12.0",
			out,
			outIsTerminal: false,
			log,
			...overrides,
		},
	}
}

describe("backup", () => {
	it("writes the manifest first, then the dump, secrets and objects, and the checksums last", async ({
		expect,
	}) => {
		const bin = fakeBin()
		const t = options(bin, fakeVolume())

		await backup(t.options)

		const found = await entries(t.out.bytes())

		expect(found.map((e) => e.name)).toEqual([
			"manifest.json",
			"database.sql.gz",
			"secrets/auth-secret",
			"secrets/database-password",
			"object-storage/organizations/o/documents/d/files/f",
			"checksums.json",
		])
		expect(JSON.parse(found[0]?.body.toString() ?? "")).toEqual({
			format: 1,
			imageVersion: "0.12.0",
			migrationVersion: 13,
			database: "embedded",
			skipped: [],
			secretsFromEnv: [],
		})
		expect(
			gunzipSync(
				found[1]?.body ?? Buffer.alloc(0),
			).toString(),
		).toBe("SET transaction_timeout = 0;\nCREATE TABLE t ();\n")

		const sums = JSON.parse(
			found.at(-1)?.body.toString() ?? "",
		) as Record<string, string>

		for (const entry of found.slice(0, -1)) {
			expect(sums[entry.name]).toBe(
				createHash("sha256")
					.update(entry.body)
					.digest("hex"),
			)
		}
	})

	// --no-owner and --no-acl let the dump load into a database another
	// role owns
	it("dumps without owners or grants, and before reading the migration version", async ({
		expect,
	}) => {
		const bin = fakeBin()
		const t = options(bin, fakeVolume())

		await backup(t.options)

		expect(bin.log()[0]).toBe(
			"pg_dump --format=plain --no-owner --no-acl -h /sock",
		)
		expect(bin.log()[1]).toContain("goose_db_version")
	})

	it("refuses a terminal and writes nothing", async ({ expect }) => {
		const bin = fakeBin()
		const dataDir = fakeVolume()
		const t = options(bin, dataDir, { outIsTerminal: true })

		await expect(backup(t.options)).rejects.toThrow(
			"backup writes a tar to standard output. Redirect it to a file: backup > oxynote-backup.tar",
		)
		expect(bin.log()).toEqual([])
		expect(readdirSync(dataDir)).not.toContainEqual(
			expect.stringMatching(/^backup\.partial-/),
		)
	})

	it("skips the objects of a remote store and records that", async ({
		expect,
	}) => {
		const bin = fakeBin()
		const t = options(bin, fakeVolume(), { remoteObjects: true })

		await backup(t.options)

		const found = await entries(t.out.bytes())

		expect(
			found.some((e) => e.name.startsWith("object-storage/")),
		).toBe(false)
		expect(
			JSON.parse(found[0]?.body.toString() ?? ""),
		).toMatchObject({
			skipped: ["object-storage"],
		})
		expect(t.log.lines).toContain(
			"info skipping object-storage/: the objects are in a remote store",
		)
	})

	it("records an external database and the secrets taken from env", async ({
		expect,
	}) => {
		const bin = fakeBin()
		const t = options(bin, fakeVolume(), {
			external: true,
			secretsFromEnv: ["OXYNOTE_AUTH_SECRET"],
		})

		await backup(t.options)

		const found = await entries(t.out.bytes())

		expect(
			JSON.parse(found[0]?.body.toString() ?? ""),
		).toMatchObject({
			database: "external",
			secretsFromEnv: ["OXYNOTE_AUTH_SECRET"],
		})
	})

	it("backs up a volume without objects", async ({ expect }) => {
		const bin = fakeBin()
		const dataDir = fakeVolume()

		rmSync(join(dataDir, "object-storage"), { recursive: true })

		const t = options(bin, dataDir)

		await backup(t.options)

		expect(
			(await entries(t.out.bytes())).map((e) => e.name),
		).toEqual([
			"manifest.json",
			"database.sql.gz",
			"secrets/auth-secret",
			"secrets/database-password",
			"checksums.json",
		])
	})

	it("leaves out the temp file of an object being written", async ({
		expect,
	}) => {
		const bin = fakeBin()
		const dataDir = fakeVolume()

		writeFileSync(
			join(
				dataDir,
				"object-storage/organizations/o/.tmp-123",
			),
			"half",
		)

		const t = options(bin, dataDir)

		await backup(t.options)

		expect(
			(await entries(t.out.bytes())).map((e) => e.name),
		).not.toContain("object-storage/organizations/o/.tmp-123")
	})

	it("fails with the dump's own error and leaves no partial dump", async ({
		expect,
	}) => {
		const bin = fakeBin()
		const dataDir = fakeVolume()
		const t = options(bin, dataDir)

		bin.set("fail-dump")

		await expect(backup(t.options)).rejects.toThrow(
			"pg_dump exited with code 1: connection refused",
		)
		expect(readdirSync(dataDir)).not.toContainEqual(
			expect.stringMatching(/^backup\.partial-/),
		)
		expect(t.out.bytes().length).toBe(0)
	})

	// a scheduled backup can overlap one started by hand
	it("writes two whole archives when two backups run at once", async ({
		expect,
	}) => {
		const bin = fakeBin()
		const dataDir = fakeVolume()
		const first = options(bin, dataDir)
		const second = options(bin, dataDir)

		await Promise.all([
			backup(first.options),
			backup(second.options),
		])

		for (const t of [first, second]) {
			const dump = (await entries(t.out.bytes()))[1]

			expect(
				gunzipSync(
					dump?.body ?? Buffer.alloc(0),
				).toString(),
			).toBe(
				"SET transaction_timeout = 0;\nCREATE TABLE t ();\n",
			)
		}
	})
})

describe("removePartialBackups", () => {
	it("removes what killed backups left and nothing else", ({
		expect,
	}) => {
		const dataDir = fakeVolume()

		mkdirSync(join(dataDir, "backup.partial-abc"))

		removePartialBackups(dataDir)

		expect(readdirSync(dataDir).toSorted()).toEqual([
			"object-storage",
			"postgres",
			"search",
			"secrets",
		])
	})
})
