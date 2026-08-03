export default function () {
	const config = useRuntimeConfig()
	const route = useRoute()

	function createCurrentUrl(options?: { hash?: string | null }) {
		const url = new URL(route.fullPath || "/", config.public.appBaseURL)

		if (options && "hash" in options) {
			url.hash = options.hash || ""
		}

		return url
	}

	return {
		currentUrl: computed(() => createCurrentUrl()),
		createCurrentUrl,
	}
}
