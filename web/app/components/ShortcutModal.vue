<script lang="ts" setup>
const open = defineModel<boolean>({ default: false })

const { osType } = useDetectHost()
</script>
<template>
	<ShadcnUiDialog :open="open" @update:open="(val) => (open = val)">
		<ShadcnUiDialogContent
			class="max-h-[85dvh] w-[90dvw] overflow-y-auto p-0 text-foreground outline-none md:max-h-[70dvh] lg:w-176"
			@open-auto-focus.prevent
			@close-auto-focus.prevent
		>
			<div class="flex flex-col gap-8 p-6">
				<ShadcnUiDialogHeader>
					<ShadcnUiDialogTitle class="text-2xl">
						{{ $t("shortcuts.modal.title") }}
					</ShadcnUiDialogTitle>
					<ShadcnUiDialogDescription class="sr-only">
						{{ $t("shortcuts.modal.description") }}
					</ShadcnUiDialogDescription>
					<ShadcnUiButton
						variant="ghost-plain"
						class="absolute top-1/2 right-0 -translate-y-1/2 p-0"
						@click="open = false"
					>
						<Icon name="lucide:x" size="1.3rem" />
						<span class="sr-only">
							{{ $t("general.modal-close-screen-reader-hint") }}
						</span>
					</ShadcnUiButton>
				</ShadcnUiDialogHeader>
				<div
					v-for="group in SHORTCUT_GROUPS"
					:key="group.i18nKey"
					class="flex flex-col gap-4"
				>
					<h3 class="text-2base">{{ $t(group.i18nKey) }}</h3>
					<div
						class="flex flex-col divide-y divide-border/70 rounded-md border bg-muted/50 px-4 py-1 dark:divide-border/50"
					>
						<div
							v-for="entry in group.shortcuts"
							:key="entry.i18nKey"
							class="flex items-center justify-between gap-4 py-2.5"
						>
							<div class="flex min-w-0 flex-col gap-0.5">
								<div class="text-2base">{{ $t(entry.i18nKey) }}</div>
								<div class="text-xs text-muted-foreground">
									{{ $t(entry.descriptionI18nKey) }}
								</div>
							</div>
							<ShadcnUiKbdGroup class="shrink-0">
								<template
									v-for="(val, index) in extractShortcutKeys(
										shortcutByOS(entry.action.keyboardKey, osType),
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
					</div>
				</div>
			</div>
		</ShadcnUiDialogContent>
	</ShadcnUiDialog>
</template>
