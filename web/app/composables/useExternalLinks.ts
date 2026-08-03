export default function () {
	const { $host } = useNuxtApp()
	const { isDesktop } = useDetectHost()

	function openExternalLink(url: string) {
		if (isDesktop.value) {
			$host?.openExternal(url)
			return
		}

		if (typeof window !== "undefined") {
			window.open(url, "_blank", "noopener")
		}
	}

	return {
		openExternalLink,
	}
}
