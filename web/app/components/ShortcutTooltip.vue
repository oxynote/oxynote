<script lang="ts" setup>
import { cn } from "~/lib/utils"

const props = defineProps<{
	shortcut?: {
		keyboardKey: { macOS: string; other: string }
		i18nKey: string | null
	}
	side?: "top" | "right" | "bottom" | "left"
	align?: "start" | "center" | "end"
	class?: string
}>()
const { osType } = useDetectHost()

const isOpen = ref(false)
let hoverTimeout: ReturnType<typeof setTimeout> | null = null

function handlePointerEnter() {
	hoverTimeout = setTimeout(() => {
		isOpen.value = true
	}, 800)
}

function handlePointerLeave() {
	if (hoverTimeout) {
		clearTimeout(hoverTimeout)
		hoverTimeout = null
	}

	isOpen.value = false
}
</script>
<template>
	<ShadcnUiTooltip v-if="props.shortcut" :open="isOpen">
		<ShadcnUiTooltipTrigger
			as-child
			@pointerenter="handlePointerEnter"
			@pointerleave="handlePointerLeave"
		>
			<slot />
		</ShadcnUiTooltipTrigger>
		<ShadcnUiTooltipContent
			:side="props.side"
			:align="props.align"
			:class="cn('px-2 py-1', props.class)"
		>
			<div class="flex items-center justify-between gap-2 text-2sm">
				<p v-if="props.shortcut.i18nKey">
					{{ $t(props.shortcut.i18nKey) }}
				</p>
				<ShadcnUiKbdGroup>
					<template
						v-for="(val, index) in extractShortcutKeys(
							shortcutByOS(props.shortcut.keyboardKey, osType),
						)"
						:key="index"
					>
						<ShadcnUiKbd v-if="!val.connector">{{ val.key }}</ShadcnUiKbd>
						<span v-else class="text-xs text-muted-foreground">
							{{ $t("shortcuts.connector") }}
						</span>
					</template>
				</ShadcnUiKbdGroup>
			</div>
		</ShadcnUiTooltipContent>
	</ShadcnUiTooltip>
	<slot v-else />
</template>
