// shared helpers for the log block component suites. Test-only: the
// app/**/test-helpers.ts coverage exclude keeps this out of the
// denominator, and nothing here is imported by app code.
import { flushPromises } from "@vue/test-utils"

// a log request runs through the test-time h3 app, which answers a few
// macrotasks later, and the view updates after that
export async function settleLogRequests() {
	for (let round = 0; round < 5; round++) {
		await flushPromises()
	}
}

// a plain line on the first of January 2026, which is its own message
export function logLine(id: string, clock: string): LogLine {
	return {
		id,
		time: `2026-01-01T${clock}`,
		level: LogLevel.Info,
		line: id,
		labels: { env: "prod" },
		fields: {},
	}
}

export function logLinesResult(
	lines: LogLine[],
	cursor: string | null = null,
): LogLinesResult {
	return {
		status: lines.length
			? GenericQueryResultStatus.Ok
			: GenericQueryResultStatus.NoData,
		lines,
		cursor,
		partial: false,
	}
}
