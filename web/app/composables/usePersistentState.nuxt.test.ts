import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, vi } from "vitest"
import { nextTick } from "vue"
import usePersistentState from "./usePersistentState"

const { useDetectHostMock } = vi.hoisted(() => {
	// plain value objects instead of vue refs: the default implementation
	// runs during nuxt bootstrap (the 01.process-user-agent plugin), where
	// only property reads happen
	return {
		useDetectHostMock: vi.fn((): any => ({
			platformType: { value: "web" },
			isWeb: { value: true },
			isDesktop: { value: false },
			osType: { value: "other" },
			browserType: { value: "non-browser" },
			setOsType: () => undefined,
			setBrowserType: () => undefined,
		})),
	}
})

mockNuxtImport("useDetectHost", () => useDetectHostMock)

function arrange(isWeb: boolean) {
	useDetectHostMock.mockReturnValue({ isWeb: { value: isWeb } })
}

// the tests arrange a shared module-level mock (mockNuxtImport singleton),
// so they cannot interleave. Every test uses its own storage key so no
// persisted value leaks between tests.
describe("usePersistentState", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	// — reset the module-level mock explicitly
	beforeEach(() => {
		useDetectHostMock.mockReset()
	})

	it("returns the default value when nothing is stored", ({ expect }) => {
		arrange(true)

		const state = usePersistentState({
			key: "ups-default",
			defaultValue: "fallback",
		})

		expect(state.value).toBe("fallback")
	})

	it("computes the default from a factory function", ({ expect }) => {
		arrange(true)

		const state = usePersistentState({
			key: "ups-factory",
			defaultValue: () => ({ nested: true }),
		})

		expect(state.value).toEqual({ nested: true })
	})

	it("shares written state with later readers of the same key", async ({
		expect,
	}) => {
		arrange(true)
		const writer = usePersistentState({
			key: "ups-shared",
			defaultValue: "initial",
		})

		writer.value = "updated"
		// the cookie write is flushed by a pre-flush watcher
		await nextTick()
		const reader = usePersistentState({
			key: "ups-shared",
			defaultValue: "initial",
		})

		expect(reader.value).toBe("updated")
	})

	it("reads an existing cookie value", ({ expect }) => {
		arrange(true)
		document.cookie = "ups-existing=hello"

		const state = usePersistentState({
			key: "ups-existing",
			defaultValue: "fallback",
		})

		expect(state.value).toBe("hello")
	})

	it("decodes a cookie with a custom serializer", ({ expect }) => {
		arrange(true)
		document.cookie = "ups-serialized=5"

		const state = usePersistentState({
			key: "ups-serialized",
			defaultValue: 0,
			serializer: {
				read: (raw) => Number(raw) * 2,
				write: (value) => String(value),
			},
		})

		expect(state.value).toBe(10)
	})

	it("falls back to the default when the cookie serializer throws", ({
		expect,
	}) => {
		arrange(true)
		document.cookie = "ups-broken=garbage"

		const state = usePersistentState({
			key: "ups-broken",
			defaultValue: 7,
			serializer: {
				read: () => {
					throw new Error("bad payload")
				},
				write: (value) => String(value),
			},
		})

		expect(state.value).toBe(7)
	})

	it("uses local storage when configured on the web", async ({ expect }) => {
		arrange(true)

		const state = usePersistentState({
			key: "ups-local",
			defaultValue: "initial",
			storage: { web: "local-storage" },
		})
		state.value = "persisted"
		await nextTick()

		expect(localStorage.getItem("ups-local")).toBe("persisted")
	})

	it("falls back to the default when the local-storage serializer throws", ({
		expect,
	}) => {
		arrange(true)
		localStorage.setItem("ups-local-broken", "garbage")

		const state = usePersistentState({
			key: "ups-local-broken",
			defaultValue: 7,
			storage: { web: "local-storage" },
			serializer: {
				read: () => {
					throw new Error("bad payload")
				},
				write: (value) => String(value),
			},
		})

		expect(state.value).toBe(7)
	})

	it("defaults to local storage on desktop", async ({ expect }) => {
		arrange(false)

		const state = usePersistentState({
			key: "ups-desktop",
			defaultValue: "initial",
		})
		state.value = "persisted"
		await nextTick()

		expect(localStorage.getItem("ups-desktop")).toBe("persisted")
	})
})
