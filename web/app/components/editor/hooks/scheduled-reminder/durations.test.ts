import { describe, it } from "vitest"
import { presetDurations } from "./durations"

describe("presetDurations", () => {
	// import.meta.dev is undefined in the vitest node project, so the
	// list under test is the production one — without the dev-only
	// "0" and "1m" entries
	it("lists the production durations with the custom option last", ({
		expect,
	}) => {
		expect(presetDurations).toEqual([
			"24h",
			"72h",
			"168h",
			"336h",
			"720h",
			"2160h",
			"4320h",
			"custom",
		])
	})
})
