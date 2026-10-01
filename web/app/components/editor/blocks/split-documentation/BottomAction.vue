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

const { t } = useI18n({ useScope: "global" })

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
		<ShortcutTooltip
			v-if="props.buttons.length === 1"
			side="bottom"
			:shortcut="props.buttons[0]?.shortcut"
		>
			<ShadcnUiButton
				variant="dim"
				size="custom"
				class="gap-1 text-2sm opacity-0 transition-opacity group-hover/right-side-extra-action:opacity-100 supports-[hover:none]:opacity-100"
				@click="handleClick(props.buttons[0]!.id)"
			>
				<Icon :name="props.buttons[0]?.icon ?? 'lucide:circle-plus'" />
				{{ props.buttons[0]?.text }}
			</ShadcnUiButton>
		</ShortcutTooltip>
		<ShadcnUiDropdownMenu v-else>
			<ShadcnUiDropdownMenuTrigger as-child>
				<ShadcnUiButton
					variant="dim"
					size="custom"
					class="gap-1 text-2sm opacity-0 transition-opacity group-hover/right-side-extra-action:opacity-100 supports-[hover:none]:opacity-100"
				>
					<Icon name="lucide:circle-plus" />
					{{ t("editor.split-documentation.bottom-action-trigger-label") }}
				</ShadcnUiButton>
			</ShadcnUiDropdownMenuTrigger>
			<ShadcnUiDropdownMenuContent side="bottom" align="center">
				<ShadcnUiDropdownMenuItem
					v-for="button in props.buttons"
					:key="button.id"
					@click="handleClick(button.id)"
				>
					<Icon :name="button.icon ?? 'lucide:circle-plus'" />
					<span>{{ button.text }}</span>
				</ShadcnUiDropdownMenuItem>
			</ShadcnUiDropdownMenuContent>
		</ShadcnUiDropdownMenu>
	</div>
</template>
