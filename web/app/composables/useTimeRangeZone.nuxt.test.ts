import { beforeEach, afterEach, describe, it } from "vitest"
import {
	getLocalTimeZone,
	setLocalTimeZone,
	resetLocalTimeZone,
} from "@internationalized/date"
import { seedPersistentState } from "~/components/test-helpers"
import { runInApp } from "~/composables/api/test-helpers"
import { useTimeRangeZone } from "./useTimeRangeZone"

// the browser zone and persisted preference are shared app state
describe("useTimeRangeZone", { concurrent: false }, () => {
	beforeEach(() => {
		seedPersistentState("custom-time-range-zone", "browser")
	})
	afterEach(resetLocalTimeZone)

	it("resolves browser time again when a picker opens in another zone", ({
		expect,
	}) => {
		setLocalTimeZone("America/New_York")
		const first = runInApp(() => useTimeRangeZone())
		expect(first.zone.value).toBe("America/New_York")
		setLocalTimeZone("Europe/Paris")
		const next = runInApp(() => useTimeRangeZone())
		expect(next.choice.value).toBe("browser")
		expect(next.zone.value).toBe("Europe/Paris")
	})

	it("migrates a legacy resolved browser zone to the browser choice", ({
		expect,
	}) => {
		seedPersistentState("custom-time-range-zone", "America/New_York")
		setLocalTimeZone("Europe/Paris")
		const state = runInApp(() => useTimeRangeZone())
		expect(state.choice.value).toBe("browser")
		expect(state.zone.value).toBe("Europe/Paris")
	})

	it("keeps an explicit UTC choice when the browser zone changes", ({
		expect,
	}) => {
		seedPersistentState("custom-time-range-zone", "UTC")
		setLocalTimeZone("Europe/Paris")
		expect(runInApp(() => useTimeRangeZone()).zone.value).toBe("UTC")
		expect(getLocalTimeZone()).toBe("Europe/Paris")
	})
})
