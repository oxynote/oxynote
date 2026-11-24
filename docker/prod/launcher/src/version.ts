import { archiveFormat, type Manifest } from "./archive.js"

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
