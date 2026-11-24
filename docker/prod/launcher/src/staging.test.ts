import {
	existsSync,
	mkdirSync,
	readFileSync,
	rmSync,
	writeFileSync,
} from "node:fs"
import { join } from "node:path"
import { gzipSync } from "node:zlib"
import { describe, it } from "vitest"
import { client } from "./psql.js"
import { commit, finishRestore, type Commit } from "./staging.js"
import {
	fakeBin,
	fakeVolume,
	recordingLog,
	volumeState,
} from "./test-helpers.js"

const external = { args: ["--dbname", "postgresql://db"], env: {} }

describe("finishRestore", () => {
	it("does nothing without a staged restore", async ({ expect }) => {
		const dataDir = fakeVolume()
		const before = volumeState(dataDir)
		const log = recordingLog()

		await finishRestore(dataDir, undefined, log)

		expect(volumeState(dataDir)).toEqual(before)
		expect(log.lines).toEqual([])
	})

	it("removes a restore that never committed", async ({ expect }) => {
		const dataDir = staged()
		const log = recordingLog()

		await finishRestore(dataDir, undefined, log)

		expect(existsSync(join(dataDir, "restore"))).toBe(false)
		expect(read(dataDir, "secrets/auth-secret")).toBe("auth")
		expect(log.lines).toEqual([
			"info removed an unfinished restore",
		])
	})

	// the crash came after postgres/ was swapped in, so only secrets/ is
	// still staged
	it("finishes a committed restore that stopped halfway through the swap", async ({
		expect,
	}) => {
		const dataDir = staged({ database: "embedded" })
		const log = recordingLog()

		await finishRestore(dataDir, undefined, log)

		expect(read(dataDir, "secrets/auth-secret")).toBe("new")
		expect(read(dataDir, "postgres/PG_VERSION")).toBe("18")
		expect(existsSync(join(dataDir, "search"))).toBe(false)
		expect(existsSync(join(dataDir, "restore"))).toBe(false)
		expect(log.lines).toEqual(["info finished the restore"])
	})

	// the transaction committed, and the process died before the swap
	it("swaps in an external restore the database already holds", async ({
		expect,
	}) => {
		const dataDir = staged({ database: "external", id: "id-1" })
		const bin = fakeBin()

		bin.set("comment", "oxynote-restore:id-1")

		await finishRestore(
			dataDir,
			client(bin.dir, external),
			recordingLog(),
		)

		expect(read(dataDir, "secrets/auth-secret")).toBe("new")
		expect(
			bin.log().some((line) => line.startsWith("psql -X -q")),
		).toBe(false)
	})

	it("loads an external restore, then swaps it in", async ({
		expect,
	}) => {
		const dataDir = staged({ database: "external", id: "id-1" })
		const bin = fakeBin()

		await finishRestore(
			dataDir,
			client(bin.dir, external),
			recordingLog(),
		)

		expect(bin.loaded()).toContain(
			"COMMENT ON SCHEMA public IS 'oxynote-restore:id-1';",
		)
		expect(read(dataDir, "secrets/auth-secret")).toBe("new")
	})

	it("drops an external restore whose load fails", async ({ expect }) => {
		const dataDir = staged({ database: "external", id: "id-1" })
		const bin = fakeBin()

		bin.set("fail-load")

		await expect(
			finishRestore(
				dataDir,
				client(bin.dir, external),
				recordingLog(),
			),
		).rejects.toThrow("ERROR: boom")
		expect(read(dataDir, "secrets/auth-secret")).toBe("auth")
		expect(existsSync(join(dataDir, "restore"))).toBe(false)
	})

	it("drops an external restore whose dump is cut short", async ({
		expect,
	}) => {
		const dataDir = staged({ database: "external", id: "id-1" })
		const bin = fakeBin()
		const dump = gzipSync("CREATE TABLE t ();\n".repeat(100))

		writeFileSync(
			join(dataDir, "restore/database.sql.gz"),
			dump.subarray(0, dump.length / 2),
		)

		await expect(
			finishRestore(
				dataDir,
				client(bin.dir, external),
				recordingLog(),
			),
		).rejects.toThrow("unexpected end of file")
		expect(bin.loaded()).not.toContain("COMMIT;")
		expect(read(dataDir, "secrets/auth-secret")).toBe("auth")
		expect(existsSync(join(dataDir, "restore"))).toBe(false)
	})

	it("drops an external restore whose dump cannot be read", async ({
		expect,
	}) => {
		const dataDir = staged({ database: "external", id: "id-1" })
		const bin = fakeBin()

		rmSync(join(dataDir, "restore/database.sql.gz"))
		mkdirSync(join(dataDir, "restore/database.sql.gz"))

		await expect(
			finishRestore(
				dataDir,
				client(bin.dir, external),
				recordingLog(),
			),
		).rejects.toThrow("EISDIR")
		expect(read(dataDir, "secrets/auth-secret")).toBe("auth")
	})

	it("swaps in an external restore whose commit reply was lost", async ({
		expect,
	}) => {
		const dataDir = staged({ database: "external", id: "id-1" })
		const bin = fakeBin()

		bin.set("lost-reply")

		await finishRestore(
			dataDir,
			client(bin.dir, external),
			recordingLog(),
		)

		expect(read(dataDir, "secrets/auth-secret")).toBe("new")
		expect(existsSync(join(dataDir, "restore"))).toBe(false)
	})

	// a crash before the marker reached the disk leaves it empty
	it("removes a restore whose marker cannot be read", async ({
		expect,
	}) => {
		const dataDir = staged()

		writeFileSync(join(dataDir, "restore/committed"), "")

		await finishRestore(dataDir, undefined, recordingLog())

		expect(read(dataDir, "secrets/auth-secret")).toBe("auth")
	})

	it("refuses to guess about an external restore without the database", async ({
		expect,
	}) => {
		const dataDir = staged({ database: "external", id: "id-1" })

		await expect(
			finishRestore(dataDir, undefined, recordingLog()),
		).rejects.toThrow(
			"a restore into an external database is waiting, but OXYNOTE_DB_DSN is not set",
		)
	})
})

// staged leaves a restore on a fake volume with a new secret staged, and
// committed when marker is given.
function staged(marker?: Commit): string {
	const dataDir = fakeVolume()

	mkdirSync(join(dataDir, "restore/secrets"), { recursive: true })
	writeFileSync(join(dataDir, "restore/secrets/auth-secret"), "new")
	writeFileSync(
		join(dataDir, "restore/database.sql.gz"),
		gzipSync("CREATE TABLE t ();\n"),
	)

	if (marker !== undefined) {
		commit(dataDir, marker)
	}

	return dataDir
}

function read(dataDir: string, path: string): string {
	return readFileSync(join(dataDir, path), "utf8")
}
