import { describe, it } from "vitest"
import type { Manifest } from "./archive.js"
import { refusal } from "./version.js"

const manifest: Manifest = {
	format: 1,
	imageVersion: "0.12.0",
	migrationVersion: 13,
	database: "embedded",
	skipped: [],
	secretsFromEnv: [],
}

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
