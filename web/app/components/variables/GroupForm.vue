<script lang="ts" setup>
import { toTypedSchema } from "@vee-validate/zod"
import { useForm } from "vee-validate"
import * as z from "zod"
import type { VariableGroupTreeElement, VariableTreeResponse } from "~/utils"
// for some reason this isn't auto imported
import { FormField as ShadcnUiFormField } from "#layers/ui/app/components/shadcn/ui/form"
import { showToastMessage } from "../toast"
import { useGroupOpenState } from "./group-state"
import { NO_AUTOFILL } from "./variables"

const props = defineProps<{
	tree: VariableTreeResponse
	groupId: string | null
}>()
const emit = defineEmits<{
	(e: "close"): void
	(e: "delete", group: VariableGroupTreeElement): void
}>()

const { t } = useI18n({ useScope: "global" })
const { createVariableGroup, updateVariableGroup } = useVariableAPI()
const { created: createdGroups } = useGroupOpenState()

const group = props.tree.find((g) => g.id === props.groupId) ?? null
const formSchema = toTypedSchema(
	z.object({
		name: z
			.string()
			.trim()
			.min(1, { message: t("variable.group-form.name-required") })
			.refine(
				(name) =>
					!props.tree.some((g) => g.id !== group?.id && g.groupName === name),
				{ message: t("variable.group-form.name-taken") },
			),
		color: z.string(),
	}),
)
const form = useForm({
	validationSchema: formSchema,
	initialValues: {
		name: group?.groupName ?? "",
		color: group?.color ?? suggestedColor(),
	},
})

const saving = ref(false)

const currentGroup = computed(() => props.tree.find((g) => g.id === group?.id))
const variableCount = computed(() => currentGroup.value?.variables.length ?? 0)

const onSubmit = form.handleSubmit(async (values) => {
	saving.value = true
	await delay(300) // show loading spinner for at least a moment
	const req = {
		groupName: values.name,
		color: values.color,
	}

	try {
		if (group) {
			await updateVariableGroup.mutateAsync({ id: group.id, req: req })
		} else {
			const created = await createVariableGroup.mutateAsync(req)
			createdGroups.value.push(created.id)
		}

		emit("close")
	} catch {
		showToastMessage("error", t("variable.errors.group-save-failed"))
	} finally {
		saving.value = false
	}
})

// a new group takes a color no group has yet, or else one of the least
// used, as a new tag does
function suggestedColor(): string {
	return (
		pickLeastUsedColor(
			PALETTE_COLORS,
			props.tree.map((g) => g.color),
		) ?? DEFAULT_PALETTE_COLOR
	)
}
</script>
<template>
	<div class="flex min-h-0 flex-1 flex-col">
		<VariablesFormHeader
			:title="
				group
					? t('variable.group-form.edit-title')
					: t('variable.group-form.new-title')
			"
			:dirty="form.meta.value.dirty"
			:creating="!group"
			:saving="saving"
			@back="emit('close')"
			@cancel="emit('close')"
			@save="onSubmit()"
		/>
		<form
			class="flex min-h-0 flex-1 flex-col overflow-y-auto pb-4"
			autocomplete="off"
			@submit="onSubmit"
		>
			<ShadcnUiFormField v-slot="{ componentField }" name="name">
				<ShadcnUiFormItem class="gap-1 px-4 py-3">
					<ShadcnUiFormLabel
						disable-destructive-effect
						class="text-2sm font-medium text-foreground"
					>
						{{ t("variable.group-form.name-label") }}
					</ShadcnUiFormLabel>
					<div class="flex gap-2">
						<ShadcnUiFormControl>
							<ShadcnUiInput
								disable-focus-effect
								v-bind="{ ...componentField, ...NO_AUTOFILL }"
								:placeholder="t('variable.group-form.name-placeholder')"
								class="h-[1.775rem] px-2 text-2sm md:text-2sm"
							/>
						</ShadcnUiFormControl>
						<ShadcnUiFormField v-slot="{ value, handleChange }" name="color">
							<ShadcnUiPopover>
								<ShadcnUiPopoverTrigger as-child>
									<ShadcnUiButton
										type="button"
										variant="outline"
										size="custom"
										class="size-[1.775rem] shrink-0"
									>
										<span
											class="size-3.5 rounded-full"
											:style="{ backgroundColor: paletteColor(value) }"
										/>
										<span class="sr-only">
											{{ t("variable.group-form.color-button") }}
										</span>
									</ShadcnUiButton>
								</ShadcnUiPopoverTrigger>
								<ShadcnUiPopoverContent
									side="bottom"
									align="end"
									class="w-fit min-w-0"
									inside-sheet
								>
									<ColorSelect
										:model-value="value"
										@update:model-value="handleChange"
									/>
								</ShadcnUiPopoverContent>
							</ShadcnUiPopover>
						</ShadcnUiFormField>
					</div>
					<ShadcnUiFormMessage class="text-xs text-destructive-foreground" />
				</ShadcnUiFormItem>
			</ShadcnUiFormField>
			<template v-if="group">
				<div class="px-4 pt-1 pb-2.5">
					<ShadcnUiSeparator />
				</div>
				<ShadcnUiButton
					type="button"
					variant="ghost-plain-destructive"
					size="custom"
					class="mx-4 h-7 gap-1.5 self-center text-2sm"
					@click="currentGroup && emit('delete', currentGroup)"
				>
					<Icon name="mingcute:delete-2-line" class="size-3.5" />
					{{ t("variable.group-form.delete-button") }}
				</ShadcnUiButton>
				<p
					v-if="variableCount > 0"
					class="px-4 text-center text-xs text-muted-foreground"
				>
					{{
						t("variable.group-form.delete-blocked-hint", {
							count: variableCount,
						})
					}}
				</p>
				<p v-else class="px-4 text-center text-xs text-muted-foreground">
					{{ t("variable.group-form.delete-clean-hint") }}
				</p>
			</template>
		</form>
	</div>
</template>
