<script lang="ts" setup generic="T extends string">
const props = defineProps<{
	buttons: {
		id: T
		text: string
		icon?: string
		shortcut?: {
			keyboardKey: { macOS: string; other: string }
			i18nKey: string | null
		}
	}[]
}>()
const emit = defineEmits<{
	(e: "button-click", id: T): void
}>()

function handleClick(id: T) {
	emit("button-click", id)
}
</script>

<template>
	<div
		role="presentation"
		class="group/right-side-extra-action absolute -bottom-7 left-0 z-10 flex h-7 w-full cursor-default items-center justify-center gap-3"
		@mousedown="
			(e) => {
				if (e.target === e.currentTarget) {
					// only prevent when the overlay background itself is clicked
					e.stopPropagation()
					e.preventDefault()
				} else {
					// allow children to behave normally (focus, etc.)
					e.stopPropagation()
				}
			}
		"
	>
		<template v-for="(button, index) in props.buttons" :key="button.id">
			<div
				v-if="index > 0"
				class="h-4 w-px bg-accent-foreground/10 opacity-0 transition-opacity group-hover/right-side-extra-action:opacity-100 supports-[hover:none]:opacity-100"
			/>
			<ShortcutTooltip side="bottom" :shortcut="button.shortcut">
				<ShadcnUiButton
					variant="dim"
					size="custom"
					class="gap-1 text-2sm opacity-0 transition-opacity group-hover/right-side-extra-action:opacity-100 supports-[hover:none]:opacity-100"
					@click="handleClick(button.id)"
				>
					<Icon :name="button.icon ?? 'lucide:circle-plus'" />
					{{ button.text }}
				</ShadcnUiButton>
			</ShortcutTooltip>
		</template>
	</div>
</template>
