export default function () {
	const colorCookie = usePersistentState<"light" | "dark" | "auto">({
		key: "color-mode",
		defaultValue: "auto",
		watch: "shallow",
		cookie: {
			sameSite: "lax",
			secure: false,
		},
	})

	const lastAutoColorCookie = usePersistentState<"light" | "dark">({
		key: "last-auto-resolved-color-mode",
		defaultValue: "light",
		watch: "shallow",
		cookie: {
			sameSite: "lax",
			secure: false,
		},
	})
	// useColorMode is capable of adding "dark" class to html element,
	// but we disable that behavior to reduce flickering (light to dark on load)
	// and handle it manually in app.vue
	const mode = useColorMode({
		storageKey: null,
		storageRef: colorCookie,
		initialValue: colorCookie.value,
		selector: "html",
		attribute: "class",
		onChanged: () => {
			// disable automatic class handling
		},
	})
	const resolvedMode = computed<"light" | "dark">(() => {
		if (mode.store.value === "auto") {
			if (import.meta.server) {
				// on SSR, fall back to last auto (or light by default)
				return lastAutoColorCookie.value
			}

			// on client, use system preference from useColorMode
			return mode.system.value === "dark" ? "dark" : "light"
		}

		return mode.store.value
	})

	if (import.meta.client) {
		watchImmediate([mode.store, mode.system], () => {
			if (mode.store.value === "auto") {
				lastAutoColorCookie.value =
					mode.system.value === "dark" ? "dark" : "light"
				return
			}

			lastAutoColorCookie.value = mode.store.value
		})
	}

	function changeColorTheme(theme: "light" | "dark" | "auto") {
		mode.value = theme
	}

	return {
		color: computed(() => {
			return {
				system: mode.store.value === "auto",
				theme: resolvedMode.value,
			}
		}),
		isDark: computed(() => resolvedMode.value === "dark"),
		changeColorTheme,
	}
}
