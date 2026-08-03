<script lang="ts" setup>
const props = withDefaults(
	defineProps<{
		buttonText: string
		buttonIcon?: string
		buttonShortcut?: {
			keyboardKey: { macOS: string; other: string }
			i18nKey: string | null
		}
		secondButtonText?: string
		secondButtonIcon?: string
		secondButtonShortcut?: {
			keyboardKey: { macOS: string; other: string }
			i18nKey: string | null
		}
	}>(),
	{
		buttonIcon: "lucide:circle-plus",
		secondButtonIcon: "lucide:circle-plus",
	},
)
const emit = defineEmits<{
	(e: "button-click", btn: "first" | "second"): void
}>()

function handleClick(btn: "first" | "second") {
	emit("button-click", btn)
}
</script>

<template>
	<div
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
		<ShortcutTooltip side="bottom" :shortcut="props.buttonShortcut">
			<ShadcnUiButton
				variant="dim"
				size="custom"
				class="gap-1 text-2sm opacity-0 transition-opacity group-hover/right-side-extra-action:opacity-100 supports-[hover:none]:opacity-100"
				@click="handleClick('first')"
			>
				<Icon :name="props.buttonIcon" />
				{{ props.buttonText }}
			</ShadcnUiButton>
		</ShortcutTooltip>
		<template v-if="props.secondButtonText">
			<div
				class="h-4 w-px bg-accent-foreground/10 opacity-0 transition-opacity group-hover/right-side-extra-action:opacity-100 supports-[hover:none]:opacity-100"
			/>
			<ShortcutTooltip side="bottom" :shortcut="props.secondButtonShortcut">
				<ShadcnUiButton
					variant="dim"
					size="custom"
					class="gap-1 text-2sm opacity-0 transition-opacity group-hover/right-side-extra-action:opacity-100 supports-[hover:none]:opacity-100"
					@click="handleClick('second')"
				>
					<Icon :name="props.secondButtonIcon" />
					{{ props.secondButtonText }}
				</ShadcnUiButton>
			</ShortcutTooltip>
		</template>
	</div>
</template>
