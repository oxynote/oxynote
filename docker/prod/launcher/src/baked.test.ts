import { describe, it } from "vitest"
import { bakedSentryDsns } from "./baked.js"

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
