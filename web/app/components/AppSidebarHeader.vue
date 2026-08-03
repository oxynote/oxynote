<script lang="ts" setup>
const props = defineProps<{
	workspaceName: string | null | undefined
	avatar: {
		src: string
		alt: string
		initials: string
	}
}>()
const emit = defineEmits<{
	(e: "create-new-item" | "open-settings" | "log-out"): void
}>()

const { changeColorTheme, color } = useAppearance()
</script>

<template>
	<ShadcnUiSidebarMenuItem>
		<ShadcnUiDropdownMenu>
			<ShadcnUiDropdownMenuTrigger as-child>
				<ShadcnUiSidebarMenuButton
					class="h-7.5 cursor-pointer px-1 py-0 text-sm data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
				>
					<ShadcnUiAvatar class="size-6 rounded-md">
						<ShadcnUiAvatarImage
							:src="props.avatar.src"
							:alt="props.avatar.alt"
						/>
						<ShadcnUiAvatarFallback class="rounded-md border">
							{{ props.avatar.initials }}
						</ShadcnUiAvatarFallback>
					</ShadcnUiAvatar>
					<div class="flex items-center gap-0.5">
						<span class="font-medium">
							{{ props.workspaceName || $t("sidebar.default-workspace-name") }}
						</span>
						<Icon name="lucide:chevron-down" class="mt-[0.15rem]" />
					</div>
				</ShadcnUiSidebarMenuButton>
			</ShadcnUiDropdownMenuTrigger>
			<ShadcnUiDropdownMenuContent
				class="w-(--reka-dropdown-menu-trigger-width)"
				align="start"
				inside-sheet
			>
				<ShortcutTooltip
					side="right"
					:shortcut="SHORTCUT_ACTIONS.toggleSettings"
				>
					<ShadcnUiDropdownMenuItem @click="$emit('open-settings')">
						<Icon name="lucide:settings" class="shrink-0" />
						{{ $t("sidebar.header.settings") }}
					</ShadcnUiDropdownMenuItem>
				</ShortcutTooltip>
				<ShadcnUiDropdownMenuSub>
					<ShadcnUiDropdownMenuSubTrigger>
						<Icon name="lucide:palette" class="shrink-0" />
						<span>
							{{ $t("sidebar.header.appearance.title") }}
						</span>
					</ShadcnUiDropdownMenuSubTrigger>
					<ShadcnUiDropdownMenuSubContent side="right" align="start" loop>
						<ShadcnUiDropdownMenuItem @click="changeColorTheme('light')">
							<Icon name="lucide:sun" class="shrink-0" />
							<div class="flex items-center justify-between gap-2">
								{{ $t("sidebar.header.appearance.light") }}
								<Icon
									v-show="!color.system && color.theme === 'light'"
									name="mingcute:check-fill"
								/>
							</div>
						</ShadcnUiDropdownMenuItem>
						<ShadcnUiDropdownMenuItem @click="changeColorTheme('dark')">
							<Icon name="lucide:moon" class="shrink-0" />
							<div class="flex items-center justify-between gap-2">
								{{ $t("sidebar.header.appearance.dark") }}
								<Icon
									v-show="!color.system && color.theme === 'dark'"
									name="mingcute:check-fill"
								/>
							</div>
						</ShadcnUiDropdownMenuItem>
						<ShadcnUiDropdownMenuItem @click="changeColorTheme('auto')">
							<Icon name="lucide:refresh-cw" class="shrink-0" />
							<div class="flex items-center justify-between gap-2">
								{{ $t("sidebar.header.appearance.auto") }}
								<Icon v-show="color.system" name="mingcute:check-fill" />
							</div>
						</ShadcnUiDropdownMenuItem>
					</ShadcnUiDropdownMenuSubContent>
				</ShadcnUiDropdownMenuSub>
				<ShadcnUiDropdownMenuSeparator />
				<ShadcnUiDropdownMenuItem @click="$emit('log-out')">
					<Icon name="lucide:log-out" class="shrink-0" />
					{{ $t("sidebar.header.log-out") }}
				</ShadcnUiDropdownMenuItem>
			</ShadcnUiDropdownMenuContent>
			<ShortcutTooltip
				side="bottom"
				:shortcut="SHORTCUT_ACTIONS.createNewDocument"
			>
				<ShadcnUiSidebarMenuAction
					class="top-[0.18rem]! w-6 border border-sidebar-border bg-background text-foreground"
					@click="emit('create-new-item')"
				>
					<Icon name="lucide:square-pen" />
				</ShadcnUiSidebarMenuAction>
			</ShortcutTooltip>
		</ShadcnUiDropdownMenu>
	</ShadcnUiSidebarMenuItem>
</template>
