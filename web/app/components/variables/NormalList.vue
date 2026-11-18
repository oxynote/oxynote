<script lang="ts" setup>
import type { VariableTreeResponse } from "~/utils"
import {
	hasOwnGroups,
	orderedGroups,
	variableSections,
	type VariableSection,
} from "./variables"

const props = defineProps<{
	tree: VariableTreeResponse
	usedIds: Set<string>
	byPage: boolean
}>()
const emit = defineEmits<{
	(e: "edit"): void
}>()

const { t } = useI18n({ useScope: "global" })
const { canRoleEdit } = useAuthSession()

const sections = computed<VariableSection[]>(() =>
	props.byPage
		? variableSections(props.tree, props.usedIds)
		: [
				{
					id: "all",
					groups: orderedGroups(props.tree).map((group) => ({
						group: group,
						variables: group.variables,
					})),
				},
			],
)
const onlyBuiltins = computed(() => !hasOwnGroups(props.tree))

function sectionLabel(id: VariableSection["id"]) {
	switch (id) {
		case "on-this-page":
			return t("variable.sections.on-this-page")
		default:
			// "other" only reads right next to the on this page section
			return sections.value.some((s) => s.id === "on-this-page")
				? t("variable.sections.other")
				: t("variable.sections.used-on-other-pages")
	}
}
</script>
<template>
	<div class="flex min-w-0 flex-1 flex-col">
		<section
			v-for="section in sections"
			:key="section.id"
			class="flex min-w-0 flex-col"
		>
			<h3
				v-if="section.id !== 'all'"
				class="px-2 pt-3 pb-1 text-2xs font-semibold tracking-wide text-muted-foreground/70 uppercase"
			>
				{{ sectionLabel(section.id) }}
			</h3>
			<VariablesGroup
				v-for="entry in section.groups"
				:key="entry.group.id"
				:group="entry.group"
				:section="section.id"
			>
				<template #header="{ open }">
					<VariablesGroupHeader :group="entry.group" :open="open" />
				</template>
				<VariablesValueControl
					v-for="variable in entry.variables"
					:key="variable.id"
					:variable="variable"
				/>
				<p
					v-if="!entry.group.variables.length"
					class="py-1.5 pr-2 pl-7 text-2sm text-muted-foreground"
				>
					{{ t("variable.empty.group") }}
				</p>
			</VariablesGroup>
		</section>
		<template v-if="onlyBuiltins">
			<div class="px-2 pt-2.5">
				<ShadcnUiSeparator />
			</div>
			<ShadcnUiEmpty class="flex-none gap-3 p-6 md:p-8">
				<ShadcnUiEmptyHeader>
					<ShadcnUiEmptyMedia>
						<Icon
							name="mingcute:braces-line"
							class="size-8 text-muted-foreground/50"
						/>
					</ShadcnUiEmptyMedia>
					<ShadcnUiEmptyTitle class="font-semibold">
						{{ t("variable.empty.builtin-only-title") }}
					</ShadcnUiEmptyTitle>
					<ShadcnUiEmptyDescription>
						{{
							canRoleEdit
								? t("variable.empty.builtin-only-description")
								: t("variable.empty.builtin-only-description-viewer")
						}}
					</ShadcnUiEmptyDescription>
				</ShadcnUiEmptyHeader>
				<ShadcnUiEmptyContent v-if="canRoleEdit">
					<ShadcnUiButton size="sm" class="text-2sm" @click="emit('edit')">
						<Icon name="mingcute:add-line" />
						{{ t("variable.empty.builtin-only-action") }}
					</ShadcnUiButton>
				</ShadcnUiEmptyContent>
			</ShadcnUiEmpty>
		</template>
	</div>
</template>
