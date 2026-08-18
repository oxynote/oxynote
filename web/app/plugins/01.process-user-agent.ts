export default defineNuxtPlugin(() => {
	const { setOsType, setBrowserType } = useDetectHost()

	if (import.meta.server) {
		// NOCOV: server-only branch; tests run the client bundle, where
		// import.meta.server is literal false.
		const headers = useRequestHeaders(["user-agent", "sec-ch-ua-platform"])
		const ua = headers["user-agent"] || ""
		const secChUaPlatform = headers["sec-ch-ua-platform"] || ""

		setOsType(detectOsType(ua, secChUaPlatform))
		setBrowserType(detectBrowserType(ua))

		return
	}

	if (__DESKTOP_BUILD__) {
		// NOCOV: desktop-only branch; __DESKTOP_BUILD__ is literal false in
		// web and test bundles.
		const { $host } = useNuxtApp()
		if (!$host) {
			return
		}

		setOsType($host.osType)
		setBrowserType(HostBrowserType.NonBrowser)
	}
})
