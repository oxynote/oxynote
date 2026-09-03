<script lang="ts" setup>
import {
	SIDEBAR_ITEM_PLACEHOLDER_ID,
	type SidebarItem,
	type SidebarItemCreate,
	type SidebarItemLocationUpdate,
} from "./sidebar"

const props = defineProps<{
	itemId: string | null // null indicates root level
}>()
const emit = defineEmits<{
	(e: "create", data: SidebarItemCreate): void
	(e: "update-location", data: SidebarItemLocationUpdate): void
}>()
const items = defineModel<SidebarItem[]>()

// parentId indicates that the current item can also be a parent to other items.
const parentId = computed(() => props.itemId)

const collapseOpen = usePersistentState<Record<string, number>>({
	key: "sidebar-item-collapse",
	defaultValue: () => {
		return {}
	},
	serializer: {
		write: (data) => btoa(jsonStableStringify(data)),
		read: (data) => JSON.parse(atob(data)) as Record<string, number>,
	},
})

const collapseWrapperElems = ref<
	Record<string, HTMLElement | null | undefined>
>({})

if (!props.itemId && !Object.keys(collapseOpen.value).length) {
	// make the first level items open
	items.value?.forEach((item) => {
		if (
			!item.children?.length ||
			(item.children.length === 1 &&
				item.children[0]?.id === SIDEBAR_ITEM_PLACEHOLDER_ID)
		) {
			return
		}

		collapseOpen.value[item.id] = 1
	})
}

function collectCollapseWrappers(
	id: string,
	elem: HTMLElement | null | undefined,
) {
	if (!elem) {
		delete collapseWrapperElems.value[id]
		return
	}

	collapseWrapperElems.value[id] = elem
}

function toggleCollapseOpen(id: string) {
	if (collapseOpen.value[id]) {
		delete collapseOpen.value[id]
		return
	}

	collapseOpen.value[id] = 1
}
</script>

<template>
	<div>
		<div v-for="item in items" :key="item.id">
			<ShortcutTooltip side="right" :shortcut="item.shortcutTooltip">
				<ShadcnUiSidebarMenuItem
					:data-item-id="item.id"
					:data-item-children="item.children?.length"
				>
					<ShadcnUiCollapsible
						class="group/collapsible"
						:default-open="collapseOpen[item.id] === 1"
						:open="collapseOpen[item.id] === 1"
					>
						<SidebarItem
							:parent-id="parentId"
							:item="item"
							:wrapper="collapseWrapperElems[item.id]"
							:active="item.active"
							:open="collapseOpen[item.id] === 1"
							@toggle-collapse="toggleCollapseOpen(item.id)"
							@create="
								emit('create', {
									parentId:
										item.id === SIDEBAR_ITEM_PLACEHOLDER_ID
											? parentId
											: item.id,
								})
							"
							@update-location="
								(data) =>
									emit('update-location', {
										id: item.id,
										parentId: data.parentId,
										insertBeforeId: data.insertBeforeId,
									})
							"
						/>
						<ShadcnUiCollapsibleContent v-if="item.children" as-child>
							<div
								:ref="
									(elem) =>
										collectCollapseWrappers(
											item.id,
											elem as HTMLElement | null | undefined,
										)
								"
							>
								<ShadcnUiSidebarMenuSub>
									<SidebarNestedGroup
										v-model="item.children"
										:item-id="item.id"
										@update-location="(data) => emit('update-location', data)"
										@create="(data) => emit('create', data)"
									/>
								</ShadcnUiSidebarMenuSub>
							</div>
						</ShadcnUiCollapsibleContent>
					</ShadcnUiCollapsible>
				</ShadcnUiSidebarMenuItem>
			</ShortcutTooltip>
		</div>
	</div>
</template>

<style lang="css" scoped>
@reference "@/assets/css/main.css";

.sidebar-floating-item {
	@apply hidden;
}
</style>
