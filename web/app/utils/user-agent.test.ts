import { describe, it } from "vitest"
import { detectBrowserType, detectOsType } from "./user-agent"

describe("detectOsType", () => {
	it.for([
		{ name: "detects ios from the platform hint", platform: '"iOS"' },
		{ name: "detects ios from a lowercase platform hint", platform: "ios" },
	])("$name", ({ platform }, { expect }) => {
		expect(detectOsType("", platform)).toBe(HostOsType.IOS)
	})

	it.for([
		{ platform: '"Android"', expected: HostOsType.Android },
		{ platform: '"macOS"', expected: HostOsType.MacOS },
		{ platform: '"Windows"', expected: HostOsType.Windows },
		{ platform: '"Linux"', expected: HostOsType.Linux },
	])(
		"maps the $platform platform hint to $expected",
		({ platform, expected }, { expect }) => {
			expect(detectOsType("", platform)).toBe(expected)
		},
	)

	it("prefers the platform hint over a contradicting user agent", ({
		expect,
	}) => {
		expect(
			detectOsType("Mozilla/5.0 (Windows NT 10.0; Win64; x64)", '"macOS"'),
		).toBe(HostOsType.MacOS)
	})

	it.for([
		{
			name: "detects windows from the user agent",
			ua: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
			expected: HostOsType.Windows,
		},
		{
			name: "detects android from the user agent",
			ua: "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36",
			expected: HostOsType.Android,
		},
		{
			name: "detects ios from an iphone user agent",
			ua: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)",
			expected: HostOsType.IOS,
		},
		{
			name: "detects ios from an ipados desktop-style user agent",
			ua: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Mobile/15E148",
			expected: HostOsType.IOS,
		},
		{
			name: "detects macos from the user agent",
			ua: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15",
			expected: HostOsType.MacOS,
		},
		{
			name: "detects linux from the user agent",
			ua: "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:124.0) Gecko/20100101",
			expected: HostOsType.Linux,
		},
		{
			name: "falls back to other for an empty user agent",
			ua: "",
			expected: HostOsType.Other,
		},
		{
			name: "falls back to other for an unrecognized user agent",
			ua: "curl/8.4.0",
			expected: HostOsType.Other,
		},
	])("$name", ({ ua, expected }, { expect }) => {
		expect(detectOsType(ua, "")).toBe(expected)
	})
})

describe("detectBrowserType", () => {
	it.for([
		{
			name: "reports non-browser for an empty user agent",
			ua: "",
			expected: HostBrowserType.NonBrowser,
		},
		{
			name: "reports non-browser for a non-browser user agent",
			ua: "curl/8.4.0",
			expected: HostBrowserType.NonBrowser,
		},
		{
			name: "detects desktop firefox",
			ua: "Mozilla/5.0 (X11; Linux x86_64; rv:124.0) Gecko/20100101 Firefox/124.0",
			expected: HostBrowserType.Firefox,
		},
		{
			name: "detects firefox on ios",
			ua: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) FxiOS/124.0 Mobile/15E148 Safari/605.1.15",
			expected: HostBrowserType.Firefox,
		},
		{
			name: "detects chrome as chromium despite its safari token",
			ua: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36",
			expected: HostBrowserType.Chromium,
		},
		{
			name: "detects edge as chromium",
			ua: "Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
			expected: HostBrowserType.Chromium,
		},
		{
			name: "detects opera as chromium",
			ua: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36 OPR/106.0.0.0",
			expected: HostBrowserType.Chromium,
		},
		{
			name: "detects safari when no chromium token is present",
			ua: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/17.0 Safari/605.1.15",
			expected: HostBrowserType.Safari,
		},
		{
			name: "reports other for a generic browser-like user agent",
			ua: "Mozilla/5.0 (compatible; ExampleBot/1.0)",
			expected: HostBrowserType.Other,
		},
	])("$name", ({ ua, expected }, { expect }) => {
		expect(detectBrowserType(ua)).toBe(expected)
	})
})
