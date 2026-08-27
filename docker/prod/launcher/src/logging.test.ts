import { describe, it, vi } from "vitest"
import { createLinePrefixer } from "./logging.js"

describe("createLinePrefixer", () => {
	it("prefixes each complete line in a chunk", ({ expect }) => {
		const write = vi.fn()
		const prefixer = createLinePrefixer("[core]", write)

		prefixer.data("first\nsecond\n")

		expect(write.mock.calls).toEqual([
			["[core] first"],
			["[core] second"],
		])
	})

	it("holds a trailing fragment until its newline arrives", ({
		expect,
	}) => {
		const write = vi.fn()
		const prefixer = createLinePrefixer("[web]", write)

		prefixer.data("par")
		expect(write).toHaveBeenCalledTimes(0)

		prefixer.data("tial\nrest")

		expect(write.mock.calls).toEqual([["[web] partial"]])
	})

	it("flushes the held fragment at stream end", ({ expect }) => {
		const write = vi.fn()
		const prefixer = createLinePrefixer("[auth-realtime]", write)

		prefixer.data("no newline")
		prefixer.end()

		expect(write.mock.calls).toEqual([
			["[auth-realtime] no newline"],
		])
	})

	it("ends quietly when nothing is buffered", ({ expect }) => {
		const write = vi.fn()
		const prefixer = createLinePrefixer("[caddy]", write)

		prefixer.data("done\n")
		prefixer.end()

		expect(write).toHaveBeenCalledTimes(1)
	})

	it("reads buffer chunks the way streams deliver them", ({ expect }) => {
		const write = vi.fn()
		const prefixer = createLinePrefixer("[core]", write)

		prefixer.data(Buffer.from("from a buffer\n"))

		expect(write.mock.calls).toEqual([["[core] from a buffer"]])
	})
})
