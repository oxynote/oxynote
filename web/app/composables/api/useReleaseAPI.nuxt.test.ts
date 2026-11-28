import { flushPromises } from "@vue/test-utils"
import { setResponseHeader } from "h3"
import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	runInApp,
} from "./test-helpers"
import useReleaseAPI from "./useReleaseAPI"

function makeReleaseAPI() {
	return runInApp(() => useReleaseAPI())
}

// the tests share the app-wide query cache and the test-time endpoint
// registry, so they cannot interleave
describe("useReleaseAPI", { concurrent: false }, () => {
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	describe("useFetchLatestRelease", () => {
		it("fetches the latest release core knows about", async ({ expect }) => {
			const release = {
				version: "0.12.0",
				notesUrl: "https://oxynote.io/blog/oxynote-v0-12",
			}
			const calls = mockEndpoint("GET", "/api/releases/latest", () => release)
			const api = makeReleaseAPI()

			const result = await runInApp(() =>
				api.useFetchLatestRelease(true),
			).refresh()

			expect(result.data).toEqual(release)
			expect(calls).toHaveLength(1)
		})

		it("reads no release while core has none", async ({ expect }) => {
			// core answers with a json null, not an empty body
			mockEndpoint("GET", "/api/releases/latest", (_call, event) => {
				setResponseHeader(event, "content-type", "application/json")

				return "null"
			})
			const api = makeReleaseAPI()

			const result = await runInApp(() =>
				api.useFetchLatestRelease(true),
			).refresh()

			expect(result.data).toBeNull()
		})

		it("sends no request while disabled", async ({ expect }) => {
			const calls = mockEndpoint("GET", "/api/releases/latest", () => null)
			const api = makeReleaseAPI()
			const query = runInApp(() => api.useFetchLatestRelease(false))

			await flushPromises()

			expect(query.data.value).toBeUndefined()
			expect(calls).toHaveLength(0)
		})
	})
})
