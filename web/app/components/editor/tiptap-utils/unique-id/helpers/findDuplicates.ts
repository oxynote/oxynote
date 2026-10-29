/**
 * Returns a list of duplicated items within an array.
 */
export function findDuplicates(items: any[]): any[] {
	const seen = new Set<unknown>()
	const duplicates = new Set<unknown>()

	items.forEach((item) => {
		if (seen.has(item)) {
			duplicates.add(item)
		}

		seen.add(item)
	})

	return [...duplicates]
}
