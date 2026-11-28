const RELEASE_VERSION_PATTERN = /^\d+\.\d+\.\d+$/

// numeric collation compares each run of digits as a number, so 0.10.0
// sorts after 0.9.3
const RELEASE_VERSION_COLLATOR = new Intl.Collator("en", { numeric: true })

// a release is a plain x.y.z
export function isReleaseVersion(version: string): boolean {
	return RELEASE_VERSION_PATTERN.test(version)
}

export function isNewerRelease(candidate: string, current: string): boolean {
	if (!isReleaseVersion(candidate) || !isReleaseVersion(current)) {
		return false
	}

	return RELEASE_VERSION_COLLATOR.compare(candidate, current) > 0
}
