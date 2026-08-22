<script setup lang="ts">
import { compareAsc } from "date-fns"
import { cn } from "~/lib/utils"

export interface IconMetadata {
	id: string
	name: string
	url?: string
	icon?: string
	approved?: boolean
	staleHook?: boolean
	hookUpdatedAt?: Date | string
}

const props = defineProps<{
	title: string
	default?: string
	icons: IconMetadata[]
	disabled?: boolean
	clickable?: boolean
}>()

const visible = computed(() => {
	const icons = props.icons
	return icons
		.sort((a, b) => {
			if (a.staleHook && !b.staleHook) {
				return -1
			} else if (!a.staleHook && b.staleHook) {
				return 1
			}

			if (a.hookUpdatedAt && b.hookUpdatedAt) {
				return compareAsc(a.hookUpdatedAt, b.hookUpdatedAt)
			}

			return 0
		})
		.slice(0, 3)
})
const extra = computed(() => Math.max(0, props.icons.length - 3))
</script>

<template>
	<div
		class="flex items-center gap-1"
		:class="props.clickable ? 'cursor-pointer' : undefined"
	>
		<span class="text-sm font-medium text-muted-foreground">
			{{ title }}
		</span>
		<ul class="flex -space-x-2">
			<li v-for="(im, key) in visible" :key="key" class="relative">
				<ShadcnUiTooltip>
					<ShadcnUiTooltipTrigger as-child>
						<div
							:class="
								cn(
									// the wrapper is needed to prevent jumpiness due to border change
									'flex items-center justify-center rounded-full border border-background',
									im.approved && 'border-status-success',
								)
							"
						>
							<ShadcnUiAvatar
								:data-disabled="props.disabled ? '' : undefined"
								:class="
									cn(
										'size-6 cursor-pointer border data-disabled:cursor-default',
										im.approved && 'border-status-success',
									)
								"
							>
								<ShadcnUiAvatarImage
									v-if="im.url"
									:src="im.url"
									:alt="$t('settings.workspace.logo-alt')"
								/>
								<ShadcnUiAvatarFallback v-if="im.icon">
									<Icon
										:name="im.icon"
										:data-hook-stale="im.staleHook ? 'stale' : undefined"
										class="size-3.5"
									/>
								</ShadcnUiAvatarFallback>
								<ShadcnUiAvatarFallback v-else class="rounded-md text-2xs">
									{{ extractInitials(im.name || "", 2) }}
								</ShadcnUiAvatarFallback>
							</ShadcnUiAvatar>
						</div>
					</ShadcnUiTooltipTrigger>
					<ShadcnUiTooltipContent class="px-2 py-1">
						{{ im.name }}
					</ShadcnUiTooltipContent>
				</ShadcnUiTooltip>
				<div
					v-if="im.approved"
					:class="
						cn(
							'absolute -bottom-0.25 flex size-3 items-center justify-center rounded-full bg-status-success',
							visible.length > 1 ? '-left-0.75' : '-right-0.75',
						)
					"
				>
					<Icon name="lucide:check" class="size-2 text-white" />
				</div>
			</li>
			<li v-if="!props.icons.length">
				<ShadcnUiAvatar class="size-6 border">
					<ShadcnUiAvatarFallback>
						<Icon :name="props.default ?? 'lucide:plus'" class="size-3.5" />
					</ShadcnUiAvatarFallback>
				</ShadcnUiAvatar>
			</li>
			<li v-if="extra > 0">
				<ShadcnUiAvatar class="size-6 border">
					<ShadcnUiAvatarFallback>
						<i18n-t
							scope="global"
							keypath="editor.icon-stack.more-indicator"
							tag="div"
							class="items-center justify-center text-xs font-normal"
						>
							<template #count>{{ extra }}</template>
						</i18n-t>
					</ShadcnUiAvatarFallback>
				</ShadcnUiAvatar>
			</li>
		</ul>
	</div>
</template>
