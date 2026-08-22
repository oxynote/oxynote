import { mountSuspended } from "@nuxt/test-utils/runtime"
import { enableAutoUnmount, type VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import type { Ref } from "vue"
import MermaidPreview from "./MermaidPreview.vue"
import { t, WAIT_FOR_OPTIONS } from "~/components/test-helpers"

const DEBOUNCE_MS = 400

// the real composable dynamically imports mermaid and drives a browser
// renderer; the suite drives its three outputs directly instead
const mermaid = vi.hoisted(() => {
	return {
		render:
			vi.fn<(id: string, source: string) => Promise<Record<string, string>>>(),
		isLoading: null as unknown as Ref<boolean>,
		loadError: null as unknown as Ref<string | null>,
	}
})

vi.mock("./useMermaid", async () => {
	const { ref } = await import("vue")

	mermaid.isLoading = ref(false)
	mermaid.loadError = ref<string | null>(null)

	return {
		useMermaid: () => ({
			render: mermaid.render,
			isLoading: mermaid.isLoading,
			loadError: mermaid.loadError,
		}),
	}
})

function mountPreview(source: string) {
	return mountSuspended(MermaidPreview, {
		props: { source: source, uid: "mermaid-1" },
	})
}

// puts the colour mode back on "follow the system" and hands back a
// switch that flips the system preference to dark, the way a reader
// changing their OS theme would
function followSystemTheme(): () => void {
	const listeners: ((event: { matches: boolean }) => void)[] = []
	const query = {
		matches: false,
		media: "(prefers-color-scheme: dark)",
		onchange: null,
		addEventListener: (
			_event: string,
			listener: (e: { matches: boolean }) => void,
		) => {
			listeners.push(listener)
		},
		removeEventListener: () => undefined,
		dispatchEvent: () => true,
	}

	vi.stubGlobal(
		"matchMedia",
		vi.fn(() => query),
	)
	useAppearance().changeColorTheme("auto")

	return () => {
		query.matches = true
		listeners.forEach((listener) => {
			listener({ matches: true })
		})
	}
}

function previewHtml(wrapper: VueWrapper): string {
	return wrapper.get(".mermaid-preview").html()
}

// the mocked composable's refs and the colour mode are shared by the
// whole file, so these tests cannot interleave
describe("<MermaidPreview>", { concurrent: false }, () => {
	// each preview keeps watching the colour mode for as long as it is
	// mounted, so a leftover one would redraw during the next test
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		// the debounce tests leave the clock frozen behind them
		vi.useRealTimers()
		mermaid.render.mockReset()
		mermaid.render.mockResolvedValue({ svg: "<svg><g></g></svg>" })
		mermaid.isLoading.value = false
		mermaid.loadError.value = null
		useAppearance().changeColorTheme("light")
	})

	it("reports that the diagram engine is still loading", async ({ expect }) => {
		mermaid.isLoading.value = true

		const wrapper = await mountPreview("")

		expect(wrapper.text()).toContain(t("editor.mermaid.preview.loading"))
	})

	it("reports a diagram engine that failed to load", async ({ expect }) => {
		mermaid.loadError.value = "network down"

		const wrapper = await mountPreview("")

		expect(wrapper.text()).toContain(t("editor.mermaid.preview.load-error"))
		expect(wrapper.text()).toContain("Network down")
	})

	it("invites the reader to write code while the block is empty", async ({
		expect,
	}) => {
		const wrapper = await mountPreview("   ")

		expect(wrapper.text()).toContain(t("editor.mermaid.preview.empty"))
		expect(mermaid.render).toHaveBeenCalledTimes(0)
	})

	it("renders the diagram the code describes", async ({ expect }) => {
		const wrapper = await mountPreview("graph TD; A-->B;")

		await vi.waitFor(() => {
			expect(previewHtml(wrapper)).toContain("<svg>")
		}, WAIT_FOR_OPTIONS)
		expect(mermaid.render).toHaveBeenCalledTimes(1)
		expect(mermaid.render.mock.calls[0]?.[1]).toBe("graph TD; A-->B;")
	})

	it("strips scripting out of the rendered diagram", async ({ expect }) => {
		mermaid.render.mockResolvedValue({
			svg: "<svg><text>kept</text><script>alert(1)</script></svg>",
		})

		const wrapper = await mountPreview("graph TD; A-->B;")

		await vi.waitFor(() => {
			expect(previewHtml(wrapper)).toContain("kept")
		}, WAIT_FOR_OPTIONS)
		expect(previewHtml(wrapper)).not.toContain("alert(1)")
	})

	it("expands tabs in the code before rendering it", async ({ expect }) => {
		const wrapper = await mountPreview("graph TD;\n\tA-->B;")

		await vi.waitFor(() => {
			expect(mermaid.render).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		expect(mermaid.render.mock.calls[0]?.[1]).toBe("graph TD;\n    A-->B;")
		expect(wrapper.exists()).toBe(true)
	})

	it("reports a diagram the engine could not draw", async ({ expect }) => {
		mermaid.render.mockResolvedValue({ error: "syntax error in graph" })

		const wrapper = await mountPreview("graph TD; A-->")

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain(t("editor.mermaid.preview.render-error"))
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.text()).toContain("Syntax error in graph")
	})

	it("stays hidden until the first diagram is drawn", async ({ expect }) => {
		let resolveRender: (value: Record<string, string>) => void = () => undefined
		mermaid.render.mockReturnValue(
			new Promise((resolve) => {
				resolveRender = resolve
			}),
		)

		const wrapper = await mountPreview("graph TD; A-->B;")

		expect(wrapper.get(".mermaid-preview").classes()).toContain("opacity-0")

		resolveRender({ svg: "<svg><g></g></svg>" })
		await vi.waitFor(() => {
			expect(wrapper.get(".mermaid-preview").classes()).toContain("opacity-100")
		}, WAIT_FOR_OPTIONS)
	})

	it("shows an empty block right away", async ({ expect }) => {
		const wrapper = await mountPreview("")

		expect(wrapper.get(".mermaid-preview").classes()).toContain("opacity-100")
	})

	it("redraws the diagram once the code settles", async ({ expect }) => {
		const wrapper = await mountPreview("graph TD; A-->B;")
		await vi.waitFor(() => {
			expect(mermaid.render).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		vi.useFakeTimers()

		await wrapper.setProps({ source: "graph TD; A-->C;" })
		await vi.advanceTimersByTimeAsync(DEBOUNCE_MS)

		expect(mermaid.render).toHaveBeenCalledTimes(2)
		expect(mermaid.render.mock.calls[1]?.[1]).toBe("graph TD; A-->C;")
	})

	it("leaves the diagram alone while the code is still being typed", async ({
		expect,
	}) => {
		const wrapper = await mountPreview("graph TD; A-->B;")
		await vi.waitFor(() => {
			expect(mermaid.render).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		vi.useFakeTimers()

		await wrapper.setProps({ source: "graph TD; A-->C;" })
		await vi.advanceTimersByTimeAsync(DEBOUNCE_MS - 1)

		expect(mermaid.render).toHaveBeenCalledTimes(1)
	})

	it("clears the diagram when the code is deleted", async ({ expect }) => {
		const wrapper = await mountPreview("graph TD; A-->B;")
		await vi.waitFor(() => {
			expect(previewHtml(wrapper)).toContain("<svg>")
		}, WAIT_FOR_OPTIONS)
		vi.useFakeTimers()

		await wrapper.setProps({ source: "" })
		await vi.advanceTimersByTimeAsync(DEBOUNCE_MS)

		expect(wrapper.text()).toContain(t("editor.mermaid.preview.empty"))
		expect(mermaid.render).toHaveBeenCalledTimes(1)
	})

	it("keeps only the newest diagram when two renders overlap", async ({
		expect,
	}) => {
		const pending: ((value: Record<string, string>) => void)[] = []
		mermaid.render.mockImplementation(
			() =>
				new Promise((resolve) => {
					pending.push(resolve)
				}),
		)
		const wrapper = await mountPreview("graph TD; A-->B;")
		await vi.waitFor(() => {
			expect(pending).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		vi.useFakeTimers()
		await wrapper.setProps({ source: "graph TD; A-->C;" })
		await vi.advanceTimersByTimeAsync(DEBOUNCE_MS)

		pending[1]?.({ svg: '<svg><g id="second"></g></svg>' })
		await vi.advanceTimersByTimeAsync(0)
		pending[0]?.({ svg: '<svg><g id="first"></g></svg>' })
		await vi.advanceTimersByTimeAsync(0)

		expect(previewHtml(wrapper)).toContain("second")
		expect(previewHtml(wrapper)).not.toContain("first")
	})

	it("redraws the diagram in the reader's new colour theme", async ({
		expect,
	}) => {
		// the colour mode is read out of a cookie the composable re-reads
		// per instance, so a change made from the test would never reach a
		// mounted component. Following the system theme instead gives a
		// switch the mounted preview does see.
		const switchToDark = followSystemTheme()
		const wrapper = await mountPreview("graph TD; A-->B;")
		await vi.waitFor(() => {
			expect(mermaid.render).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)

		switchToDark()

		await vi.waitFor(() => {
			expect(mermaid.render).toHaveBeenCalledTimes(2)
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.exists()).toBe(true)
	})
})
