import {
	closeSync,
	existsSync,
	fsyncSync,
	openSync,
	readdirSync,
	readFileSync,
	renameSync,
	rmSync,
	writeSync,
} from "node:fs"
import { join } from "node:path"
import { databaseEntry } from "./archive.js"
import { dumpScript } from "./dump.js"
import type { Logger } from "./logging.js"
import { objectsDir, postgresDir, searchDir, secretsDir } from "./mapping.js"
import type { Client } from "./psql.js"

// a restore builds the new instance in this directory of the data volume,
// so moving it into place is a rename. Its committed file is the commit
// point: before it exists the staged instance is discarded, after it the
// restore is finished by the next restore or boot.
export function stagingDir(dataDir: string): string {
	return join(dataDir, "restore")
}

// the parts of the data volume a restore replaces.
const parts = [postgresDir, secretsDir, objectsDir]

// an embedded restore commits with its cluster built. An external one
// commits with only its dump staged, and the dump is loaded when the
// restore is finished, because the database is in use until then.
export type Commit =
	{ database: "embedded" } | { database: "external"; id: string }

// commit records the commit point. The staged instance reaches the disk
// first and the marker after it, so a power loss never leaves a marker
// over a staged instance that is only half written.
export function commit(dataDir: string, value: Commit): void {
	syncTree(stagingDir(dataDir))

	const fd = openSync(markerPath(dataDir), "w", 0o600)

	try {
		writeSync(fd, JSON.stringify(value))
		fsyncSync(fd)
	} finally {
		closeSync(fd)
	}

	syncPath(stagingDir(dataDir))
}

export function committed(dataDir: string): boolean {
	return readMarker(dataDir) !== undefined
}

// finishRestore completes a committed restore and removes one that never
// committed. It runs at every boot before the secrets are read, and at the
// end of a restore of a stopped instance. external is the database when it
// is not the embedded one.
export async function finishRestore(
	dataDir: string,
	external: Client | undefined,
	log: Logger,
): Promise<void> {
	const staging = stagingDir(dataDir)

	if (!existsSync(staging)) {
		return
	}

	const marker = readMarker(dataDir)

	if (marker === undefined) {
		rmSync(staging, { recursive: true, force: true })
		log.info("removed an unfinished restore")

		return
	}

	if (marker.database === "external") {
		if (external === undefined) {
			throw new Error(
				"a restore into an external database is waiting, but OXYNOTE_DB_DSN is not set",
			)
		}

		await loadExternal(dataDir, external, marker.id, log)
	}

	swap(dataDir)
	log.info("finished the restore")
}

// a restore into an external database leaves its id in this comment, in the
// transaction that loads it.
const commentQuery =
	"SELECT coalesce(obj_description('public'::regnamespace, 'pg_namespace'), '')"

// loadExternal replaces the database's schemas with the staged dump in one
// transaction, unless an earlier run already did. A load that failed before
// its commit leaves the database as it was, so the staged restore is dropped
// and the old instance stays.
async function loadExternal(
	dataDir: string,
	external: Client,
	id: string,
	log: Logger,
): Promise<void> {
	if ((await external.query(commentQuery)) === restoreComment(id)) {
		return
	}

	const version = Number(await external.query("SHOW server_version_num"))

	log.info("loading the database")

	try {
		// core keeps its tables in public and its job queue in dbos,
		// and the dump creates both again. It writes no CREATE SCHEMA
		// public, so that schema is created here. The comment marks the
		// transaction as committed for a later run.
		await external.load(
			dumpScript(join(stagingDir(dataDir), databaseEntry), {
				before: "DROP SCHEMA IF EXISTS public CASCADE;\nDROP SCHEMA IF EXISTS dbos CASCADE;\nCREATE SCHEMA public;\n",
				after: `\nCOMMENT ON SCHEMA public IS '${restoreComment(id)}';\n`,
				dropTransactionTimeout: version < 170000,
			}),
		)
	} catch (err) {
		// the reply to the commit can be lost, so the comment tells
		// whether the load committed. When the database cannot answer,
		// the restore stays staged.
		if (
			(await external.query(commentQuery)) ===
			restoreComment(id)
		) {
			return
		}

		rmSync(stagingDir(dataDir), { recursive: true, force: true })
		throw err
	}
}

function restoreComment(id: string): string {
	return `oxynote-restore:${id}`
}

// swap moves every staged part into place. Each step can run again after a
// crash, because a part already moved is no longer staged.
function swap(dataDir: string): void {
	const staging = stagingDir(dataDir)

	for (const part of parts) {
		const staged = join(staging, part)

		if (existsSync(staged)) {
			rmSync(join(dataDir, part), {
				recursive: true,
				force: true,
			})
			renameSync(staged, join(dataDir, part))
		}
	}

	// the renames reach the disk before the marker that repeats them is
	// removed.
	syncPath(dataDir)
	syncPath(staging)

	// the index is a cache of the database, and an old one would answer
	// every search with nothing.
	rmSync(join(dataDir, searchDir), { recursive: true, force: true })
	rmSync(staging, { recursive: true, force: true })
}

// syncTree flushes dir and every file and directory below it to the disk.
function syncTree(dir: string): void {
	for (const entry of readdirSync(dir, {
		recursive: true,
		withFileTypes: true,
	})) {
		if (entry.isFile() || entry.isDirectory()) {
			syncPath(join(entry.parentPath, entry.name))
		}
	}

	syncPath(dir)
}

function syncPath(path: string): void {
	const fd = openSync(path, "r")

	try {
		fsyncSync(fd)
	} finally {
		closeSync(fd)
	}
}

function markerPath(dataDir: string): string {
	return join(stagingDir(dataDir), "committed")
}

// a marker cut off before its flush reads as none.
function readMarker(dataDir: string): Commit | undefined {
	try {
		return JSON.parse(
			readFileSync(markerPath(dataDir), "utf8"),
		) as Commit
	} catch {
		return undefined
	}
}
