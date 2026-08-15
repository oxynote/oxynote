<script lang="ts" setup>
import type { InputProps } from "./shadcn/ui/input"

const props = defineProps<
	InputProps & {
		positive?: boolean
		negative?: boolean
		zero?: boolean
		decimal?: boolean
		min?: number
		max?: number
	}
>()
const model = defineModel<number | undefined>()

const { modelValue, ...restProps } = props
const localValue = ref<string | undefined>(undefined)

const { ignoreUpdates: ignoreLocalValueUpdates } = watchIgnorable(
	localValue,
	(newVal) => {
		if (newVal === undefined) {
			model.value = undefined
			return
		}

		// we need to handle negative/decimal sign input separately to allow user to
		// type it - return current value to prevent any change.
		// also preserve trailing zeros after decimal (e.g. "0.0", "1.00", "2.50")
		if (
			newVal.endsWith(".") ||
			newVal === "-" ||
			newVal === "-0" ||
			/\.\d*0$/.test(newVal)
		) {
			return
		}

		let num = Number(newVal)
		if (Number.isFinite(num) && isAllowed(num)) {
			if (props.min !== undefined && num < props.min) {
				num = props.min
			}

			if (props.max !== undefined && num > props.max) {
				num = props.max
			}

			model.value = num

			if (num.toString() !== newVal) {
				ignoreLocalValueUpdates(() => {
					localValue.value = num.toString()
				})
			}
		}
	},
)

watchImmediate(model, (newVal) => {
	ignoreLocalValueUpdates(() => {
		localValue.value = newVal !== undefined ? newVal.toString() : undefined
	})
})

function thresholdBeforeInput(e: InputEvent) {
	if (!e.data) {
		return
	}

	const input = e.target as HTMLInputElement
	const newValue =
		input.value.slice(0, input.selectionStart ?? 0) +
		e.data +
		input.value.slice(input.selectionEnd ?? 0)

	const pattern =
		props.positive && !props.negative
			? props.decimal
				? /^\d*\.?\d*$/
				: /^\d*$/
			: props.decimal
				? /^-?\d*\.?\d*$/
				: /^-?\d*$/
	if (!pattern.test(newValue)) {
		e.preventDefault()
	}
}

function isAllowed(num: number): boolean {
	if (num > 0) {
		return props.positive
	} else if (num < 0) {
		return props.negative
	}

	return props.zero
}

function thresholdValueInput(e: Event) {
	const input = e.target as HTMLInputElement
	const value = input.value
	if (value === "") {
		localValue.value = undefined
		return
	}

	if (
		value.endsWith(".") ||
		value === "-" ||
		value === "-0" ||
		/\.\d*0$/.test(value)
	) {
		localValue.value = value
		return
	}

	let num = Number(value)
	if (Number.isFinite(num) && isAllowed(num)) {
		if (props.min !== undefined && num < props.min) {
			num = props.min
		}
		if (props.max !== undefined && num > props.max) {
			num = props.max
		}

		const clamped = num.toString()
		input.value = clamped
		localValue.value = clamped
	}
}
</script>
<template>
	<ShadcnUiInput
		v-model="localValue"
		v-bind="restProps"
		inputmode="numeric"
		@beforeinput="thresholdBeforeInput"
		@input="thresholdValueInput"
	/>
</template>
