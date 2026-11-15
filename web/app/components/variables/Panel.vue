<script lang="ts" setup>
import type { VariableGroupTreeElement, VariableTreeElement } from "~/utils"
import { showRequestErrorToast, showToastMessage } from "../toast"
import { variableReference, type DeletionTarget } from "./variables"

type View =
	| { kind: "list" }
	| { kind: "variable"; variableId: string | null; groupId: string | null }
	| { kind: "group"; groupId: string | null }

const props = defineProps<{
	usedVariableIds: Set<string>
}>()
const emit = defineEmits<{
	(e: "close-variable-panel"): void
}>()
// the panel cannot close while it is in edit mode, so the page follows it
const editing = defineModel<boolean>("editing", { default: false })

const { t } = useI18n({ useScope: "global" })
const { fetchVariables, deleteVariable, deleteVariableGroup } = useVariableAPI()
const { canRoleEdit } = useAuthSession()

const view = ref<View>({ kind: "list" })
// normal mode puts the variables this page uses first, in their own section
const byPage = ref(true)
const pendingDeletion = ref<DeletionTarget | null>(null)

const tree = computed(() => fetchVariables.data.value ?? [])
const loading = computed(
	() => !tree.value.length && fetchVariables.status.value === "pending",
)
const failed = computed(
	() => !tree.value.length && fetchVariables.status.value === "error",
)

onBeforeMount(() => {
	void fetchVariables.refresh()
})

// a reader whose role changes to viewer mid-edit leaves edit mode, since
// a viewer changes no variables
watch(canRoleEdit, (allowed) => {
	if (!allowed) {
		stopEditing()
	}
})

function startEditing() {
	editing.value = true
}

function stopEditing() {
	editing.value = false
	view.value = { kind: "list" }
}

function showList() {
	view.value = { kind: "list" }
}

function requestVariableDeletion(variable: VariableTreeElement) {
	pendingDeletion.value = {
		kind: "variable",
		name: variable.name,
		reference: variableReference(variable.key),
		confirm: async () => {
			try {
				await deleteVariable.mutateAsync(variable.id)
			} catch (err) {
				showRequestErrorToast(err, t("variable.errors.delete-failed"))
				return
			}

			showToastMessage(
				"success",
				t("variable.toasts.deleted", { name: variable.name }),
			)
			showList()
		},
	}
}

function requestGroupDeletion(group: VariableGroupTreeElement) {
	pendingDeletion.value = {
		kind: "group",
		name: group.groupName,
		variableCount: group.variables.length,
		confirm: async () => {
			try {
				await deleteVariableGroup.mutateAsync(group.id)
			} catch (err) {
				showRequestErrorToast(err, t("variable.errors.group-delete-failed"))
				return
			}

			showToastMessage(
				"success",
				t("variable.toasts.deleted", { name: group.groupName }),
			)
			showList()
		},
	}
}
</script>
<template>
	<aside class="flex max-h-svh shrink-0 flex-col bg-background">
		<template v-if="view.kind === 'list'">
			<header
				class="top-0 z-navbar h-10 shrink-0 border-b border-border bg-background pr-2 pl-4"
			>
				<div class="flex h-full w-full items-center justify-between gap-4">
					<h2 class="flex text-sm">
						{{ t("variable.title") }}
					</h2>
					<div class="flex items-center gap-0.5">
						<ShadcnUiButton
							v-if="editing"
							size="2sm"
							class="h-6.5 px-2.5 text-xs"
							@click="stopEditing"
						>
							<Icon name="mingcute:check-line" class="size-3.5" />
							{{ t("variable.actions.done") }}
						</ShadcnUiButton>
						<ShadcnUiButton
							v-else-if="canRoleEdit"
							variant="ghost"
							size="icon-sm"
							class="shrink-0"
							@click="startEditing"
						>
							<Icon name="mingcute:pencil-line" class="size-4" />
							<span class="sr-only">{{ t("variable.actions.edit") }}</span>
						</ShadcnUiButton>
						<ShadcnUiTooltip v-if="!editing" :delay-duration="600">
							<ShadcnUiTooltipTrigger as-child>
								<ShadcnUiButton
									variant="ghost"
									size="icon-sm"
									class="shrink-0"
									@click="byPage = !byPage"
								>
									<Icon
										:name="
											byPage
												? 'mingcute:rows-2-line'
												: 'mingcute:align-justify-line'
										"
										class="size-4"
									/>
									<span class="sr-only">
										{{
											byPage
												? t("variable.actions.show-one-list")
												: t("variable.actions.show-by-page")
										}}
									</span>
								</ShadcnUiButton>
							</ShadcnUiTooltipTrigger>
							<ShadcnUiTooltipContent
								side="bottom"
								align="end"
								class="max-w-65 text-2sm text-wrap"
							>
								<div class="flex flex-col gap-2">
									<p>
										{{
											byPage
												? t("variable.by-page-tooltip.on-now")
												: t("variable.by-page-tooltip.off-now")
										}}
									</p>
									<p>
										{{
											byPage
												? t("variable.by-page-tooltip.on-click")
												: t("variable.by-page-tooltip.off-click")
										}}
									</p>
								</div>
							</ShadcnUiTooltipContent>
						</ShadcnUiTooltip>
						<ShadcnUiButton
							v-if="!editing"
							variant="ghost"
							size="icon-sm"
							class="shrink-0"
							@click.stop="emit('close-variable-panel')"
						>
							<Icon name="ph:caret-double-left-bold" class="size-4" />
							<span class="sr-only">{{ t("variable.actions.close") }}</span>
						</ShadcnUiButton>
					</div>
				</div>
			</header>
			<p class="shrink-0 px-4 pt-2.5 text-xs text-muted-foreground">
				{{ editing ? t("variable.intro.edit") : t("variable.intro.normal") }}
			</p>
			<div class="flex flex-1 overflow-y-auto p-1.25">
				<div
					v-if="loading"
					class="flex flex-1 items-center justify-center py-10 text-muted-foreground"
				>
					<Icon
						name="svg-spinners:blocks-shuffle-3"
						class="size-5 opacity-50"
					/>
				</div>
				<p
					v-else-if="failed"
					class="flex-1 px-2 py-10 text-center text-2sm text-muted-foreground"
				>
					{{ t("variable.errors.load-failed") }}
				</p>
				<VariablesEditList
					v-else-if="editing"
					:tree="tree"
					@open-variable="
						(id) => (view = { kind: 'variable', variableId: id, groupId: null })
					"
					@new-variable="
						(id) => (view = { kind: 'variable', variableId: null, groupId: id })
					"
					@edit-group="(id) => (view = { kind: 'group', groupId: id })"
					@new-group="view = { kind: 'group', groupId: null }"
				/>
				<VariablesNormalList
					v-else
					:tree="tree"
					:used-ids="props.usedVariableIds"
					:by-page="byPage"
					@edit="startEditing"
				/>
			</div>
		</template>
		<VariablesVariableForm
			v-else-if="view.kind === 'variable'"
			:key="view.variableId ?? `new-${view.groupId}`"
			:tree="tree"
			:variable-id="view.variableId"
			:group-id="view.groupId"
			@close="showList"
			@delete="requestVariableDeletion"
		/>
		<VariablesGroupForm
			v-else
			:key="view.groupId ?? 'new'"
			:tree="tree"
			:group-id="view.groupId"
			@close="showList"
			@delete="requestGroupDeletion"
		/>
		<VariablesDeletionModal v-model="pendingDeletion" />
	</aside>
</template>
