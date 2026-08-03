<script lang="ts" setup>
import { cn } from "~/lib/utils"
import {
	VisualizationCoreUnit,
	VisualizationDataUnit,
	VisualizationMiscUnit,
	VisualizationTimeUnit,
	type VisualizationUnit,
} from "../utils"
import { DiffStatus } from "~/components/editor/diff/position-map"

const props = defineProps<{
	diffMode?: boolean
	diffStatus?: DiffStatus | null
	oldUnit?: VisualizationUnit | null
}>()
const selectedUnit = defineModel<VisualizationUnit | undefined | null>()
const { isEditable } = useEditorMeta()
const editorStore = useEditorStore()

const isEditingDisabled = computed(() => {
	return !isEditable.value || editorStore.reviewableDiffActive
})
const nonCoreUnits = {
	time: Object.values(VisualizationTimeUnit),
	data: Object.values(VisualizationDataUnit),
	misc: Object.values(VisualizationMiscUnit),
}
const unitToCategory = Object.fromEntries(
	Object.entries(nonCoreUnits).flatMap(([category, units]) =>
		units.map((unit: any) => [unit, category]),
	),
) as Record<
	VisualizationTimeUnit | VisualizationDataUnit | VisualizationMiscUnit,
	keyof typeof nonCoreUnits
>
const processedSelectedUnit = computed(() => {
	let unit = selectedUnit.value
	if (!unit && props.oldUnit) {
		unit = props.oldUnit
	}

	if (!unit) {
		return null
	}

	const category = unitToCategory[unit]
	if (category) {
		return {
			category,
			unit: unit as
				| VisualizationTimeUnit
				| VisualizationDataUnit
				| VisualizationMiscUnit,
		}
	}

	return {
		category: null,
		unit: unit as VisualizationCoreUnit,
	}
})

function selectUnit(unit: VisualizationUnit) {
	if (!isEditingDisabled.value) {
		selectedUnit.value = unit
	}
}
</script>
<template>
	<ShadcnUiDropdownMenu>
		<ShadcnUiDropdownMenuTrigger as-child :disabled="isEditingDisabled">
			<ShadcnUiButton
				variant="outline-transparent"
				size="custom"
				:class="
					cn(
						'h-[1.775rem]! justify-between gap-1 px-1.5 text-2sm font-normal opacity-100!',
						isEditingDisabled && 'pointer-events-none select-text',
						props.diffStatus === DiffStatus.Added && 'bg-diff-field-added',
						props.diffStatus === DiffStatus.Removed && 'bg-diff-field-removed',
					)
				"
			>
				<span
					v-if="!processedSelectedUnit"
					class="text-muted-foreground select-none"
				>
					{{
						isEditingDisabled
							? $t("editor.metrics.config.unit-empty-value-placeholder")
							: $t("editor.metrics.config.unit-placeholder")
					}}
				</span>
				<span v-else>
					{{
						processedSelectedUnit.category
							? $t(
									`editor.metrics.config.unit-options.${processedSelectedUnit.category}.options.${processedSelectedUnit.unit}`,
								)
							: $t(
									`editor.metrics.config.unit-options.${processedSelectedUnit.unit}`,
								)
					}}
				</span>
				<Icon v-show="!isEditingDisabled" name="lucide:chevron-down" />
			</ShadcnUiButton>
		</ShadcnUiDropdownMenuTrigger>
		<ShadcnUiDropdownMenuContent
			class="w-[var(--reka-dropdown-menu-trigger-width)]"
			align="start"
			loop
			inside-modal
		>
			<ShadcnUiDropdownMenuSub
				v-for="[groupKey, units] in Object.entries(nonCoreUnits)"
				:key="groupKey"
			>
				<ShadcnUiDropdownMenuSubTrigger>
					{{ $t(`editor.metrics.config.unit-options.${groupKey}.title`) }}
				</ShadcnUiDropdownMenuSubTrigger>
				<ShadcnUiDropdownMenuPortal>
					<ShadcnUiDropdownMenuSubContent inside-modal>
						<ShadcnUiDropdownMenuItem
							v-for="unit in units"
							:key="unit"
							@click="selectUnit(unit)"
						>
							{{
								$t(
									`editor.metrics.config.unit-options.${groupKey}.options.${unit}`,
								)
							}}
						</ShadcnUiDropdownMenuItem>
					</ShadcnUiDropdownMenuSubContent>
				</ShadcnUiDropdownMenuPortal>
			</ShadcnUiDropdownMenuSub>
			<ShadcnUiDropdownMenuItem
				v-for="unit in Object.values(VisualizationCoreUnit)"
				:key="unit"
				@click="selectUnit(unit)"
			>
				{{ $t(`editor.metrics.config.unit-options.${unit}`) }}
			</ShadcnUiDropdownMenuItem>
		</ShadcnUiDropdownMenuContent>
	</ShadcnUiDropdownMenu>
</template>
