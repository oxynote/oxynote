import { getLocalTimeZone } from "@internationalized/date"

export function useTimeRangeZone() {
	const choice = usePersistentState<string>({
		key: "custom-time-range-zone",
		defaultValue: "browser",
		storage: { web: "local-storage", desktop: "local-storage" },
	})

	// older clients stored the resolved browser zone instead of the choice
	if (choice.value !== "browser" && choice.value !== "UTC") {
		choice.value = "browser"
	}

	const zone = computed(() =>
		choice.value === "UTC" ? "UTC" : getLocalTimeZone(),
	)

	return { choice, zone }
}
