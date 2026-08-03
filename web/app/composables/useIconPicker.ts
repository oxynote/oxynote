import type { Ref } from "vue"

interface IconPickerContext {
	open: Ref<boolean>
	anchor: Ref<HTMLElement | null>
	openIconPicker: (
		anchor: HTMLElement,
		onSelect: (icon: string) => void,
	) => void
	closeIconPicker: () => void
	selectIcon: (icon: string) => void
}

const ICON_PICKER_KEY = Symbol("icon-picker")

export default function () {
	const existing = inject(ICON_PICKER_KEY, null)
	if (existing) {
		return existing
	}

	const open = ref(false)
	const anchor = ref<HTMLElement | null>(null)
	let onSelect: ((icon: string) => void) | null = null

	function openIconPicker(
		anchorEl: HTMLElement,
		callback: (icon: string) => void,
	) {
		anchor.value = anchorEl
		onSelect = callback
		open.value = true
	}

	function closeIconPicker() {
		open.value = false
	}

	function selectIcon(icon: string) {
		onSelect?.(icon)
		closeIconPicker()
	}

	const context: IconPickerContext = {
		open,
		anchor,
		openIconPicker,
		closeIconPicker,
		selectIcon,
	}

	provide(ICON_PICKER_KEY, context)

	return context
}
