import type { MermaidConfig } from "mermaid"
import { beforeEach, describe, it, vi, type Mock } from "vitest"
import { ref } from "vue"

const themeColors = {
	background: "#000001",
	foreground: "#000002",
	card: "#000003",
	muted: "#000004",
	mutedForeground: "#000005",
	accent: "#000006",
	border: "#000007",
	primary: "#000008",
	primaryForeground: "#000009",
	destructive: "#00000a",
	destructiveForeground: "#00000b",
	chart: Array.from({ length: 12 }, (_, i) => `#c0000${i.toString(16)}`),
	fontFamily: "Test Sans",
}

interface ThemeVariables {
	[key: string]: unknown
	xyChart?: { plotColorPalette?: string }
	radar?: { graticuleColor?: string }
}

interface MermaidStub {
	initialize: Mock
	render: Mock
}

// the composable keeps the loaded module, the in-flight load promise and
// the last applied theme in module scope, so every case re-imports it
// against a freshly mocked "mermaid" to start from a clean slate
async function loadComposable(
	options: { failLoad?: boolean; stub?: Partial<MermaidStub> } = {},
) {
	const stub: MermaidStub = {
		initialize: vi.fn(),
		render: vi.fn().mockResolvedValue({ svg: "<svg />" }),
		...options.stub,
	}

	vi.resetModules()
	vi.doMock("~/assets/css", () => ({ mermaidThemeColors: () => themeColors }))
	vi.doMock("mermaid", () => {
		if (options.failLoad) {
			throw new Error("chunk unavailable")
		}

		return { default: stub }
	})

	const mod = await import("./useMermaid")

	return { useMermaid: mod.useMermaid, stub }
}

function initializedConfig(stub: MermaidStub, call = 0): MermaidConfig {
	return stub.initialize.mock.calls[call]?.[0] as MermaidConfig
}

// suites share the composable's module-scoped load state, and each case
// rebuilds it through vi.resetModules — interleaving would cross-wire
// the mocks
describe("useMermaid", { concurrent: false }, () => {
	beforeEach(() => {
		vi.stubGlobal("useI18n", () => ({ t: (key: string) => key }))
	})

	it("renders the source through the loaded mermaid module", async ({
		expect,
	}) => {
		const { useMermaid, stub } = await loadComposable()

		const { render } = useMermaid(ref(false))
		const result = await render("id-1", "graph LR")

		expect(result).toEqual({ svg: "<svg />" })
		expect(stub.render).toHaveBeenCalledTimes(1)
		expect(stub.render).toHaveBeenCalledWith("id-1", "graph LR")
		expect(stub.initialize).toHaveBeenCalledTimes(1)
	})

	it("clears the loading flag and the load error after a successful load", async ({
		expect,
	}) => {
		const { useMermaid } = await loadComposable()

		const { isLoading, loadError, render } = useMermaid(ref(false))
		await render("id-1", "graph LR")

		expect(isLoading.value).toBe(false)
		expect(loadError.value).toBeNull()
	})

	it("initializes mermaid with the theme colors", async ({ expect }) => {
		const { useMermaid, stub } = await loadComposable()

		const { render } = useMermaid(ref(true))
		await render("id-1", "graph LR")

		const config = initializedConfig(stub)

		expect(config.startOnLoad).toBe(false)
		expect(config.securityLevel).toBe("strict")
		expect(config.suppressErrorRendering).toBe(true)
		expect(config.theme).toBe("null")
		expect(config.darkMode).toBe(true)
		expect(config.fontFamily).toBe(themeColors.fontFamily)
	})

	it("maps the chart palette onto every indexed theme variable", async ({
		expect,
	}) => {
		const { useMermaid, stub } = await loadComposable()

		const { render } = useMermaid(ref(false))
		await render("id-1", "graph LR")

		const vars = (initializedConfig(stub).themeVariables ??
			{}) as ThemeVariables

		expect(vars.pie1).toBe(themeColors.chart[0])
		expect(vars.pie12).toBe(themeColors.chart[11])
		expect(vars.cScale11).toBe(themeColors.chart[11])
		expect(vars.venn8).toBe(themeColors.chart[7])
		expect(vars.git7).toBe(themeColors.chart[7])
		expect(vars.fillType7).toBe(themeColors.chart[7])
		expect(vars.xyChart?.plotColorPalette).toBe(themeColors.chart.join(","))
		expect(vars.radar?.graticuleColor).toBe(themeColors.border)
	})

	it("reinitializes mermaid when the dark flag flips between renders", async ({
		expect,
	}) => {
		const { useMermaid, stub } = await loadComposable()
		const dark = ref(false)

		const { render } = useMermaid(dark)
		await render("id-1", "graph LR")
		dark.value = true
		await render("id-2", "graph LR")

		expect(stub.initialize).toHaveBeenCalledTimes(2)
		expect(initializedConfig(stub, 0).darkMode).toBe(false)
		expect(initializedConfig(stub, 1).darkMode).toBe(true)
	})

	it("keeps the existing configuration when the dark flag is unchanged", async ({
		expect,
	}) => {
		const { useMermaid, stub } = await loadComposable()

		const { render } = useMermaid(ref(false))
		await render("id-1", "graph LR")
		await render("id-2", "graph LR")

		expect(stub.initialize).toHaveBeenCalledTimes(1)
		expect(stub.render).toHaveBeenCalledTimes(2)
	})

	it("loads mermaid once across several composable instances", async ({
		expect,
	}) => {
		const { useMermaid, stub } = await loadComposable()

		const first = useMermaid(ref(false))
		const second = useMermaid(ref(false))
		await first.render("id-1", "graph LR")
		await second.render("id-2", "graph LR")

		expect(stub.initialize).toHaveBeenCalledTimes(1)
	})

	it("reuses the in-flight load promise instead of importing twice", async ({
		expect,
	}) => {
		const { useMermaid, stub } = await loadComposable()

		// the setup call starts the load without awaiting it, so the
		// render below reaches loadMermaid while the promise is pending
		const { render } = useMermaid(ref(false))
		const result = await render("id-1", "graph LR")

		expect(result).toEqual({ svg: "<svg />" })
		expect(stub.initialize).toHaveBeenCalledTimes(1)
	})

	it("leaves the module unloaded when initialization throws", async ({
		expect,
	}) => {
		const { useMermaid, stub } = await loadComposable({
			stub: {
				initialize: vi.fn(() => {
					throw new Error("bad config")
				}),
			},
		})

		const { render, loadError } = useMermaid(ref(false))
		const result = await render("id-1", "graph LR")

		expect(result).toEqual({ error: "bad config" })
		expect(loadError.value).toBe("bad config")
		expect(stub.initialize).toHaveBeenCalledTimes(1)
		expect(stub.render).toHaveBeenCalledTimes(0)
	})

	it("surfaces the import failure as the render error", async ({ expect }) => {
		const { useMermaid } = await loadComposable({ failLoad: true })

		const { render, loadError, isLoading } = useMermaid(ref(false))
		const result = await render("id-1", "graph LR")

		expect(result).toEqual({ error: loadError.value })
		expect(loadError.value).toEqual(expect.any(String))
		expect(isLoading.value).toBe(false)
	})

	it("retries the import after a failed load", async ({ expect }) => {
		const { useMermaid } = await loadComposable({ failLoad: true })

		const { render } = useMermaid(ref(false))
		await render("id-1", "graph LR")
		const stub: MermaidStub = {
			initialize: vi.fn(),
			render: vi.fn().mockResolvedValue({ svg: "<svg />" }),
		}
		vi.doMock("mermaid", () => ({ default: stub }))
		const result = await render("id-2", "graph LR")

		expect(result).toEqual({ svg: "<svg />" })
		expect(stub.initialize).toHaveBeenCalledTimes(1)
	})

	it("returns the message of an error thrown while rendering", async ({
		expect,
	}) => {
		const { useMermaid } = await loadComposable({
			stub: { render: vi.fn().mockRejectedValue(new Error("bad syntax")) },
		})

		const { render } = useMermaid(ref(false))
		const result = await render("id-1", "graph LR")

		expect(result).toEqual({ error: "bad syntax" })
	})

	it("falls back to the render-failed message for a non-error throw", async ({
		expect,
	}) => {
		const { useMermaid } = await loadComposable({
			stub: { render: vi.fn().mockRejectedValue("boom") },
		})

		const { render } = useMermaid(ref(false))
		const result = await render("id-1", "graph LR")

		expect(result).toEqual({ error: "editor.mermaid.errors.render-failed" })
	})
})
