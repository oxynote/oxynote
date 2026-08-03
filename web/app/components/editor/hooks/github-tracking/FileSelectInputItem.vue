<script setup lang="ts">
import { cn } from "~/lib/utils"
import type { GitHubFileTreeItem } from "~/utils/api/github"

const props = defineProps<{
	item: GitHubFileTreeItem
	selected: string[] | undefined
	parentItems: GitHubFileTreeItem[]
	emptyFolderPlaceholder: string
}>()
const emit = defineEmits<{
	(
		e: "selected",
		item: GitHubFileTreeItem,
		active: boolean,
		parents: GitHubFileTreeItem[],
	): void
}>()

const itemSelected = computed(() => props.selected?.includes(props.item.name))
const processedName = computed(() => {
	return lastFilePathElement(props.item.name)
})
const subItems = computed(() => {
	if (!props.item.items) {
		return []
	}

	return clone(props.item.items).sort((a, b) => {
		if (a.type === "folder" && b.type !== "folder") return -1
		if (a.type !== "folder" && b.type === "folder") return 1
		return a.name.localeCompare(b.name)
	})
})
const open = ref(false)
</script>
<template>
	<div class="flex w-full flex-col">
		<ShadcnUiCollapsible :open="open">
			<div
				:data-selected="itemSelected ? '' : undefined"
				:class="
					cn(
						'flex cursor-pointer items-center justify-between gap-1 rounded py-0.75 pr-2 pl-1 text-2sm font-medium select-none hover:bg-accent/50 hover:text-accent-foreground active:bg-accent active:text-accent-foreground',
						'hover:[&>*>*>.hide-on-parent-hover]:pointer-events-none hover:[&>*>*>.hide-on-parent-hover]:opacity-0',
						'hover:[&>*>*>.show-on-parent-hover]:pointer-events-auto hover:[&>*>*>.show-on-parent-hover]:opacity-100',
						'data-[selected]:bg-accent/50 data-[selected]:text-accent-foreground data-[selected]:active:bg-accent data-[selected]:active:text-accent-foreground',
						'has-[.show-on-parent-hover:active]:bg-transparent has-[.show-on-parent-hover:active]:text-foreground',
						'has-[.show-on-parent-hover:hover]:bg-transparent has-[.show-on-parent-hover:hover]:text-foreground',
						'has-[.show-on-parent-hover:active]:data-[selected]:active:bg-accent/50 has-[.show-on-parent-hover:active]:data-[selected]:active:text-accent-foreground',
						'has-[.show-on-parent-hover:hover]:data-[selected]:bg-accent/50 has-[.show-on-parent-hover:hover]:data-[selected]:text-accent-foreground',
					)
				"
				@click="emit('selected', props.item, !itemSelected, props.parentItems)"
			>
				<div class="flex min-w-0 items-center gap-1">
					<div
						:class="[
							'relative flex size-5 shrink-0 items-center justify-center rounded-md',
							props.item.type === 'folder'
								? 'hover:bg-accent/50 hover:text-accent-foreground active:bg-accent active:text-accent-foreground'
								: undefined,
						]"
					>
						<Icon
							v-if="props.item.type === 'folder'"
							name="lucide:chevron-right"
							:class="[
								'show-on-parent-hover',
								'pointer-events-none absolute size-3.5 opacity-0 transition-all duration-50',
								'inset-1/2 mt-0.25 -translate-x-1/2 -translate-y-1/2',
								open ? 'rotate-90' : undefined,
							]"
							@click.stop="open = !open"
						/>
						<Icon
							:name="
								props.item.type === 'file'
									? 'mingcute:file-line'
									: open
										? 'mingcute:folder-open-fill'
										: 'mingcute:folder-fill'
							"
							:class="[
								props.item.type === 'folder'
									? 'hide-on-parent-hover'
									: undefined,
								'size-3.5 opacity-100',
							]"
						/>
					</div>
					<div class="truncate">{{ processedName }}</div>
				</div>
				<Icon
					v-if="itemSelected"
					name="mingcute:check-fill"
					class="size-3.5 shrink-0"
				/>
			</div>
			<ShadcnUiCollapsibleContent v-if="item.type === 'folder'" as-child>
				<div :class="['ml-3.25 py-1 pl-2', 'border-l border-border']">
					<div v-if="subItems.length" class="flex flex-col gap-1">
						<FileSelectInputItem
							v-for="subItem in subItems"
							:key="subItem.name"
							:item="subItem"
							:selected="props.selected"
							:empty-folder-placeholder="props.emptyFolderPlaceholder"
							:parent-items="props.parentItems.concat([item])"
							@selected="
								(selItem, active, selParents) =>
									emit('selected', selItem, active, selParents)
							"
						/>
					</div>
					<div
						v-else
						class="truncate pl-2.25 text-2sm font-medium text-foreground"
					>
						{{ props.emptyFolderPlaceholder }}
					</div>
				</div>
			</ShadcnUiCollapsibleContent>
		</ShadcnUiCollapsible>
	</div>
</template>
