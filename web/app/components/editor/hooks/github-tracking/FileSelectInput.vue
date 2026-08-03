<script setup lang="ts">
import { cn } from "~/lib/utils"
import type { GitHubFileTreeItem } from "~/utils/api/github"
import FileSelectInputItem from "./FileSelectInputItem.vue"

const props = withDefaults(
	defineProps<{
		disabled?: boolean
		placeholder: string
		emptyFolderPlaceholder: string
		options: GitHubFileTreeItem[]
		iconClass?: string
		buttonClass?: string
		buttonSize?: "default" | "2sm" | "sm" | "lg" | "custom"
	}>(),
	{
		iconClass: cn("size-3.5"),
		buttonClass: cn("text-2sm px-1.5 py-1.25"),
		buttonSize: "custom",
	},
)
const selected = defineModel<string[] | undefined>()

const opts = computed(() => {
	return clone(props.options).sort((a, b) => {
		if (a.type === "folder" && b.type !== "folder") return -1
		if (a.type !== "folder" && b.type === "folder") return 1
		return a.name.localeCompare(b.name)
	})
})

function applySelection(
	item: GitHubFileTreeItem,
	active: boolean,
	parents?: GitHubFileTreeItem[],
	old?: string[] | undefined,
): string[] {
	if (!old) {
		old = selected.value ? clone(selected.value) : undefined
	}

	if (active) {
		if (!old) {
			old = []
		}

		if (!old.includes(item.name)) {
			old.push(item.name)
		}

		if (item.type === "folder") {
			item.items?.forEach((subItem) => {
				old = applySelection(subItem, true, parents, old)
			})
		}
	} else {
		old = old?.filter((n) => n !== item.name)

		if (item.type === "folder") {
			item.items?.forEach((subItem) => {
				old = applySelection(subItem, false, parents, old)
			})
		}
	}

	return old || []
}

function handleSelection(
	item: GitHubFileTreeItem,
	active: boolean,
	parents: GitHubFileTreeItem[],
): void {
	selected.value = applySelection(item, active, parents)
}
</script>
<template>
	<div class="mt-0.5 flex flex-col">
		<div class="mb-1 text-left text-2sm font-medium text-foreground">
			<slot name="label" />
		</div>
		<ShadcnUiPopover>
			<ShadcnUiPopoverTrigger as-child>
				<ShadcnUiButton
					variant="outline-no-effect"
					:size="props.buttonSize"
					:disabled="props.disabled"
					:class="
						cn(
							'w-full items-center justify-between gap-1 font-normal',
							props.buttonClass,
							!selected?.length && 'text-muted-foreground',
						)
					"
				>
					<div
						class="flex min-w-0 flex-1 flex-col items-start justify-start gap-1 font-normal"
					>
						<div
							v-if="selected?.length"
							class="w-full truncate text-left text-2sm"
						>
							<slot name="selection-text" :count="selected.length" />
						</div>
						<div v-else class="w-full truncate text-left text-2sm">
							{{ props.placeholder }}
						</div>
					</div>
					<Icon name="lucide:chevron-down" class="mt-[0.15rem] size-4" />
				</ShadcnUiButton>
			</ShadcnUiPopoverTrigger>
			<ShadcnUiPopoverContent align="start" side="bottom">
				<div
					class="flex max-h-[45dvh] max-w-[20rem] min-w-[15rem] flex-col gap-1 overflow-y-auto"
				>
					<FileSelectInputItem
						v-for="item in opts"
						:key="item.name"
						:item="item"
						:selected="selected"
						:parent-items="[]"
						:empty-folder-placeholder="props.emptyFolderPlaceholder"
						@selected="handleSelection"
					/>
				</div>
			</ShadcnUiPopoverContent>
		</ShadcnUiPopover>
	</div>
</template>
