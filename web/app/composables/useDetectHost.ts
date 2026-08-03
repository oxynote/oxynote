export enum HostPlatformType {
	Web = "web",
	Desktop = "desktop",
}

export enum HostOsType {
	MacOS = "macOS",
	IOS = "ios",
	Windows = "windows",
	Linux = "linux",
	Android = "android",
	Other = "other",
}

export enum HostBrowserType {
	Firefox = "firefox",
	Safari = "safari",
	Chromium = "chromium", // chromium-based
	Other = "other", // still browser
	NonBrowser = "non-browser",
}

export default function () {
	const platformType = useState<HostPlatformType>("platformType", () =>
		__DESKTOP_BUILD__ ? HostPlatformType.Desktop : HostPlatformType.Web,
	)
	const osType = useState<HostOsType>("osType", () => HostOsType.Other)
	const browserType = useState<HostBrowserType>(
		"browserType",
		() => HostBrowserType.NonBrowser,
	)

	const isDesktop = computed(
		() => platformType.value === HostPlatformType.Desktop,
	)
	const isWeb = computed(() => platformType.value === HostPlatformType.Web)

	function setOsType(t: HostOsType) {
		osType.value = t
	}

	function setBrowserType(t: HostBrowserType) {
		browserType.value = t
	}

	return {
		platformType: readonly(platformType),
		isDesktop,
		isWeb,
		osType: readonly(osType),
		setOsType,
		browserType: readonly(browserType),
		setBrowserType,
	}
}
