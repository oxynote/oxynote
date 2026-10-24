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
const { osType } = useDetectHost()

const singleButton = computed(() =>
	props.buttons.length === 1 ? props.buttons[0] : undefined,
)

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
			v-if="singleButton"
			side="bottom"
			:shortcut="singleButton.shortcut"
		>
			<ShadcnUiButton
				variant="dim"
				size="custom"
				class="gap-1 text-2sm opacity-0 transition-opacity group-hover/right-side-extra-action:opacity-100 supports-[hover:none]:opacity-100"
				@click="handleClick(singleButton.id)"
			>
				<Icon :name="singleButton.icon ?? 'lucide:circle-plus'" />
				{{ singleButton.text }}
			</ShadcnUiButton>
		</ShortcutTooltip>
		<ShadcnUiDropdownMenu v-else>
			<ShadcnUiDropdownMenuTrigger as-child>
				<!--
					the open menu is teleported out of the row, so hovering it
					does not keep the row hovered
				-->
				<ShadcnUiButton
					variant="dim"
					size="custom"
					class="gap-1 text-2sm opacity-0 transition-opacity group-hover/right-side-extra-action:opacity-100 data-[state=open]:opacity-100 supports-[hover:none]:opacity-100"
				>
					<Icon name="mingcute:add-circle-line" />
					{{ t("editor.split-documentation.bottom-action-trigger-label") }}
				</ShadcnUiButton>
			</ShadcnUiDropdownMenuTrigger>
			<ShadcnUiDropdownMenuContent side="bottom" align="center" loop>
				<ShadcnUiDropdownMenuItem
					v-for="button in props.buttons"
					:key="button.id"
					@click="handleClick(button.id)"
				>
					<div class="flex w-full items-center gap-1">
						<Icon :name="button.icon ?? 'lucide:circle-plus'" />
						<span>{{ button.text }}</span>
						<ShadcnUiKbdGroup v-if="button.shortcut" class="ml-auto pl-4">
							<template
								v-for="(val, index) in extractShortcutKeys(
									shortcutByOS(button.shortcut.keyboardKey, osType),
								)"
								:key="index"
							>
								<ShadcnUiKbd v-if="!val.connector">{{ val.key }}</ShadcnUiKbd>
								<span
									v-else-if="val.connector === 'sequence'"
									class="text-xs text-muted-foreground"
								>
									{{ t("shortcuts.connector") }}
								</span>
							</template>
						</ShadcnUiKbdGroup>
					</div>
				</ShadcnUiDropdownMenuItem>
			</ShadcnUiDropdownMenuContent>
		</ShadcnUiDropdownMenu>
	</div>
</template>
