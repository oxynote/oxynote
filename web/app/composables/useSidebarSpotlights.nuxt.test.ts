import { flushPromises } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	runInApp,
} from "./api/test-helpers"
import useSidebarSpotlights from "./useSidebarSpotlights"

const NOTES_URL = "https://oxynote.io/blog/oxynote-v0-12"

// the runtime config, the query cache, the endpoint registry and the
// persisted collapse are app-wide singletons the tests share
describe("useSidebarSpotlights", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		collapsedVersion().value = ""
	})

	afterEach(() => {
		disposeMockEndpoints()
		useRuntimeConfig().public.appVersion = ""
	})

	it("checks nothing on a build without a release version", async ({
		expect,
	}) => {
		const { releaseCalls } = arrange({ version: "dev", latest: "0.12.0" })

		const spotlights = await makeSpotlights()

		expect.soft(spotlights.spotlight.value).toBeNull()
		expect.soft(spotlights.newRelease.value).toBeNull()
		expect.soft(spotlights.newReleaseRowOpen.value).toBe(false)
		expect.soft(releaseCalls).toHaveLength(0)
	})

	it("offers the update card when a newer release is out", async ({
		expect,
	}) => {
		const { releaseCalls } = arrange({ version: "0.11.0", latest: "0.12.0" })

		const spotlights = await makeSpotlights()

		expect.soft(spotlights.spotlight.value).toBe("update")
		expect.soft(spotlights.shownSpotlight.value).toBe("update")
		expect.soft(spotlights.newRelease.value).toEqual({
			latestVersion: "0.12.0",
			currentVersion: "0.11.0",
			notesUrl: NOTES_URL,
		})
		expect.soft(spotlights.newReleaseRowOpen.value).toBe(false)
		expect.soft(releaseCalls).toHaveLength(1)
	})

	it("offers nothing while the app runs the latest release", async ({
		expect,
	}) => {
		arrange({ version: "0.12.0", latest: "0.12.0" })

		const spotlights = await makeSpotlights()

		expect.soft(spotlights.spotlight.value).toBeNull()
		expect.soft(spotlights.newRelease.value).toBeNull()
		expect.soft(spotlights.newReleaseRowOpen.value).toBe(false)
	})

	it("folds the closed update card into the row and remembers its version", async ({
		expect,
	}) => {
		arrange({ version: "0.11.0", latest: "0.12.0" })
		const spotlights = await makeSpotlights()

		spotlights.collapseUpdateSpotlight()

		expect.soft(spotlights.spotlight.value).toBeNull()
		// the card keeps its content while it folds away
		expect.soft(spotlights.shownSpotlight.value).toBe("update")
		expect.soft(spotlights.newReleaseRowOpen.value).toBe(true)
		expect.soft(collapsedVersion().value).toBe("0.12.0")
	})

	it("keeps the row for a release whose card was closed before", async ({
		expect,
	}) => {
		collapsedVersion().value = "0.12.0"
		arrange({ version: "0.11.0", latest: "0.12.0" })

		const spotlights = await makeSpotlights()

		expect.soft(spotlights.spotlight.value).toBeNull()
		expect.soft(spotlights.newReleaseRowOpen.value).toBe(true)
	})

	it("shows the card again for a release newer than the closed one", async ({
		expect,
	}) => {
		collapsedVersion().value = "0.12.0"
		arrange({ version: "0.11.0", latest: "0.13.0" })

		const spotlights = await makeSpotlights()

		expect.soft(spotlights.spotlight.value).toBe("update")
		expect.soft(spotlights.newReleaseRowOpen.value).toBe(false)
	})

	it("lets a shown star spotlight win and keeps the update in its row", async ({
		expect,
	}) => {
		arrange({
			version: "0.11.0",
			latest: "0.12.0",
			star: SpotlightState.Shown,
		})

		const spotlights = await makeSpotlights()

		expect.soft(spotlights.spotlight.value).toBe("star")
		expect.soft(spotlights.newReleaseRowOpen.value).toBe(true)
	})

	it("shows nothing for a dismissed star spotlight", async ({ expect }) => {
		arrange({ version: "dev", star: SpotlightState.Dismissed })

		const spotlights = await makeSpotlights()

		expect.soft(spotlights.spotlight.value).toBeNull()
		expect.soft(spotlights.shownSpotlight.value).toBeNull()
	})

	it("dismisses the star spotlight for good and leaves a waiting update in its row", async ({
		expect,
	}) => {
		const { spotlightWrites } = arrange({
			version: "0.11.0",
			latest: "0.12.0",
			star: SpotlightState.Shown,
		})
		const spotlights = await makeSpotlights()

		await spotlights.closeStarSpotlight()

		expect.soft(spotlights.spotlight.value).toBeNull()
		expect.soft(spotlights.shownSpotlight.value).toBe("star")
		expect.soft(spotlights.newReleaseRowOpen.value).toBe(true)
		expect.soft(spotlightWrites).toHaveLength(1)
		expect
			.soft(spotlightWrites[0]?.body)
			.toEqual({ star: SpotlightState.Dismissed })
	})

	it("keeps the star spotlight closed for this load when the write fails", async ({
		expect,
	}) => {
		const { spotlightWrites } = arrange({
			version: "dev",
			star: SpotlightState.Shown,
			failWrite: true,
		})
		const spotlights = await makeSpotlights()

		await spotlights.closeStarSpotlight()

		expect.soft(spotlights.spotlight.value).toBeNull()
		expect.soft(spotlights.shownSpotlight.value).toBe("star")
		expect.soft(spotlightWrites).toHaveLength(1)
	})
})

function arrange(options: {
	version: string
	latest?: string
	star?: SpotlightState
	failWrite?: boolean
}) {
	useRuntimeConfig().public.appVersion = options.version

	const releaseCalls = mockEndpoint("GET", "/api/releases/latest", () => ({
		version: options.latest,
		notesUrl: NOTES_URL,
	}))
	const spotlightWrites = mockEndpoint("PUT", "/api/users/spotlights", () => {
		if (options.failWrite) {
			throw createError({ statusCode: 500 })
		}

		return null
	})
	// a write that succeeds dismisses the spotlight, and the refetch after it
	// reads that back
	mockEndpoint("GET", "/api/users/spotlights", () => ({
		star:
			spotlightWrites.length && !options.failWrite
				? SpotlightState.Dismissed
				: (options.star ?? null),
	}))

	return { releaseCalls, spotlightWrites }
}

// the composable starts its loads on creation, and refresh() joins them
// rather than sending a second request
async function makeSpotlights() {
	const spotlights = runInApp(() => useSidebarSpotlights())
	await runInApp(() => useUserAPI().useFetchUserSpotlights()).refresh()

	if (isReleaseVersion(useRuntimeConfig().public.appVersion)) {
		await runInApp(() => useReleaseAPI().useFetchLatestRelease(true)).refresh()
	}

	await flushPromises()

	return spotlights
}

function collapsedVersion() {
	return runInApp(() =>
		usePersistentState<string>({
			key: "update-spotlight-collapsed-version",
			defaultValue: "",
			storage: { web: "local-storage" },
		}),
	)
}
