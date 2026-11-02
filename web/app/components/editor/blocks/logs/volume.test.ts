import { describe, it } from "vitest"
import { VOLUME_BUCKETS, volumeBuckets } from "./volume"

const FROM_SECONDS = Date.UTC(2026, 0, 1, 12, 0, 0) / 1000

// four minutes, one bucket per minute when split into four
const WINDOW = {
	from: new Date(FROM_SECONDS * 1000),
	to: new Date((FROM_SECONDS + 240) * 1000),
}

describe("volumeBuckets", () => {
	it("splits the window into the default number of empty buckets", ({
		expect,
	}) => {
		const buckets = volumeBuckets([], WINDOW)

		expect(buckets).toHaveLength(VOLUME_BUCKETS)
		expect(buckets[0]).toEqual(bucket(0))
		expect(buckets[1]).toEqual(bucket(5))
		expect(buckets.at(-1)).toEqual(bucket(235))
	})

	it.for([
		{
			name: "counts each level apart and an unrecognised one as unknown",
			input: () => [
				result([
					series("critical", [[0, 1]]),
					series("error", [[0, 2]]),
					series("warning", [[0, 4]]),
					series("info", [[0, 8]]),
					series("debug", [[0, 16]]),
					series("trace", [[0, 32]]),
					series(undefined, [[0, 64]]),
					series("verbose", [[0, 128]]),
				]),
			],
			expected: [
				bucket(0, {
					critical: 1,
					error: 2,
					warning: 4,
					info: 8,
					debug: 16,
					trace: 32,
					unknown: 192,
				}),
				bucket(60),
				bucket(120),
				bucket(180),
			],
		},
		{
			name: "puts each point in the bucket its time falls in",
			input: () => [
				result([
					series("info", [
						[59, 1],
						[60, 2],
						[239, 4],
					]),
				]),
			],
			expected: [
				bucket(0, { info: 1 }),
				bucket(60, { info: 2 }),
				bucket(120),
				bucket(180, { info: 4 }),
			],
		},
		{
			name: "ignores points outside the window",
			input: () => [
				result([
					series("error", [
						[-1, 1],
						[240, 2],
					]),
				]),
			],
			expected: [bucket(0), bucket(60), bucket(120), bucket(180)],
		},
		{
			name: "adds up the counts of every query and skips missing results",
			input: () => [
				result([series("info", [[0, 1]])]),
				null,
				undefined,
				{ status: GenericQueryResultStatus.NoData } as GenericQueryResult,
				result([series("info", [[0, 2]])]),
			],
			expected: [bucket(0, { info: 3 }), bucket(60), bucket(120), bucket(180)],
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(volumeBuckets(input(), WINDOW, 4)).toEqual(expected)
	})

	it("counts into the first bucket when the window has no length", ({
		expect,
	}) => {
		const buckets = volumeBuckets(
			[result([series("info", [[0, 1]])])],
			{ from: WINDOW.from, to: WINDOW.from },
			2,
		)

		expect(buckets).toEqual([
			bucket(0, { info: 1 }),
			{ ...bucket(0), start: WINDOW.from.getTime() + 0.5 },
		])
	})
})

function result(data: GenericQueryResult["data"]): GenericQueryResult {
	return { status: GenericQueryResultStatus.Ok, data }
}

// a series of points given in seconds from the window's start
function series(
	level: string | undefined,
	points: [number, number][],
): GenericQueryResult["data"][number] {
	return {
		labels: level ? { level } : {},
		metrics: points.map(([seconds, value]) => [FROM_SECONDS + seconds, value]),
	}
}

function bucket(
	offsetSeconds: number,
	counts: Partial<Record<LogLevel, number>> = {},
) {
	return {
		start: (FROM_SECONDS + offsetSeconds) * 1000,
		counts: {
			critical: 0,
			error: 0,
			warning: 0,
			info: 0,
			debug: 0,
			trace: 0,
			unknown: 0,
			...counts,
		},
	}
}
