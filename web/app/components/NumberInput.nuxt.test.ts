import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { DOMWrapper } from "@vue/test-utils"
import { describe, it } from "vitest"
import NumberInput from "./NumberInput.vue"

async function mountInput(props: Record<string, unknown> = {}) {
	const wrapper = await mountSuspended(NumberInput, { props: props })

	return {
		wrapper,
		input: wrapper.get("input"),
		// the latest value the component pushed into its v-model, or
		// undefined when it never pushed one
		model: () => {
			const updates = wrapper.emitted("update:modelValue")

			return updates?.[updates.length - 1]?.[0]
		},
	}
}

// drives the input the way a user does: the DOM value changes, then the
// component's input handler runs
async function type(
	input: Omit<DOMWrapper<HTMLInputElement>, "exists">,
	value: string,
) {
	await input.setValue(value)
	await nextTick()
}

describe("<NumberInput>", () => {
	it("shows the model value in the input", async ({ expect }) => {
		const { input } = await mountInput({ modelValue: 42 })

		expect(input.element.value).toBe("42")
	})

	it("shows an empty input when the model is undefined", async ({ expect }) => {
		const { input } = await mountInput({ modelValue: undefined })

		expect(input.element.value).toBe("")
	})

	it("follows the model when it changes from the outside", async ({
		expect,
	}) => {
		const { wrapper, input } = await mountInput({ modelValue: 1 })

		await wrapper.setProps({ modelValue: 7 })

		expect(input.element.value).toBe("7")
	})

	it("reports the typed number through the model", async ({ expect }) => {
		const { input, model } = await mountInput()

		await type(input, "12")

		expect(model()).toBe(12)
	})

	it("clears the model when the input is emptied", async ({ expect }) => {
		const { input, model } = await mountInput({ modelValue: 5 })

		await type(input, "")

		expect(model()).toBeUndefined()
	})

	it("holds a half-typed decimal without reporting it", async ({ expect }) => {
		const { input, model } = await mountInput({ decimal: true })

		await type(input, "1.")

		expect(input.element.value).toBe("1.")
		expect(model()).toBeUndefined()
	})

	it("holds a half-typed negative without reporting it", async ({ expect }) => {
		const { input, model } = await mountInput({ negative: true })

		await type(input, "-")

		expect(input.element.value).toBe("-")
		expect(model()).toBeUndefined()
	})

	it("keeps trailing zeros after the decimal point while typing", async ({
		expect,
	}) => {
		const { input, model } = await mountInput({ decimal: true })

		await type(input, "2.50")

		expect(input.element.value).toBe("2.50")
		expect(model()).toBeUndefined()
	})

	it("raises a value below min up to min", async ({ expect }) => {
		const { input, model } = await mountInput({ min: 10 })

		await type(input, "3")

		expect(model()).toBe(10)
		expect(input.element.value).toBe("10")
	})

	it("lowers a value above max down to max", async ({ expect }) => {
		const { input, model } = await mountInput({ max: 100 })

		await type(input, "150")

		expect(model()).toBe(100)
		expect(input.element.value).toBe("100")
	})

	it("accepts any sign when no sign flag is set", async ({ expect }) => {
		const { input, model } = await mountInput({ negative: false })

		await type(input, "-5")

		expect(model()).toBe(-5)
	})

	it("ignores a negative number when only positives are allowed", async ({
		expect,
	}) => {
		const { input, model } = await mountInput({ positive: true })

		await type(input, "-5")

		expect(model()).toBeUndefined()
	})

	it("ignores zero when only positives are allowed", async ({ expect }) => {
		const { input, model } = await mountInput({ positive: true })

		await type(input, "0")

		expect(model()).toBeUndefined()
	})

	it("accepts zero when zero is allowed", async ({ expect }) => {
		const { input, model } = await mountInput({ positive: true, zero: true })

		await type(input, "0")

		expect(model()).toBe(0)
	})

	it("ignores a positive number when only negatives are allowed", async ({
		expect,
	}) => {
		const { input, model } = await mountInput({ negative: true })

		await type(input, "5")

		expect(model()).toBeUndefined()
	})

	describe("keystroke filtering", () => {
		// beforeinput is the only place the component can reject a keystroke, so
		// the assertion is on whether the event was cancelled
		async function press(
			props: Record<string, unknown>,
			current: string,
			data: string,
		) {
			const wrapper = await mountSuspended(NumberInput, { props: props })
			const input = wrapper.get("input")
			input.element.value = current
			input.element.setSelectionRange(current.length, current.length)

			const event = new InputEvent("beforeinput", {
				data: data,
				cancelable: true,
				bubbles: true,
			})
			input.element.dispatchEvent(event)

			return event.defaultPrevented
		}

		it("accepts a digit", async ({ expect }) => {
			expect(await press({}, "1", "2")).toBe(false)
		})

		it("rejects a letter", async ({ expect }) => {
			expect(await press({}, "1", "a")).toBe(true)
		})

		it("rejects a decimal point when decimals are off", async ({ expect }) => {
			expect(await press({}, "1", ".")).toBe(true)
		})

		it("accepts a decimal point when decimals are on", async ({ expect }) => {
			expect(await press({ decimal: true }, "1", ".")).toBe(false)
		})

		it("rejects a minus sign when only positives are allowed", async ({
			expect,
		}) => {
			expect(await press({ positive: true }, "", "-")).toBe(true)
		})

		it("accepts a minus sign when negatives are allowed", async ({
			expect,
		}) => {
			expect(await press({ negative: true }, "", "-")).toBe(false)
		})

		it("accepts a keystroke that carries no data", async ({ expect }) => {
			expect(await press({}, "1", "")).toBe(false)
		})
	})
})
