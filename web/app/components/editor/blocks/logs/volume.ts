export interface VolumeBucket {
	start: number // unix milliseconds
	counts: Record<LogLevel, number>
}

// volumeBuckets adds up the line counts of every query, one bucket for each
// time the source counted at. The source sends every bucket of the window,
// empty ones as zero. So the buckets are a step apart, and each bar the
// strip draws covers the same time. A line without a known level counts as
// unknown.
export function volumeBuckets(
	results: (GenericQueryResult | null | undefined)[],
): VolumeBucket[] {
	const levels = new Set<string>(Object.values(LogLevel))
	const buckets = new Map<number, VolumeBucket>()

	for (const result of results) {
		for (const series of result?.data ?? []) {
			const label = series.labels.level ?? ""
			const level = levels.has(label) ? (label as LogLevel) : LogLevel.Unknown

			for (const [seconds, value] of series.metrics) {
				const start = seconds * 1000
				const bucket = buckets.get(start) ?? {
					start: start,
					counts: emptyCounts(),
				}

				bucket.counts[level] += value
				buckets.set(start, bucket)
			}
		}
	}

	return [...buckets.values()].sort((a, b) => a.start - b.start)
}

function emptyCounts(): Record<LogLevel, number> {
	return {
		[LogLevel.Critical]: 0,
		[LogLevel.Error]: 0,
		[LogLevel.Warning]: 0,
		[LogLevel.Info]: 0,
		[LogLevel.Debug]: 0,
		[LogLevel.Trace]: 0,
		[LogLevel.Unknown]: 0,
	}
}
