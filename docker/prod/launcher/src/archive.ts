import { createHash, type Hash } from "node:crypto"
import {
	createReadStream,
	createWriteStream,
	fstatSync,
	mkdirSync,
	openSync,
} from "node:fs"
import { dirname } from "node:path"
import type { Writable, Readable } from "node:stream"
import { Transform } from "node:stream"
import { pipeline } from "node:stream/promises"
import { extract, pack, type Pack } from "tar-stream"
import { z } from "zod"
import { isNoEntry } from "./fs-errors.js"
import { objectsDir } from "./mapping.js"

// a backup is one plain tar. The manifest comes first, so a restore can
// refuse before it writes anything. The checksums come last, because a
// stream knows them only once every entry is written.
export const archiveFormat = 1
export const databaseEntry = "database.sql.gz"
const manifestEntry = "manifest.json"
const checksumsEntry = "checksums.json"

export const remoteObjectsNote =
	"skipping object-storage/: the objects are in a remote store"

const manifestSchema = z.object({
	format: z.number().int(),
	imageVersion: z.string(),
	migrationVersion: z.number().int(),
	database: z.enum(["embedded", "external"]),
	// the parts a backup left out, such as objects kept in a remote store.
	skipped: z.array(z.literal(objectsDir)),
	// the secrets the instance took from its environment, which are on no
	// volume.
	secretsFromEnv: z.array(z.string()),
})

export type Manifest = z.infer<typeof manifestSchema>

const checksumsSchema = z.record(z.string(), z.string())

export interface ArchiveFile {
	name: string
	path: string
}

// writeArchive writes the manifest, the files and their checksums to out.
export async function writeArchive(
	out: Writable,
	manifest: Manifest,
	files: ArchiveFile[],
): Promise<void> {
	const tar = pack()
	// settled at once, because the output can fail while an entry is
	// still being written, and then that entry fails too.
	const written = Promise.allSettled([pipeline(tar, out)])

	try {
		await writeEntries(tar, manifest, files)
	} catch (err) {
		tar.destroy()
		await written
		throw err
	}

	const [result] = await written

	if (result.status === "rejected") {
		throw result.reason
	}
}

async function writeEntries(
	tar: Pack,
	manifest: Manifest,
	files: ArchiveFile[],
): Promise<void> {
	const sums: Record<string, string> = {}
	const manifestText = JSON.stringify(manifest)

	sums[manifestEntry] = createHash("sha256")
		.update(manifestText)
		.digest("hex")
	tar.entry({ name: manifestEntry, mode: 0o600 }, manifestText)

	for (const file of files) {
		// the instance keeps running, so a listed file may be gone by
		// now, and it is left out. The dump is the backup's own, so it
		// must be there. An open file keeps its content and size even
		// when another is renamed over it.
		let fd: number

		try {
			fd = openSync(file.path, "r")
		} catch (err) {
			if (isNoEntry(err) && file.name !== databaseEntry) {
				continue
			}

			throw err
		}

		const hash = createHash("sha256")
		const entry = tar.entry({
			name: file.name,
			size: fstatSync(fd).size,
			mode: 0o600,
		})

		await pipeline(
			createReadStream(file.path, { fd }),
			hashing(hash),
			entry,
		)
		sums[file.name] = hash.digest("hex")
	}

	tar.entry({ name: checksumsEntry, mode: 0o600 }, JSON.stringify(sums))
	tar.finalize()
}

export interface ArchiveReader {
	// sees the manifest before any entry is written, and throws to refuse
	// the backup.
	accept(manifest: Manifest): void
	// names the file an entry is written to, or undefined to drop it.
	target(name: string): string | undefined
}

// readArchive writes every entry where reader says and checks it against
// its checksum. It throws when the archive is damaged in any way.
export async function readArchive(
	input: Readable,
	reader: ArchiveReader,
): Promise<Manifest> {
	try {
		return await read(input, reader)
	} catch (err) {
		throw new Error(
			`cannot read the backup: ${err instanceof Error ? err.message : String(err)}`,
			{ cause: err },
		)
	}
}

async function read(input: Readable, reader: ArchiveReader): Promise<Manifest> {
	const tar = extract()
	const feeding = pipeline(input, tar)
	const actual: Record<string, string> = {}
	let manifest: Manifest | undefined
	let expected: Record<string, string> | undefined

	// an error in the loop below breaks the feeding pipeline too, and only
	// the loop's error is worth reporting.
	feeding.catch(() => undefined)

	for await (const entry of tar) {
		const name = checkedName(entry.header.name)
		const hash = createHash("sha256")

		if (expected !== undefined) {
			throw new Error(`it holds ${name} after its checksums`)
		}

		if (manifest === undefined) {
			if (name !== manifestEntry) {
				throw new Error(
					`it starts with ${name} instead of ${manifestEntry}`,
				)
			}

			manifest = manifestSchema.parse(
				JSON.parse(await readText(entry, hash)),
			)
			reader.accept(manifest)
		} else if (name === checksumsEntry) {
			expected = checksumsSchema.parse(
				JSON.parse(await readText(entry)),
			)

			continue
		} else {
			await writeEntry(entry, reader.target(name), hash)
		}

		actual[name] = hash.digest("hex")
	}

	await feeding

	if (manifest === undefined) {
		throw new Error("it is empty")
	}

	if (expected === undefined) {
		throw new Error(
			"it ends before its checksums, so it is incomplete",
		)
	}

	for (const name of new Set([
		...Object.keys(expected),
		...Object.keys(actual),
	])) {
		if (expected[name] !== actual[name]) {
			throw new Error(`${name} does not match its checksum`)
		}
	}

	return manifest
}

// writeEntry writes an entry to target and hashes it. A dropped entry, with
// no target, is only hashed, because it still counts towards the checksums.
async function writeEntry(
	entry: AsyncIterable<unknown>,
	target: string | undefined,
	hash: Hash,
): Promise<void> {
	if (target === undefined) {
		// a tar entry yields buffers.
		for await (const chunk of entry) {
			hash.update(chunk as Buffer)
		}

		return
	}

	mkdirSync(dirname(target), { recursive: true, mode: 0o700 })
	await pipeline(
		entry,
		hashing(hash),
		createWriteStream(target, { mode: 0o600 }),
	)
}

// checkedName returns an entry's name once it is sure the name stays inside
// the directory it is written to.
function checkedName(name: string): string {
	if (
		name.startsWith("/") ||
		name
			.split("/")
			.some(
				(part) =>
					part === "" ||
					part === "." ||
					part === "..",
			)
	) {
		throw new Error(`it holds an invalid entry name: ${name}`)
	}

	return name
}

// hashing passes every chunk through unchanged and adds it to hash.
function hashing(hash: Hash): Transform {
	return new Transform({
		transform(chunk: Buffer, _encoding, done) {
			hash.update(chunk)
			done(null, chunk)
		},
	})
}

async function readText(
	entry: AsyncIterable<unknown>,
	hash?: Hash,
): Promise<string> {
	const chunks: Buffer[] = []

	// a tar entry yields buffers.
	for await (const chunk of entry) {
		chunks.push(chunk as Buffer)
	}

	const buffer = Buffer.concat(chunks)

	hash?.update(buffer)

	return buffer.toString("utf8")
}
