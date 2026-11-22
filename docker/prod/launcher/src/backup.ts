import {
	createWriteStream,
	existsSync,
	mkdtempSync,
	readdirSync,
	rmSync,
} from "node:fs"
import { join, relative } from "node:path"
import type { Writable } from "node:stream"
import { pipeline } from "node:stream/promises"
import { createGzip } from "node:zlib"
import {
	archiveFormat,
	databaseEntry,
	remoteObjectsNote,
	writeArchive,
	type ArchiveFile,
} from "./archive.js"
import type { Logger } from "./logging.js"
import { objectsDir, secretsDir } from "./mapping.js"
import { client, type Connection } from "./psql.js"

// a backup keeps its dump in a directory of its own on the data volume while
// it runs, because a tar entry needs its size up front and a pod's /tmp is
// often small.
const partialPrefix = "backup.partial-"

export interface BackupOptions {
	dataDir: string
	binDir: string
	connection: Connection
	external: boolean
	remoteObjects: boolean
	secretsFromEnv: string[]
	imageVersion: string
	out: Writable
	outIsTerminal: boolean
	log: Logger
}

// backup writes the whole instance to out as one tar while it keeps
// running. The database is dumped before the objects are read, so the
// archive can hold an object no row points at. It misses the object of a
// file deleted in between, such as one of a document deleted during the
// backup, so that document comes back without the file.
export async function backup(options: BackupOptions): Promise<void> {
	if (options.outIsTerminal) {
		throw new Error(
			"backup writes a tar to standard output. Redirect it to a file: backup > oxynote-backup.tar",
		)
	}

	const db = client(options.binDir, options.connection)
	const partial = mkdtempSync(join(options.dataDir, partialPrefix))
	const dumpPath = join(partial, databaseEntry)

	try {
		options.log.info("dumping the database")

		const gzip = createGzip()

		await Promise.all([
			db.dump(gzip).catch((err: unknown) => {
				// the dump never completes, so neither may its
				// file.
				gzip.destroy()
				throw err
			}),
			pipeline(
				gzip,
				createWriteStream(dumpPath, { mode: 0o600 }),
			),
		])

		const migrationVersion = Number(
			await db.query(
				"SELECT max(version_id) FROM goose_db_version WHERE is_applied",
			),
		)

		if (options.remoteObjects) {
			options.log.info(remoteObjectsNote)
		}

		await writeArchive(
			options.out,
			{
				format: archiveFormat,
				imageVersion: options.imageVersion,
				migrationVersion,
				database: options.external
					? "external"
					: "embedded",
				skipped: options.remoteObjects
					? [objectsDir]
					: [],
				secretsFromEnv: options.secretsFromEnv,
			},
			[
				{ name: databaseEntry, path: dumpPath },
				...filesUnder(options.dataDir, secretsDir),
				...(options.remoteObjects
					? []
					: filesUnder(
							options.dataDir,
							objectsDir,
						)),
			],
		)
		options.log.info("backup complete")
	} finally {
		rmSync(partial, { recursive: true, force: true })
	}
}

// removePartialBackups deletes what killed backups left behind. It runs at
// boot, when no backup can be running.
export function removePartialBackups(dataDir: string): void {
	for (const name of readdirSync(dataDir)) {
		if (name.startsWith(partialPrefix)) {
			rmSync(join(dataDir, name), {
				recursive: true,
				force: true,
			})
		}
	}
}

// filesUnder lists every file below dataDir/dir in a stable order, named as
// its archive entry. It skips the temp files core writes an object to before
// it renames it into place.
function filesUnder(dataDir: string, dir: string): ArchiveFile[] {
	const root = join(dataDir, dir)

	if (!existsSync(root)) {
		return []
	}

	return readdirSync(root, { recursive: true, withFileTypes: true })
		.filter(
			(entry) =>
				entry.isFile() &&
				!entry.name.startsWith(".tmp-"),
		)
		.map((entry) => join(entry.parentPath, entry.name))
		.toSorted()
		.map((path) => ({
			name: `${dir}/${relative(root, path)}`,
			path,
		}))
}
