import { describe, it } from "vitest"

import { createLogger, type LogLevel } from "./logging.js"

// a destination that keeps what pino wrote, parsed back into records.
function capture() {
	const lines: string[] = []

	return {
		destination: {
			write(chunk: string) {
				lines.push(chunk)
			},
		},
		records: () =>
			lines.map(
				(line) =>
					JSON.parse(line) as Record<
						string,
						unknown
					>,
			),
	}
}

const thresholdCases: {
	name: string
	level: LogLevel
	expected: LogLevel[]
}[] = [
	{
		name: "DEBUG",
		level: "DEBUG",
		expected: ["DEBUG", "INFO", "WARN", "ERROR"],
	},
	{ name: "INFO", level: "INFO", expected: ["INFO", "WARN", "ERROR"] },
	{ name: "WARN", level: "WARN", expected: ["WARN", "ERROR"] },
	{ name: "ERROR", level: "ERROR", expected: ["ERROR"] },
]

describe("createLogger", () => {
	it.for(thresholdCases)(
		"at $name emits that level and every level above it",
		({ level, expected }, { expect }) => {
			const written = capture()
			const log = createLogger(level, written.destination)

			log.debug("d")
			log.info("i")
			log.warn("w")
			log.error("e")

			expect(
				written.records().map((record) => record.level),
			).toEqual(expected)
		},
	)

	it("writes core's log shape: an ISO time, an upper-case level, a msg", ({
		expect,
	}) => {
		const written = capture()

		createLogger("INFO", written.destination).warn(
			"cannot reach core",
		)

		expect(written.records()).toEqual([
			{
				time: expect.stringMatching(
					/^\d{4}-\d{2}-\d{2}T[\d:.]+Z$/,
				) as unknown,
				level: "WARN",
				msg: "cannot reach core",
			},
		])
	})
})
