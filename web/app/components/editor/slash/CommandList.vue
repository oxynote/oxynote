<script lang="ts" setup>
import {
	commandGroupSortIndex,
	type CommandData,
	type CommandGroup,
	type CommandItem,
} from "./items"
import CommandButton from "./CommandButton.vue"
import { SLASH_COMMAND_TRIGGER_CHAR } from "./extension"
import { SHORTCUT_ACTIONS } from "#imports"

const props = defineProps<{
	query: string
	items: CommandItem[]
	command: (props: {
		title: string
		command: (data: CommandData) => void
	}) => void
	initClose: () => void
}>()
defineExpose({
	close,
	onKeyDown,
})

const { osType } = useDetectHost()
const selectedIndex = ref<number | null>(0)
const open = ref(false)
const processedItems = computed(() => {
	const groups: Record<
		string,
		{
			sortIndex: number
			items: CommandItem[]
		}
	> = {}

	if (props.items.length) {
		props.items.forEach((item) => {
			let group = groups[item.group]
			if (!group) {
				group = {
					sortIndex: commandGroupSortIndex(item.group),
					items: [],
				}
				groups[item.group] = group
			}

			group.items.push(item)
		})
	}

	let res: CommandItem[] = []

	Object.values(groups)
		.sort((a, b) => (a.sortIndex < b.sortIndex ? -1 : 1))
		.forEach((group) => {
			res = res.concat(group.items)
		})

	return res
})
const multipleGroups = computed(() => {
	let firstGroup: CommandGroup | "" = ""

	for (const item of processedItems.value) {
		if (!firstGroup) {
			firstGroup = item.group
			continue
		} else if (firstGroup !== item.group) {
			return true
		}
	}

	return false
})

onMounted(() => {
	// this must be set in onMounted to have a proper opacity transition
	open.value = true
})

watch(processedItems, (newV) => {
	selectedIndex.value = newV.length ? 0 : null
})

function close(afterClose: () => void) {
	open.value = false
	setTimeout(afterClose, 500)
}

function onKeyDown(event: KeyboardEvent) {
	if (selectedIndex.value !== null) {
		if (event.key === "ArrowUp") {
			selectedIndex.value--

			if (selectedIndex.value < 0) {
				selectedIndex.value = processedItems.value.length - 1
			}

			return true
		}

		if (event.key === "ArrowDown") {
			selectedIndex.value++

			if (selectedIndex.value > processedItems.value.length - 1) {
				selectedIndex.value = 0
			}

			return true
		}
	}

	if (event.key === "Enter") {
		if (selectedIndex.value !== null) {
			selectItem(selectedIndex.value)
		} else {
			props.initClose()
		}

		return true
	}

	return false
}

function selectItem(index: number) {
	const item = processedItems.value[index]

	if (item) {
		props.command(item)
	}
}
</script>
<template>
	<div
		:data-state="open ? 'open' : 'closed'"
		class="absolute z-popover max-h-[70dvh] min-w-[13rem] overflow-x-hidden overflow-y-auto rounded-lg border bg-popover p-1 text-popover-foreground opacity-0 shadow-md transition-opacity duration-150 data-[state=open]:opacity-100"
	>
		<template v-if="processedItems.length">
			<template v-for="(item, index) in processedItems" :key="index">
				<CommandButton
					:item="item"
					:item-index="index"
					:selected-index="selectedIndex"
					@click="selectItem(index)"
					@hover="selectedIndex = index"
				/>
				<div
					v-if="
						multipleGroups &&
						index !== processedItems.length - 1 &&
						processedItems[index + 1] &&
						processedItems[index + 1]?.group !== item.group
					"
					class="-mx-1 my-1 h-px bg-border"
				/>
			</template>
		</template>
		<div v-else>
			<CommandButton
				:item="{
					title: $t('editor.slash-commands.no-results', {
						query: SLASH_COMMAND_TRIGGER_CHAR + props.query,
					}),
					shortcut: shortcutByOS(
						SHORTCUT_ACTIONS.addSlashCommandQueryAsPlainText.keyboardKey,
						osType,
					),
				}"
				:item-index="null"
				:selected-index="null"
				@click="props.initClose()"
			/>
		</div>
	</div>
</template>
