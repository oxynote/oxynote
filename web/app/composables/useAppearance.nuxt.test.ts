import { describe, it } from "vitest"
import { nextTick } from "vue"
import useAppearance from "./useAppearance"

// the composable is backed by shared cookie state — the tests cannot
// interleave, and each arranges the mode it asserts
describe("useAppearance", { concurrent: false }, () => {
	it("switches to the dark theme", ({ expect }) => {
		const appearance = useAppearance()

		appearance.changeColorTheme("dark")

		expect(appearance.color.value).toEqual({ system: false, theme: "dark" })
		expect(appearance.isDark.value).toBe(true)
	})

	it("switches to the light theme", ({ expect }) => {
		const appearance = useAppearance()

		appearance.changeColorTheme("light")

		expect(appearance.color.value).toEqual({ system: false, theme: "light" })
		expect(appearance.isDark.value).toBe(false)
	})

	it("follows the system preference in auto mode", ({ expect }) => {
		const appearance = useAppearance()

		appearance.changeColorTheme("auto")

		// happy-dom reports no dark preference, so auto resolves to light
		expect(appearance.color.value).toEqual({ system: true, theme: "light" })
		expect(appearance.isDark.value).toBe(false)
	})

	it("remembers the resolved theme for the ssr fallback", async ({
		expect,
	}) => {
		const appearance = useAppearance()

		appearance.changeColorTheme("dark")
		await nextTick()

		// the watcher mirrors the resolved theme into the cookie that seeds
		// the ssr fallback
		expect(useCookie("last-auto-resolved-color-mode").value).toBe("dark")
	})

	it("overrides a stale dark ssr cookie when the system is in light mode", async({expect}) =>{
		const appearance = useAppearance()

		useCookie("last-auto-resolved-color-mode").value = "dark"
        document.documentElement.classList.add("dark")

		appearance.changeColorTheme("auto")
		await nextTick()

		// the dom should update the value of cookie "last-auto-resolved-color-mode" from dark to light
		expect(appearance.color.value).toEqual({system:true, theme:"light"})
		expect(appearance.isDark.value).toBe(false)
		expect(document.documentElement.classList.contains("dark")).toBe(false)
		expect(useCookie("last-auto-resolved-color-mode").value).toBe("light")
	})
})
