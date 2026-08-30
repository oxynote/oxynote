import { describe, it, vi } from "vitest"
import { childRecord, createLineSplitter, createLogger } from "./logging.js"

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

describe("createLogger", () => {
	it("stamps every line with the service it was given", ({ expect }) => {
		const written = capture()

		createLogger("launcher", written.destination).info(
			"starting core",
		)

		expect(written.records()).toEqual([
			{
				time: expect.stringMatching(
					/^\d{4}-\d{2}-\d{2}T[\d:.]+Z$/,
				) as unknown,
				level: "INFO",
				service: "launcher",
				msg: "starting core",
			},
		])
	})

	it("carries the level of each message it is given", ({ expect }) => {
		const written = capture()
		const log = createLogger("launcher", written.destination)

		log.info("up")
		log.warn("exceeded its stop grace")
		log.error("exited unexpectedly")

		expect(written.records().map((record) => record.level)).toEqual(
			["INFO", "WARN", "ERROR"],
		)
	})
})

describe("childRecord", () => {
	it("passes a child's own JSON through, adding only the service", ({
		expect,
	}) => {
		const line = JSON.stringify({
			time: "2026-08-30T18:40:16.987Z",
			level: "WARN",
			msg: "github app integration is disabled",
			version: "0.0.0",
		})

		expect(JSON.parse(childRecord("core", line))).toEqual({
			time: "2026-08-30T18:40:16.987Z",
			level: "WARN",
			msg: "github app integration is disabled",
			version: "0.0.0",
			service: "core",
		})
	})

	it("keeps the child's own level and time rather than restamping them", ({
		expect,
	}) => {
		const line = JSON.stringify({
			level: "warn",
			ts: 1788112770.5264275,
			logger: "admin",
			msg: "admin endpoint disabled",
		})

		expect(JSON.parse(childRecord("caddy", line))).toEqual({
			level: "warn",
			ts: 1788112770.5264275,
			logger: "admin",
			msg: "admin endpoint disabled",
			service: "caddy",
		})
	})

	it("names the child itself when its line claims another service", ({
		expect,
	}) => {
		const line = JSON.stringify({ service: "elsewhere", msg: "hi" })

		expect(JSON.parse(childRecord("web", line))).toEqual({
			service: "web",
			msg: "hi",
		})
	})

	it.for([
		{ name: "plain text", input: "listening on the socket" },
		{ name: "a broken object", input: '{"msg":"unterminated' },
		{ name: "an array", input: '["one","two"]' },
		{ name: "a bare number", input: "42" },
	])("carries $name as the message", ({ input }, { expect }) => {
		expect(JSON.parse(childRecord("web", input))).toEqual({
			msg: input,
			service: "web",
		})
	})
})

describe("createLineSplitter", () => {
	it("emits each complete line in a chunk", ({ expect }) => {
		const emit = vi.fn()
		const splitter = createLineSplitter(emit)

		splitter.data("first\nsecond\n")

		expect(emit.mock.calls).toEqual([["first"], ["second"]])
	})

	it("holds a trailing fragment until its newline arrives", ({
		expect,
	}) => {
		const emit = vi.fn()
		const splitter = createLineSplitter(emit)

		splitter.data("par")
		expect(emit).toHaveBeenCalledTimes(0)

		splitter.data("tial\nrest")

		expect(emit.mock.calls).toEqual([["partial"]])
	})

	it("drops the lines mute matches and keeps the rest", ({ expect }) => {
		const emit = vi.fn()
		const splitter = createLineSplitter(emit, /^Listening on /)

		splitter.data("Listening on http://127.0.0.1:3000\nserving\n")
		splitter.data("Listening on http://127.0.0.1:3000")
		splitter.end()

		expect(emit.mock.calls).toEqual([["serving"]])
	})

	it("flushes the held fragment at stream end", ({ expect }) => {
		const emit = vi.fn()
		const splitter = createLineSplitter(emit)

		splitter.data("no newline")
		splitter.end()

		expect(emit.mock.calls).toEqual([["no newline"]])
	})

	it("ends quietly when nothing is buffered", ({ expect }) => {
		const emit = vi.fn()
		const splitter = createLineSplitter(emit)

		splitter.data("done\n")
		splitter.end()

		expect(emit).toHaveBeenCalledTimes(1)
	})

	it("reads buffer chunks the way streams deliver them", ({ expect }) => {
		const emit = vi.fn()
		const splitter = createLineSplitter(emit)

		splitter.data(Buffer.from("from a buffer\n"))

		expect(emit.mock.calls).toEqual([["from a buffer"]])
	})
})
