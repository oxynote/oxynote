type SidebarSpotlightKind = "star" | "update"

interface SidebarNewRelease {
	latestVersion: string
	currentVersion: string
	notesUrl: string
}

// the sidebar shows one spotlight card at a time, and a star spotlight the app
// has started showing wins over an update. Closing the update card folds it
// into a row above Docs. The row stays until the app runs that version.
export default function () {
	const config = useRuntimeConfig()
	const { useFetchLatestRelease } = useReleaseAPI()
	const { useFetchUserSpotlights, updateUserSpotlights } = useUserAPI()

	const currentVersion = config.public.appVersion
	const fetchLatestRelease = useFetchLatestRelease(
		isReleaseVersion(currentVersion),
	)
	const fetchUserSpotlights = useFetchUserSpotlights()
	// the version whose card was closed, so a newer release shows its card
	// again once
	const collapsedUpdateVersion = usePersistentState<string>({
		key: "update-spotlight-collapsed-version",
		defaultValue: "",
		storage: { web: "local-storage" },
	})
	// a closed star card stays closed for this load. When the write fails,
	// the next load shows it again. An update that waits stays in its row
	// until the next load too, so no second card replaces the one just
	// closed.
	const starClosedThisLoad = ref(false)
	// the card keeps its content while it folds away
	const shownSpotlight = ref<SidebarSpotlightKind | null>(null)

	const newRelease = computed<SidebarNewRelease | null>(() => {
		const latest = fetchLatestRelease.data.value
		if (!latest || !isNewerRelease(latest.version, currentVersion)) {
			return null
		}

		return {
			latestVersion: latest.version,
			currentVersion: currentVersion,
			notesUrl: latest.notesUrl,
		}
	})
	const spotlight = computed<SidebarSpotlightKind | null>(() => {
		if (starClosedThisLoad.value) {
			return null
		}

		if (fetchUserSpotlights.data.value?.star === SpotlightState.Shown) {
			return "star"
		}

		if (
			newRelease.value &&
			collapsedUpdateVersion.value !== newRelease.value.latestVersion
		) {
			return "update"
		}

		return null
	})
	const newReleaseRowOpen = computed(
		() => newRelease.value !== null && spotlight.value !== "update",
	)

	watchImmediate(spotlight, (value) => {
		if (value) {
			shownSpotlight.value = value
		}
	})

	async function closeStarSpotlight() {
		starClosedThisLoad.value = true

		try {
			await updateUserSpotlights.mutateAsync({
				star: SpotlightState.Dismissed,
			})
		} catch {
			// a failed write only means the card comes back on the next load
		}
	}

	function collapseUpdateSpotlight() {
		if (newRelease.value) {
			collapsedUpdateVersion.value = newRelease.value.latestVersion
		}
	}

	return {
		spotlight,
		shownSpotlight,
		newRelease,
		newReleaseRowOpen,
		closeStarSpotlight,
		collapseUpdateSpotlight,
	}
}
