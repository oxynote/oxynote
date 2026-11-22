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
import { extract, pack } from "tar-stream"
import { z } from "zod"
import { isNoEntry } from "./secrets.js"

// a backup is one plain tar. The manifest comes first, so a restore can
// refuse before it writes anything. The checksums come last, because a
// stream knows them only once every entry is written.
export const archiveFormat = 1
export const databaseEntry = "database.sql.gz"
const manifestEntry = "manifest.json"
const checksumsEntry = "checksums.json"

// the directories of the data volume a backup holds, which are also the
// prefixes of their entries.
export const secretsDir = "secrets"
export const objectsDir = "object-storage"

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
	const written = pipeline(tar, out)
	const sums: Record<string, string> = {}
	const manifestText = JSON.stringify(manifest)

	sums[manifestEntry] = createHash("sha256")
		.update(manifestText)
		.digest("hex")
	tar.entry({ name: manifestEntry, mode: 0o600 }, manifestText)

	for (const file of files) {
		// the instance keeps running, so a listed file may be gone by now,
		// and it is left out. An open file keeps its content and size even
		// when another is renamed over it.
		let fd: number

		try {
			fd = openSync(file.path, "r")
		} catch (err) {
			if (isNoEntry(err)) {
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
	await written
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
				JSON.parse(await text(entry, hash)),
			)
			reader.accept(manifest)
		} else if (name === checksumsEntry) {
			expected = checksumsSchema.parse(
				JSON.parse(await text(entry)),
			)

			continue
		} else {
			const target = reader.target(name)

			if (target === undefined) {
				// a dropped entry still counts towards the checksums.
				for await (const chunk of entry) {
					hash.update(chunk as Buffer)
				}
			} else {
				mkdirSync(dirname(target), {
					recursive: true,
					mode: 0o700,
				})
				await pipeline(
					entry,
					hashing(hash),
					createWriteStream(target, {
						mode: 0o600,
					}),
				)
			}
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

export interface ImageVersion {
	version: string
	migrationVersion: number | undefined
}

const releasePattern = /^(\d+)\.(\d+)\.(\d+)$/

// compareReleases orders two plain semver releases, or returns undefined
// when either is a dev build or a prerelease.
function compareReleases(a: string, b: string): number | undefined {
	const left = releasePattern.exec(a)
	const right = releasePattern.exec(b)

	if (left === null || right === null) {
		return undefined
	}

	for (let i = 1; i <= 3; i++) {
		const diff = Number(left[i]) - Number(right[i])

		if (diff !== 0) {
			return diff
		}
	}

	return 0
}

// refusal says why this image cannot restore a backup, or returns undefined
// when it can. An older backup is fine, because core migrates it at the next
// boot. A newer one holds a schema this image does not know.
export function refusal(
	manifest: Manifest,
	image: ImageVersion,
): string | undefined {
	if (manifest.format !== archiveFormat) {
		return `the backup has format ${String(manifest.format)}, and this image reads only format ${String(archiveFormat)}`
	}

	if (image.migrationVersion === undefined) {
		return "this build does not know its migration version, so it cannot check the backup"
	}

	if (manifest.migrationVersion > image.migrationVersion) {
		return `the backup is at migration ${String(manifest.migrationVersion)}, newer than this image's ${String(image.migrationVersion)}. Restore it with image ${manifest.imageVersion} or later`
	}

	const order = compareReleases(manifest.imageVersion, image.version)

	if (order !== undefined && order > 0) {
		return `the backup comes from image ${manifest.imageVersion}, newer than this image's ${image.version}. Restore it with image ${manifest.imageVersion} or later`
	}

	return undefined
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

async function text(
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
