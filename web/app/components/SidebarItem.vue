<script lang="ts" setup>
import type { HTMLAttributes } from "vue"
import { SIDEBAR_ITEM_PLACEHOLDER_ID, type SidebarItem } from "./sidebar"
import { cn } from "@/lib/utils"

const props = defineProps<{
	parentId: string | null
	item: SidebarItem
	wrapper: HTMLElement | null | undefined
	active?: boolean
	open?: boolean
	ghost?: boolean
	class?: HTMLAttributes["class"]
}>()
const emit = defineEmits<{
	(e: "toggle-collapse" | "delete" | "create" | "duplicate"): void
	(
		e: "update-location",
		data: {
			parentId: string | null
			insertBeforeId: string | null
		},
	): void
}>()

const itemElem = useTemplateRef("sidebar-item")
const itemTopEdgeElem = useTemplateRef("sidebar-item-top-edge")
const {
	isItemDraggedOn,
	isTopEdgeDraggedOn,
	isDraggingGlobal,
	isDraggingSelf,
	draggedGhostStyle,
} = useSidebarDraggable(
	itemElem,
	() => {
		return {
			...props.item,
			parentId: props.parentId,
		}
	},
	itemTopEdgeElem,
	() => props.wrapper,
	(data) => {
		emit("update-location", {
			parentId: data.targetParentId,
			insertBeforeId: data.targetInsertBeforeId,
		})
	},
	{
		disabled: () => !props.item.draggable,
		minDistance: 5,
	},
)
</script>

<template>
	<div
		ref="sidebar-item"
		:data-dragged-on="isItemDraggedOn ? '' : undefined"
		:data-dragging-global="
			props.item.draggable && props.item.partOfDocumentTree
				? isDraggingGlobal
				: undefined
		"
		:data-ghost="props.ghost ? '' : undefined"
		:class="cn('group/sidebar-item', props.class)"
	>
		<Teleport v-if="!props.ghost && isDraggingSelf" to="body">
			<ShadcnUiSidebarMenuItem
				:style="draggedGhostStyle"
				class="pointer-events-none fixed z-draggable list-none opacity-70"
			>
				<SidebarItem
					v-bind="{
						...props,
						ghost: true,
					}"
				/>
			</ShadcnUiSidebarMenuItem>
		</Teleport>
		<div
			:class="
				cn(
					'relative h-full w-full group-data-[dragging-global=false]/sidebar-item:hover:[&>.hide-on-parent-hover]:pointer-events-none group-data-[dragging-global=false]/sidebar-item:hover:[&>.hide-on-parent-hover]:opacity-0 group-data-[dragging-global=false]/sidebar-item:hover:[&>.show-on-parent-hover]:pointer-events-auto group-data-[dragging-global=false]/sidebar-item:hover:[&>.show-on-parent-hover]:opacity-100',
				)
			"
		>
			<div
				v-if="props.item.draggable"
				ref="sidebar-item-top-edge"
				class="absolute -top-1.5 z-5 flex h-3 w-full items-center"
			>
				<div
					v-show="isDraggingGlobal && isTopEdgeDraggedOn"
					class="h-1 w-full bg-drag-target"
				/>
			</div>
			<ShadcnUiSidebarMenuAction
				v-if="props.item.icon"
				:class="[
					'hide-on-parent-hover pointer-events-auto',
					'opacity-100 transition-opacity duration-50',
				]"
				side="left"
				:disable-direct-interaction="!props.item.partOfDocumentTree"
			>
				<Icon :name="props.item.icon" />
			</ShadcnUiSidebarMenuAction>
			<ShadcnUiSidebarMenuAction
				v-if="props.item.icon && props.item.partOfDocumentTree"
				class="show-on-parent-hover pointer-events-none opacity-0 transition-opacity duration-50"
				side="left"
				@click="emit('toggle-collapse')"
			>
				<Icon
					:class="['transition-all', props.open ? 'rotate-90' : undefined]"
					name="lucide:chevron-right"
				/>
				<span class="sr-only">
					{{
						props.open
							? $t(
									"sidebar.item-collapse-trigger-button.close-screen-reader-hint",
								)
							: $t(
									"sidebar.item-collapse-trigger-button.open-screen-reader-hint",
								)
					}}
				</span>
			</ShadcnUiSidebarMenuAction>
			<ShadcnUiSidebarMenuButton
				size="md"
				as-child
				:data-placeholder-variant="
					props.item.id === SIDEBAR_ITEM_PLACEHOLDER_ID
				"
				:is-active="props.active"
				:variant="
					props.item.id === SIDEBAR_ITEM_PLACEHOLDER_ID
						? 'placeholder'
						: 'default'
				"
				:class="
					cn(
						'flex items-center gap-1 group-data-dragged-on/sidebar-item:data-[placeholder-variant=false]:bg-drag-target/40 group-data-dragged-on/sidebar-item:data-[placeholder-variant=false]:hover:bg-drag-target/40 dark:group-data-dragged-on/sidebar-item:data-[placeholder-variant=false]:bg-drag-target dark:group-data-dragged-on/sidebar-item:data-[placeholder-variant=false]:hover:bg-drag-target',
						'group-data-ghost/sidebar-item:bg-sidebar-accent group-data-ghost/sidebar-item:text-sidebar-accent-foreground',
						'group-data-ghost/sidebar-item:data-[active=true]:bg-sidebar-accent group-data-ghost/sidebar-item:data-[active=true]:text-sidebar-accent-foreground',
					)
				"
				@click="
					props.item.id === SIDEBAR_ITEM_PLACEHOLDER_ID
						? emit('create')
						: props.item.onClick?.()
				"
			>
				<div v-if="!props.item.url" class="min-w-0 cursor-pointer">
					<div class="flex w-full items-center justify-between gap-1">
						<div class="flex min-w-0 items-center">
							<Icon
								v-if="props.item.id === SIDEBAR_ITEM_PLACEHOLDER_ID"
								name="lucide:plus"
							/>
							<span class="truncate select-none">
								{{ props.item.name }}
							</span>
						</div>
						<div
							v-if="props.item.count"
							class="flex min-w-5.5 items-center justify-center rounded border border-border bg-background px-0.5 py-0.5 text-xs font-medium select-none"
						>
							{{ props.item.count }}
						</div>
					</div>
				</div>
				<NuxtLink
					v-else
					:data-optimistic-insert="
						props.item.localOptimisticInsert ? '' : undefined
					"
					:href="
						props.item.localOptimisticInsert || isDraggingGlobal
							? ''
							: props.item.url
					"
					:prefetch="props.item.prefetchUrlOnInteraction"
					:prefetch-on="{ interaction: true, visibility: false }"
					class="data-optimistic-insert:cursor-default"
					draggable="false"
				>
					<span>{{ props.item.name }}</span>
				</NuxtLink>
			</ShadcnUiSidebarMenuButton>
			<ShadcnUiDropdownMenu
				v-if="
					props.item.id !== SIDEBAR_ITEM_PLACEHOLDER_ID &&
					props.item.partOfDocumentTree
				"
			>
				<ShadcnUiDropdownMenuTrigger as-child>
					<ShadcnUiSidebarMenuAction class="show-on-parent-hover" show-on-hover>
						<Icon name="lucide:ellipsis" />
						<span class="sr-only">
							{{
								$t(
									"sidebar.item-dropdown-menu-trigger-button.screen-reader-hint",
								)
							}}
						</span>
					</ShadcnUiSidebarMenuAction>
				</ShadcnUiDropdownMenuTrigger>
				<ShadcnUiDropdownMenuContent
					side="bottom"
					align="start"
					loop
					inside-sheet
				>
					<ShadcnUiDropdownMenuItem @click="emit('duplicate')">
						<Icon name="mingcute:copy-2-line" />
						<span>
							{{ $t("sidebar.item-dropdown-menu-buttons.duplicate-page") }}
						</span>
					</ShadcnUiDropdownMenuItem>
					<ShadcnUiDropdownMenuItem @click="emit('create')">
						<Icon name="lucide:file-plus" />
						<span>
							{{ $t("sidebar.item-dropdown-menu-buttons.add-sub-page") }}
						</span>
					</ShadcnUiDropdownMenuItem>
					<ShadcnUiDropdownMenuItem @click="emit('delete')">
						<Icon name="lucide:trash-2" />
						<span>
							{{ $t("sidebar.item-dropdown-menu-buttons.delete-page") }}
						</span>
					</ShadcnUiDropdownMenuItem>
				</ShadcnUiDropdownMenuContent>
			</ShadcnUiDropdownMenu>
		</div>
	</div>
</template>
