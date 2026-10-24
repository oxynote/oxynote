<script lang="ts" setup>
import type {
	VariableGroupTreeElement,
	VariableTreeElement,
	VariableTreeResponse,
} from "~/utils"
import { showToastMessage } from "../toast"
import {
	VARIABLE_DRAG_GROUPS,
	draggableItem,
	groupMoveIndex,
	hasOwnGroups,
	orderedGroups,
	variableMoveRequest,
} from "./variables"

const props = defineProps<{
	tree: VariableTreeResponse
}>()
const emit = defineEmits<{
	(e: "open-variable" | "edit-group" | "new-variable", id: string): void
	(e: "new-group"): void
}>()

const { t } = useI18n({ useScope: "global" })
const { moveVariable, moveVariableGroup } = useVariableAPI()

// a group dropped onto the add group row goes to the end
const ADD_GROUP_ROW = draggableItem({
	id: "variable-groups-end",
	parentId: null,
	dragGroup: VARIABLE_DRAG_GROUPS.group,
	draggable: false,
	acceptsChildren: true,
})

const groups = computed(() => orderedGroups(props.tree))
const onlyBuiltins = computed(() => !hasOwnGroups(props.tree))
const movableGroupIds = computed(() =>
	groups.value.filter((g) => !g.builtin).map((g) => g.id),
)

function groupItem(group: VariableGroupTreeElement) {
	return draggableItem({
		id: group.id,
		parentId: null,
		dragGroup: group.builtin
			? VARIABLE_DRAG_GROUPS.builtinGroup
			: VARIABLE_DRAG_GROUPS.group,
		draggable: !group.builtin,
		acceptsChildren: false,
	})
}

function variableItem(variable: VariableTreeElement) {
	return draggableItem({
		id: variable.id,
		parentId: variable.groupId,
		dragGroup: variable.builtin
			? VARIABLE_DRAG_GROUPS.builtinVariable
			: VARIABLE_DRAG_GROUPS.variable,
		draggable: !variable.builtin,
		acceptsChildren: false,
	})
}

// the add variable row carries its group's id, so a variable dropped onto
// it lands at the end of that group
function addVariableItem(group: VariableGroupTreeElement) {
	return draggableItem({
		id: group.id,
		parentId: null,
		dragGroup: VARIABLE_DRAG_GROUPS.variable,
		draggable: false,
		acceptsChildren: true,
	})
}

async function handleGroupMove(id: string, insertBeforeId: string | null) {
	const sortIndex = groupMoveIndex(props.tree, id, insertBeforeId)
	if (sortIndex === null) {
		return
	}

	try {
		await moveVariableGroup.mutateAsync({ id: id, sortIndex: sortIndex })
	} catch {
		showToastMessage("error", t("variable.errors.move-failed"))
	}
}

async function handleVariableMove(
	id: string,
	data: { parentId: string | null; insertBeforeId: string | null },
) {
	if (!data.parentId) {
		return
	}

	const req = variableMoveRequest(
		props.tree,
		id,
		data.parentId,
		data.insertBeforeId,
	)
	if (!req) {
		return
	}

	try {
		await moveVariable.mutateAsync(req)
	} catch {
		showToastMessage("error", t("variable.errors.move-failed"))
	}
}

// previousGroupId names the group right before this one. Dropping that
// group here would leave it where it is.
function previousGroupId(group: VariableGroupTreeElement) {
	const index = movableGroupIds.value.indexOf(group.id)

	return index > 0 ? movableGroupIds.value[index - 1] : null
}
</script>
<template>
	<div class="flex min-w-0 flex-1 flex-col pt-1.5">
		<VariablesGroup
			v-for="group in groups"
			:key="group.id"
			:group="group"
			section="edit"
		>
			<template #header="{ open }">
				<VariablesDraggableRow
					:item="groupItem(group)"
					:previous-id="previousGroupId(group)"
					@move="(data) => handleGroupMove(group.id, data.insertBeforeId)"
				>
					<VariablesGroupHeader :group="group" :open="open">
						<template #start>
							<Icon
								v-if="group.builtin"
								name="mingcute:lock-line"
								class="size-3.5 shrink-0 text-muted-foreground"
							/>
							<Icon
								v-else
								name="mingcute:dots-line"
								class="size-3.5 shrink-0 cursor-grab text-muted-foreground/50"
							/>
						</template>
						<template v-if="!group.builtin" #end>
							<ShadcnUiButton
								variant="ghost"
								size="icon-xsm"
								class="text-muted-foreground"
								@click="emit('edit-group', group.id)"
							>
								<Icon name="mingcute:pencil-line" class="size-3.5" />
								<span class="sr-only">
									{{
										t("variable.actions.rename-group", {
											name: group.groupName,
										})
									}}
								</span>
							</ShadcnUiButton>
						</template>
					</VariablesGroupHeader>
					<template #ghost>
						<VariablesGroupHeader :group="group" :open="open">
							<template #start>
								<Icon
									name="mingcute:dots-line"
									class="size-3.5 shrink-0 text-muted-foreground/50"
								/>
							</template>
						</VariablesGroupHeader>
						<div v-if="open" class="relative pt-0.5 pb-1">
							<div
								class="absolute top-1 bottom-2 left-3.5 z-1 w-0.5 rounded-full opacity-35"
								:style="{ backgroundColor: paletteColor(group.color) }"
							/>
							<VariablesEditVariableRow
								v-for="variable in group.variables"
								:key="variable.id"
								:variable="variable"
							/>
							<div
								class="flex h-8 items-center gap-2 pr-2 pl-7 text-2sm text-muted-foreground"
							>
								<Icon name="mingcute:add-line" class="size-3.5" />
								{{ t("variable.actions.add-variable") }}
							</div>
						</div>
					</template>
				</VariablesDraggableRow>
			</template>
			<VariablesDraggableRow
				v-for="(variable, index) in group.variables"
				:key="variable.id"
				:item="variableItem(variable)"
				:previous-id="group.variables[index - 1]?.id ?? null"
				class="group/variable-row"
				@move="(data) => handleVariableMove(variable.id, data)"
			>
				<VariablesEditVariableRow
					:variable="variable"
					@open="emit('open-variable', variable.id)"
				/>
			</VariablesDraggableRow>
			<VariablesDraggableRow
				v-if="!group.builtin"
				:item="addVariableItem(group)"
				:previous-id="group.variables.at(-1)?.id ?? null"
			>
				<button
					type="button"
					class="flex h-8 w-full cursor-pointer items-center gap-2 rounded-md pr-2 pl-7 text-2sm text-muted-foreground transition-colors duration-150 outline-none hover:bg-accent/50"
					@click="emit('new-variable', group.id)"
				>
					<Icon name="mingcute:add-line" class="size-3.5" />
					{{ t("variable.actions.add-variable") }}
				</button>
			</VariablesDraggableRow>
		</VariablesGroup>
		<template v-if="onlyBuiltins">
			<div class="px-2 pt-1.5">
				<ShadcnUiSeparator />
			</div>
			<ShadcnUiEmpty class="flex-none gap-3 p-6 md:p-8">
				<ShadcnUiEmptyHeader>
					<ShadcnUiEmptyMedia>
						<Icon
							name="mingcute:layers-line"
							class="size-8 text-muted-foreground/50"
						/>
					</ShadcnUiEmptyMedia>
					<ShadcnUiEmptyTitle class="font-semibold">
						{{ t("variable.empty.no-groups-title") }}
					</ShadcnUiEmptyTitle>
					<ShadcnUiEmptyDescription>
						{{ t("variable.empty.no-groups-description") }}
					</ShadcnUiEmptyDescription>
				</ShadcnUiEmptyHeader>
			</ShadcnUiEmpty>
		</template>
		<!--the separator belongs to the add group row, so a group dropped at the
			end shows its marker above the line, right after the last group-->
		<VariablesDraggableRow
			:item="ADD_GROUP_ROW"
			:previous-id="movableGroupIds.at(-1) ?? null"
		>
			<div class="-mx-1.25 py-1.5">
				<ShadcnUiSeparator />
			</div>
			<button
				type="button"
				class="flex h-8 w-full cursor-pointer items-center gap-2 rounded-md px-2 text-2sm text-muted-foreground transition-colors duration-150 outline-none hover:bg-accent/50"
				@click="emit('new-group')"
			>
				<Icon name="mingcute:add-line" class="size-3.5" />
				{{ t("variable.actions.add-group") }}
			</button>
		</VariablesDraggableRow>
	</div>
</template>
