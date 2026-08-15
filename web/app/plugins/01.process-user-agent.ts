const FIREFOX_UA_PATTERN = /Firefox\/|FxiOS\//
const CHROMIUM_UA_PATTERN =
	/Chrome\/|Chromium\/|CriOS\/|Edg(?:A|E|iOS)?\/|OPR\/|Opera\/|Brave\/|SamsungBrowser\//
const BROWSER_UA_PATTERN = /Mozilla\/|AppleWebKit\/|Gecko\//
const WINDOWS_UA_PATTERN = /Windows NT|Win64|WOW64/
const MACOS_UA_PATTERN = /Macintosh|Mac OS X/
const IOS_UA_PATTERN = /iPhone|iPad|iPod/
const LINUX_UA_PATTERN = /X11|Linux|Ubuntu|Fedora|Debian|CrOS/

export default defineNuxtPlugin(() => {
	const { setOsType, setBrowserType } = useDetectHost()

	if (import.meta.server) {
		const headers = useRequestHeaders(["user-agent", "sec-ch-ua-platform"])
		const ua = headers["user-agent"] || ""
		const secChUaPlatform = headers["sec-ch-ua-platform"] || ""

		setOsType(detectOsType(ua, secChUaPlatform))
		setBrowserType(detectBrowserType(ua))

		return
	}

	if (__DESKTOP_BUILD__) {
		const { $host } = useNuxtApp()
		if (!$host) {
			return
		}

		setOsType($host.osType)
		setBrowserType(HostBrowserType.NonBrowser)
	}
})

function detectOsType(ua: string, secChUaPlatform: string): HostOsType {
	const normalizedPlatform = secChUaPlatform
		.replaceAll('"', "")
		.trim()
		.toLowerCase()

	if (normalizedPlatform === "ios") {
		return HostOsType.IOS
	}

	if (normalizedPlatform === "android") {
		return HostOsType.Android
	}

	if (normalizedPlatform === "macos") {
		return HostOsType.MacOS
	}

	if (normalizedPlatform === "windows") {
		return HostOsType.Windows
	}

	if (normalizedPlatform === "linux") {
		return HostOsType.Linux
	}

	if (WINDOWS_UA_PATTERN.test(ua)) {
		return HostOsType.Windows
	}

	if (ua.includes("Android")) {
		return HostOsType.Android
	}

	if (
		IOS_UA_PATTERN.test(ua) ||
		(ua.includes("Macintosh") && ua.includes("Mobile/"))
	) {
		return HostOsType.IOS
	}

	if (MACOS_UA_PATTERN.test(ua)) {
		return HostOsType.MacOS
	}

	if (LINUX_UA_PATTERN.test(ua)) {
		return HostOsType.Linux
	}

	return HostOsType.Other
}

function detectBrowserType(ua: string): HostBrowserType {
	if (!ua) {
		return HostBrowserType.NonBrowser
	}

	if (FIREFOX_UA_PATTERN.test(ua)) {
		return HostBrowserType.Firefox
	}

	const isChromium = CHROMIUM_UA_PATTERN.test(ua)
	const isSafari = ua.includes("Safari/") && !isChromium

	if (isSafari) {
		return HostBrowserType.Safari
	}

	if (isChromium) {
		return HostBrowserType.Chromium
	}

	if (BROWSER_UA_PATTERN.test(ua)) {
		return HostBrowserType.Other
	}

	return HostBrowserType.NonBrowser
}
