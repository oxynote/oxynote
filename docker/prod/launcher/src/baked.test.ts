import { describe, it } from "vitest"
import {
	bakedImageVersion,
	bakedMigrationVersion,
	bakedSentryDsns,
} from "./baked.js"

describe("bakedSentryDsns", () => {
	it("falls back to disabled telemetry outside an image build", ({
		expect,
	}) => {
		// esbuild injects the DSNs with --define only in the image
		// build; here nothing is defined, so every fallback applies
		expect(bakedSentryDsns).toEqual({
			webDsn: "",
			coreDsn: "",
			authRealtimeDsn: "",
		})
	})
})

describe("bakedImageVersion", () => {
	it("names a build outside the image dev, with no migration version", ({
		expect,
	}) => {
		expect(bakedImageVersion).toBe("dev")
		expect(bakedMigrationVersion).toBeUndefined()
	})
})
