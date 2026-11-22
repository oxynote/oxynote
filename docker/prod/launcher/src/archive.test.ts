import { createHash } from "node:crypto"
import { mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { Readable } from "node:stream"
import { pack } from "tar-stream"
import { describe, it } from "vitest"
import {
	readArchive,
	refusal,
	writeArchive,
	type ArchiveReader,
	type Manifest,
} from "./archive.js"
import { collector } from "./test-helpers.js"

const manifest: Manifest = {
	format: 1,
	imageVersion: "0.12.0",
	migrationVersion: 13,
	database: "embedded",
	skipped: [],
	secretsFromEnv: [],
}

describe("writeArchive and readArchive", () => {
	it("carry the manifest and every file across", async ({ expect }) => {
		const source = mkdtempSync(join(tmpdir(), "oxynote-archive-"))
		const target = mkdtempSync(join(tmpdir(), "oxynote-archive-"))
		const long = `object-storage/${"x".repeat(150)}/file`

		writeFileSync(join(source, "a"), "first")
		writeFileSync(join(source, "b"), "second")

		const out = collector()

		await writeArchive(out, manifest, [
			{ name: "secrets/a", path: join(source, "a") },
			{ name: long, path: join(source, "b") },
		])

		const accepted: Manifest[] = []
		const read = await readArchive(Readable.from([out.bytes()]), {
			accept: (m) => accepted.push(m),
			target: (name) => join(target, name),
		})

		expect(read).toEqual(manifest)
		expect(accepted).toEqual([manifest])
		expect(readFileSync(join(target, "secrets/a"), "utf8")).toBe(
			"first",
		)
		expect(readFileSync(join(target, long), "utf8")).toBe("second")
	})

	it("drops an entry the reader has no target for", async ({
		expect,
	}) => {
		const source = mkdtempSync(join(tmpdir(), "oxynote-archive-"))

		writeFileSync(join(source, "a"), "first")

		const out = collector()

		await writeArchive(out, manifest, [
			{ name: "object-storage/a", path: join(source, "a") },
		])

		await expect(
			readArchive(Readable.from([out.bytes()]), {
				accept: () => undefined,
				target: () => undefined,
			}),
		).resolves.toEqual(manifest)
	})

	it("leave out a file deleted after it was listed", async ({
		expect,
	}) => {
		const source = mkdtempSync(join(tmpdir(), "oxynote-archive-"))
		const target = mkdtempSync(join(tmpdir(), "oxynote-archive-"))
		const names: string[] = []

		writeFileSync(join(source, "a"), "first")

		const out = collector()

		await writeArchive(out, manifest, [
			{
				name: "object-storage/gone",
				path: join(source, "gone"),
			},
			{ name: "object-storage/a", path: join(source, "a") },
		])
		await readArchive(Readable.from([out.bytes()]), {
			accept: () => undefined,
			target: (name) => {
				names.push(name)

				return join(target, name)
			},
		})

		expect(names).toEqual(["object-storage/a"])
	})

	it("fail on a file that cannot be read for another reason", async ({
		expect,
	}) => {
		const source = mkdtempSync(join(tmpdir(), "oxynote-archive-"))

		writeFileSync(join(source, "a"), "first")

		await expect(
			writeArchive(collector(), manifest, [
				{
					name: "object-storage/b",
					path: join(source, "a", "b"),
				},
			]),
		).rejects.toThrow("ENOTDIR")
	})
})

describe("readArchive", () => {
	const reader: ArchiveReader = {
		accept: () => undefined,
		target: (name) =>
			join(
				mkdtempSync(join(tmpdir(), "oxynote-archive-")),
				name,
			),
	}
	const manifestText = JSON.stringify(manifest)

	const cases = [
		{
			name: "an empty input",
			input: () => Promise.resolve(Buffer.alloc(1024)),
			expected: "cannot read the backup: it is empty",
		},
		{
			name: "a first entry that is not the manifest",
			input: () => tar([["database.sql.gz", "x"]]),
			expected: "cannot read the backup: it starts with database.sql.gz instead of manifest.json",
		},
		{
			name: "a manifest without a migration version",
			input: () =>
				tar([
					[
						"manifest.json",
						JSON.stringify({
							...manifest,
							migrationVersion: "13",
						}),
					],
				]),
			expected: "cannot read the backup:",
		},
		{
			name: "no checksums",
			input: () =>
				tar([["manifest.json", manifestText]], null),
			expected: "cannot read the backup: it ends before its checksums, so it is incomplete",
		},
		{
			name: "checksums that are no map of hashes",
			input: () =>
				tar([["manifest.json", manifestText]], {
					a: 1,
				}),
			expected: "cannot read the backup:",
		},
		{
			name: "a path outside the target",
			input: () =>
				tar([
					["manifest.json", manifestText],
					["secrets/../../x", "x"],
				]),
			expected: "cannot read the backup: it holds an invalid entry name: secrets/../../x",
		},
		{
			name: "an absolute path",
			input: () =>
				tar([
					["manifest.json", manifestText],
					["/etc/passwd", "x"],
				]),
			expected: "cannot read the backup: it holds an invalid entry name: /etc/passwd",
		},
		{
			name: "an entry its checksum does not match",
			input: () =>
				tar([["manifest.json", manifestText]], {
					"manifest.json": "0",
				}),
			expected: "cannot read the backup: manifest.json does not match its checksum",
		},
		{
			name: "a checksum for an entry that is missing",
			input: () =>
				tar([["manifest.json", manifestText]], {
					"manifest.json": sha256(manifestText),
					"database.sql.gz": "0",
				}),
			expected: "cannot read the backup: database.sql.gz does not match its checksum",
		},
		{
			name: "an entry after the checksums",
			input: async () =>
				Buffer.concat([
					(
						await tar([
							[
								"manifest.json",
								manifestText,
							],
						])
					).subarray(0, -1024),
					await tar(
						[["secrets/extra", "x"]],
						null,
					),
				]),
			expected: "cannot read the backup: it holds secrets/extra after its checksums",
		},
		{
			name: "a truncated archive",
			input: async () =>
				(
					await tar([
						["manifest.json", manifestText],
						["secrets/a", "x".repeat(4096)],
					])
				).subarray(0, 2048),
			expected: "cannot read the backup: Unexpected end of data",
		},
	]

	for (const c of cases) {
		it(`refuses ${c.name}`, async ({ expect }) => {
			await expect(
				readArchive(
					Readable.from([await c.input()]),
					reader,
				),
			).rejects.toThrow(c.expected)
		})
	}

	it("reports what the reader refused", async ({ expect }) => {
		const refusing: ArchiveReader = {
			accept: () => {
				throw new Error("too new")
			},
			target: () => undefined,
		}

		await expect(
			readArchive(
				Readable.from([
					await tar([
						["manifest.json", manifestText],
					]),
				]),
				refusing,
			),
		).rejects.toThrow("cannot read the backup: too new")
	})
})

describe("refusal", () => {
	const cases = {
		"the same release": {
			backup: {},
			image: { version: "0.12.0", migrationVersion: 13 },
			expected: undefined,
		},
		"an older release": {
			backup: {
				imageVersion: "0.11.0",
				migrationVersion: 12,
			},
			image: { version: "0.12.0", migrationVersion: 13 },
			expected: undefined,
		},
		"a newer release": {
			backup: { imageVersion: "0.13.0" },
			image: { version: "0.12.0", migrationVersion: 13 },
			expected: "the backup comes from image 0.13.0, newer than this image's 0.12.0. Restore it with image 0.13.0 or later",
		},
		"a newer patch release": {
			backup: { imageVersion: "0.12.1" },
			image: { version: "0.12.0", migrationVersion: 13 },
			expected: "the backup comes from image 0.12.1, newer than this image's 0.12.0. Restore it with image 0.12.1 or later",
		},
		"an older major with a higher minor": {
			backup: { imageVersion: "0.99.0" },
			image: { version: "1.0.0", migrationVersion: 13 },
			expected: undefined,
		},
		"a newer migration": {
			backup: { migrationVersion: 14 },
			image: { version: "0.12.0", migrationVersion: 13 },
			expected: "the backup is at migration 14, newer than this image's 13. Restore it with image 0.12.0 or later",
		},
		"a newer migration between dev builds": {
			backup: { imageVersion: "dev", migrationVersion: 14 },
			image: { version: "dev", migrationVersion: 13 },
			expected: "the backup is at migration 14, newer than this image's 13. Restore it with image dev or later",
		},
		"a dev backup on a release": {
			backup: { imageVersion: "dev" },
			image: { version: "0.12.0", migrationVersion: 13 },
			expected: undefined,
		},
		"a release backup on a dev image": {
			backup: { imageVersion: "9.0.0" },
			image: { version: "dev", migrationVersion: 13 },
			expected: undefined,
		},
		"an unknown format": {
			backup: { format: 2 },
			image: { version: "0.12.0", migrationVersion: 13 },
			expected: "the backup has format 2, and this image reads only format 1",
		},
		"an image that does not know its migration version": {
			backup: {},
			image: { version: "dev", migrationVersion: undefined },
			expected: "this build does not know its migration version, so it cannot check the backup",
		},
	}

	for (const [name, c] of Object.entries(cases)) {
		it(`answers ${name}`, ({ expect }) => {
			expect(
				refusal({ ...manifest, ...c.backup }, c.image),
			).toBe(c.expected)
		})
	}
})

function sha256(text: string): string {
	return createHash("sha256").update(text).digest("hex")
}

// tar builds an archive by hand, for the shapes writeArchive never writes.
// The checksums match the entries unless sums says otherwise, and null
// leaves them out.
async function tar(
	entries: [string, string][],
	sums?: Record<string, unknown> | null,
): Promise<Buffer> {
	const p = pack()
	const out = collector()
	const done = new Promise((resolve) => out.on("finish", resolve))
	const actual: Record<string, string> = {}

	p.pipe(out)

	for (const [name, body] of entries) {
		actual[name] = sha256(body)
		p.entry({ name }, body)
	}

	if (sums !== null) {
		p.entry(
			{ name: "checksums.json" },
			JSON.stringify(sums ?? actual),
		)
	}

	p.finalize()
	await done

	return out.bytes()
}
