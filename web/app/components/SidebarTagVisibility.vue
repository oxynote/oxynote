<script lang="ts" setup>
import type { TagTreeElement } from "~/utils"

const props = defineProps<{
	tags: TagTreeElement[]
}>()
const emit = defineEmits<{
	(e: "toggle", tag: TagTreeElement): void
}>()
</script>

<template>
	<ShadcnUiDropdownMenu>
		<ShadcnUiDropdownMenuTrigger as-child>
			<ShadcnUiSidebarGroupAction>
				<Icon name="lucide:list-filter" />
				<span class="sr-only">
					{{ $t("sidebar.sections.tags.visibility-action-title") }}
				</span>
			</ShadcnUiSidebarGroupAction>
		</ShadcnUiDropdownMenuTrigger>
		<ShadcnUiDropdownMenuContent side="bottom" align="start" loop inside-sheet>
			<ShadcnUiDropdownMenuItem
				v-for="tag in props.tags"
				:key="tag.id"
				:value="tag.id"
				:active="!tag.hidden"
				@select="(event: Event) => event.preventDefault()"
				@click="emit('toggle', tag)"
			>
				<div class="flex min-w-0 flex-1 items-center gap-2">
					<span
						class="size-2.5 shrink-0 rounded-full"
						:style="{ backgroundColor: tag.color }"
					/>
					<span class="min-w-0 truncate whitespace-nowrap">
						{{ tag.tagName }}
					</span>
				</div>
			</ShadcnUiDropdownMenuItem>
		</ShadcnUiDropdownMenuContent>
	</ShadcnUiDropdownMenu>
</template>
