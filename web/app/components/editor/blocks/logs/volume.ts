// how many bars the volume strip draws across the block's time range
export const VOLUME_BUCKETS = 48

export interface VolumeBucket {
	start: number // unix milliseconds
	counts: Record<LogLevel, number>
}

// volumeBuckets spreads the line counts of every query over equal buckets
// across the window. The source counts on its own step, so each of its
// buckets lands in the bar its start falls in. A line without a known
// level counts as unknown.
export function volumeBuckets(
	results: (GenericQueryResult | null | undefined)[],
	window: { from: Date; to: Date },
	count: number = VOLUME_BUCKETS,
): VolumeBucket[] {
	const from = window.from.getTime()
	const span = Math.max(window.to.getTime() - from, 1)
	const levels = new Set<string>(Object.values(LogLevel))
	const buckets: VolumeBucket[] = Array.from({ length: count }, (_, i) => ({
		start: from + (span * i) / count,
		counts: emptyCounts(),
	}))

	for (const result of results) {
		for (const series of result?.data ?? []) {
			const label = series.labels.level ?? ""
			const level = levels.has(label) ? (label as LogLevel) : LogLevel.Unknown

			for (const [seconds, value] of series.metrics) {
				const index = Math.floor(((seconds * 1000 - from) / span) * count)
				const bucket = buckets[index]

				if (bucket) {
					bucket.counts[level] += value
				}
			}
		}
	}

	return buckets
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
