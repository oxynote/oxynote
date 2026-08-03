import type { HTMLAttributes } from "vue"

export { default as Input } from "./Input.vue"

export interface InputProps {
	defaultValue?: string | number
	modelValue?: string | number
	class?: HTMLAttributes["class"]
	disableFocusEffect?: boolean
	disableDestructiveEffect?: boolean
	transparentDisable?: boolean // disable without any visual change
	ghost?: boolean
	placeholder?: string
	type?: string
}
