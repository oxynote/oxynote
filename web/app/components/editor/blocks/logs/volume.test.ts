import { describe, it } from "vitest"
import { volumeBuckets } from "./volume"

const FROM_SECONDS = Date.UTC(2026, 0, 1, 12, 0, 0) / 1000

describe("volumeBuckets", () => {
	it.for([
		{
			name: "makes no bucket from missing results or an answer without buckets",
			input: () => [
				null,
				undefined,
				{ status: GenericQueryResultStatus.NoData } as GenericQueryResult,
			],
			expected: [],
		},
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
			],
		},
		{
			name: "makes one bucket for each time counted at, in time order, empty ones too",
			input: () => [
				result([
					series("info", [
						[120, 4],
						[0, 1],
						[60, 0],
					]),
					series("error", [[180, 2]]),
				]),
			],
			expected: [
				bucket(0, { info: 1 }),
				bucket(60),
				bucket(120, { info: 4 }),
				bucket(180, { error: 2 }),
			],
		},
		{
			name: "makes an empty bucket for each bucket of an answer without lines",
			input: () => [
				{
					status: GenericQueryResultStatus.NoData,
					data: [
						series(undefined, [
							[0, 0],
							[60, 0],
						]),
					],
				},
			],
			expected: [bucket(0), bucket(60)],
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
			expected: [bucket(0, { info: 3 })],
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(volumeBuckets(input())).toEqual(expected)
	})
})

function result(data: GenericQueryResult["data"]): GenericQueryResult {
	return { status: GenericQueryResultStatus.Ok, data }
}

// a series of points given in seconds from FROM_SECONDS
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
