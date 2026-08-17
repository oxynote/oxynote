import { describe, it } from "vitest"
import { isReadonly, nextTick, ref } from "vue"
import { useLastValidRef } from "./vue"

describe("useLastValidRef", () => {
	it("starts undefined while the source has no valid value", ({ expect }) => {
		const last = useLastValidRef(ref<string | null>(null))

		expect(last.value).toBeUndefined()
	})

	it("picks up an initial valid value immediately", ({ expect }) => {
		const last = useLastValidRef(ref("first"))

		expect(last.value).toBe("first")
	})

	it("follows the source across valid updates", async ({ expect }) => {
		const source = ref<string | null>("first")
		const last = useLastValidRef(source)

		source.value = "second"
		await nextTick()

		expect(last.value).toBe("second")
	})

	it("keeps the last valid value when the source turns null", async ({
		expect,
	}) => {
		const source = ref<string | null>("first")
		const last = useLastValidRef(source)

		source.value = null
		await nextTick()

		expect(last.value).toBe("first")
	})

	it("keeps the last valid value when the source turns undefined", async ({
		expect,
	}) => {
		const source = ref<string | undefined>("first")
		const last = useLastValidRef(source)

		source.value = undefined
		await nextTick()

		expect(last.value).toBe("first")
	})

	it("returns a readonly ref", ({ expect }) => {
		expect(isReadonly(useLastValidRef(ref("first")))).toBe(true)
	})
})
