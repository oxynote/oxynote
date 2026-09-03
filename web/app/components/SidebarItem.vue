<script lang="ts" setup>
import type { HTMLAttributes } from "vue"
import {
	SIDEBAR_ITEM_PLACEHOLDER_ID,
	type SidebarItem,
	type SidebarItemAction,
} from "./sidebar"
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
	(e: "toggle-collapse" | "create"): void
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

// hovering a row swaps its icon or dot for the collapse chevron and fades
// the actions menu in. Only a row with a child list to open has the first,
// and only one that can hold children has the second; a row with neither
// takes no part in the swap.
const collapseChevron = computed(
	() => !!(props.item.icon || props.item.dotColor) && !!props.item.children,
)
const actionsMenu = computed(() => props.item.actions.length > 0)
const swapsOnHover = computed(() => collapseChevron.value || actionsMenu.value)

function handleClick() {
	if (props.item.id === SIDEBAR_ITEM_PLACEHOLDER_ID) {
		emit("create")
		return
	}

	if (props.item.onClick) {
		void props.item.onClick()
		return
	}

	if (!props.item.url && collapseChevron.value) {
		emit("toggle-collapse")
	}
}

function runAction(action: SidebarItemAction) {
	void action.fn()
}
</script>

<template>
	<div
		ref="sidebar-item"
		:data-dragged-on="isItemDraggedOn ? '' : undefined"
		:data-dragging-global="swapsOnHover ? isDraggingGlobal : undefined"
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
				v-if="props.item.icon || props.item.dotColor"
				:class="[
					collapseChevron ? 'hide-on-parent-hover' : undefined,
					'pointer-events-auto opacity-100 transition-opacity duration-50',
				]"
				side="left"
				:disable-direct-interaction="!collapseChevron"
			>
				<Icon v-if="props.item.icon" :name="props.item.icon" />
				<span
					v-else
					class="size-2.5 shrink-0 rounded-full"
					:style="{ backgroundColor: props.item.dotColor }"
				/>
			</ShadcnUiSidebarMenuAction>
			<ShadcnUiSidebarMenuAction
				v-if="collapseChevron"
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
				@click="handleClick"
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
			<ShadcnUiDropdownMenu v-if="actionsMenu">
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
					<ShadcnUiDropdownMenuItem
						v-for="action in props.item.actions"
						:key="action.id"
						@click="runAction(action)"
					>
						<Icon :name="action.icon" />
						<span>{{ action.name }}</span>
					</ShadcnUiDropdownMenuItem>
				</ShadcnUiDropdownMenuContent>
			</ShadcnUiDropdownMenu>
		</div>
	</div>
</template>
