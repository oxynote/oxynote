<script lang="ts" setup>
import { toTypedSchema } from "@vee-validate/zod"
import { useForm } from "vee-validate"
import * as z from "zod"
import type {
	VariableRequest,
	VariableTreeElement,
	VariableTreeResponse,
	VariableType,
} from "~/utils"
// for some reason this isn't auto imported
import { FormField as ShadcnUiFormField } from "#layers/ui/app/components/shadcn/ui/form"
import { cn } from "#layers/ui/app/lib/utils"
import { showToastMessage } from "../toast"
import {
	DEFAULT_TIME_RANGE,
	NO_AUTOFILL,
	VARIABLE_KEY_PREFIX,
	keyFromName,
	keyProblem,
	optionLabel,
	orderedGroups,
	variableReference,
} from "./variables"

const props = defineProps<{
	tree: VariableTreeResponse
	variableId: string | null
	groupId: string | null
}>()
const emit = defineEmits<{
	(e: "close"): void
	(e: "delete", variable: VariableTreeElement): void
}>()

const { t } = useI18n({ useScope: "global" })
const { createVariable, updateVariable, setVariableDefault } = useVariableAPI()
const typeLabelId = useId()
const optionsLabelId = useId()

const variable =
	props.tree
		.flatMap((group) => group.variables)
		.find((v) => v.id === props.variableId) ?? null
const ownGroups = orderedGroups(props.tree).filter((group) => !group.builtin)
const formSchema = toTypedSchema(
	z
		.object({
			name: z
				.string()
				.trim()
				.min(1, { message: t("variable.variable-form.name-required") }),
			key: z.string().superRefine((key, ctx) => {
				const problem = key
					? keyProblem(key, props.tree, variable?.id ?? null)
					: "missing"
				if (!problem) {
					return
				}

				ctx.addIssue({
					code: "custom",
					message:
						problem === "missing"
							? t("variable.variable-form.key-required")
							: problem === "invalid"
								? t("variable.variable-form.key-invalid")
								: t("variable.variable-form.key-taken", {
										reference: variableReference(key),
									}),
				})
			}),
			groupId: z
				.string()
				.min(1, { message: t("variable.variable-form.group-required") }),
			type: z.enum(["list", "number", "text", "time-range"]),
			options: z.array(
				z.object({ id: z.string(), existing: z.boolean(), value: z.string() }),
			),
			numberDefault: z.number().optional(),
			textDefault: z.string().optional(),
			timeRangeDefault: z.string(),
			// a built-in list changes its default by picking an option
			listDefault: z.string().optional(),
		})
		.superRefine((values, ctx) => {
			// every type needs a default: a list takes its first option, a
			// time range always holds a preset, and the others are typed in
			if (values.type === "number" && values.numberDefault === undefined) {
				ctx.addIssue({
					code: "custom",
					path: ["numberDefault"],
					message: t("variable.variable-form.default-required"),
				})
			}

			if (values.type === "text" && !values.textDefault?.trim()) {
				ctx.addIssue({
					code: "custom",
					path: ["textDefault"],
					message: t("variable.variable-form.default-required"),
				})
			}

			if (
				!variable?.builtin &&
				values.type === "list" &&
				!values.options.length
			) {
				ctx.addIssue({
					code: "custom",
					path: ["options"],
					message: t("variable.variable-form.options-required"),
				})
			}
		}),
)
// a field the chosen type hides keeps its draft, so switching types back
// restores it and the schema always sees every field
const form = useForm({
	validationSchema: formSchema,
	keepValuesOnUnmount: true,
	initialValues: {
		name: variable?.name ?? "",
		key: variable?.key ?? "",
		groupId: variable?.groupId ?? props.groupId ?? ownGroups[0]?.id ?? "",
		type: variable?.type ?? "list",
		options:
			variable?.options.map((o) => ({
				id: o.id,
				existing: true,
				value: o.value,
			})) ?? [],
		numberDefault:
			variable?.type === "number" && variable.defaultValue !== null
				? Number(variable.defaultValue)
				: undefined,
		textDefault: variable?.type === "text" ? (variable.defaultValue ?? "") : "",
		timeRangeDefault:
			variable?.type === "time-range"
				? (variable.defaultValue ?? DEFAULT_TIME_RANGE)
				: DEFAULT_TIME_RANGE,
		listDefault:
			variable?.type === "list"
				? (variable.defaultValue ?? undefined)
				: undefined,
	},
})
const typeOptions = computed<
	{ type: VariableType; label: string; icon: string }[]
>(() => [
	{
		type: "list",
		label: t("general.variable-types.list"),
		icon: "mingcute:list-check-line",
	},
	{
		type: "number",
		label: t("general.variable-types.number"),
		icon: "mingcute:hashtag-line",
	},
	{
		type: "text",
		label: t("general.variable-types.text"),
		icon: "mingcute:text-line",
	},
	{
		type: "time-range",
		label: t("general.variable-types.time-range"),
		icon: "mingcute:time-line",
	},
])

// a new variable's key follows its name until the key is edited. An
// existing key is never rewritten, since pages reference it.
const keyEdited = ref(!!variable)
const saving = ref(false)

const builtin = !!variable?.builtin
const type = computed(() => form.values.type)
const typeLabel = computed(
	() => typeOptions.value.find((option) => option.type === type.value)?.label,
)
const groupName = props.tree.find((g) => g.id === variable?.groupId)?.groupName
const defaultFieldName = computed(() => {
	switch (type.value) {
		case "list":
			return "listDefault"
		case "number":
			return "numberDefault"
		case "text":
			return "textDefault"
		default:
			return "timeRangeDefault"
	}
})
const selectedGroup = computed(() =>
	ownGroups.find((group) => group.id === form.values.groupId),
)

watch(
	() => form.values.name,
	(name) => {
		if (keyEdited.value) {
			return
		}

		const key = keyFromName(name ?? "")
		form.setFieldValue("key", key, !!key)
	},
)

const onSubmit = form.handleSubmit(async (values) => {
	saving.value = true
	await delay(300) // show loading spinner for at least a moment

	try {
		if (variable?.builtin) {
			const value =
				values.type === "list" ? values.listDefault : values.timeRangeDefault
			if (value && value !== variable.defaultValue) {
				await setVariableDefault.mutateAsync({
					id: variable.id,
					req: { value: value },
				})
			}
		} else if (variable) {
			await updateVariable.mutateAsync({
				id: variable.id,
				req: buildRequest(values),
			})
		} else {
			await createVariable.mutateAsync(buildRequest(values))
		}

		emit("close")
	} catch {
		showToastMessage("error", t("variable.errors.save-failed"))
	} finally {
		saving.value = false
	}
})

function buildRequest(
	values: Parameters<Parameters<typeof form.handleSubmit>[0]>[0],
): VariableRequest {
	const base = {
		groupId: values.groupId,
		key: values.key,
		name: values.name,
		description: variable?.description ?? null,
		type: values.type,
	}

	switch (values.type) {
		case "list":
			return {
				...base,
				options: values.options.map((o) => ({
					id: o.existing ? o.id : undefined,
					value: o.value,
				})),
			}
		case "number":
			return {
				...base,
				defaultValue:
					values.numberDefault === undefined
						? null
						: String(values.numberDefault),
			}
		case "text":
			return { ...base, defaultValue: values.textDefault || null }
		default:
			return { ...base, defaultValue: values.timeRangeDefault }
	}
}
</script>
<template>
	<div class="flex min-h-0 flex-1 flex-col">
		<VariablesFormHeader
			:title="
				variable
					? t('variable.variable-form.edit-title')
					: t('variable.variable-form.new-title')
			"
			:dirty="form.meta.value.dirty"
			:creating="!variable"
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
			<div class="flex flex-col gap-3 px-4 py-3">
				<div
					v-if="builtin"
					class="flex gap-2 rounded-md bg-muted px-3 py-2.5 text-xs text-muted-foreground"
				>
					<Icon name="mingcute:information-line" class="size-3.5 shrink-0" />
					<p>{{ t("variable.builtin.form-notice") }}</p>
				</div>
				<ShadcnUiFormField v-slot="{ componentField }" name="name">
					<ShadcnUiFormItem class="gap-1">
						<ShadcnUiFormLabel
							disable-destructive-effect
							class="text-2sm font-medium text-foreground"
						>
							{{ t("variable.variable-form.name-label") }}
						</ShadcnUiFormLabel>
						<div class="relative">
							<ShadcnUiFormControl>
								<ShadcnUiInput
									disable-focus-effect
									v-bind="{ ...componentField, ...NO_AUTOFILL }"
									:disabled="builtin"
									class="h-[1.775rem] px-2 text-2sm md:text-2sm"
								/>
							</ShadcnUiFormControl>
							<Icon
								v-if="builtin"
								name="mingcute:lock-line"
								class="pointer-events-none absolute top-1/2 right-2 size-3.5 -translate-y-1/2 text-muted-foreground"
							/>
						</div>
						<ShadcnUiFormMessage class="text-xs text-destructive-foreground" />
					</ShadcnUiFormItem>
				</ShadcnUiFormField>
				<ShadcnUiFormField
					v-slot="{ componentField, errorMessage, value }"
					name="key"
				>
					<ShadcnUiFormItem class="gap-1">
						<ShadcnUiFormLabel
							disable-destructive-effect
							class="text-2sm font-medium text-foreground"
						>
							{{ t("variable.variable-form.key-label") }}
						</ShadcnUiFormLabel>
						<div class="relative">
							<ShadcnUiInputGroup
								disable-focus-effect
								:data-disabled="builtin"
								class="h-[1.775rem]"
							>
								<ShadcnUiInputGroupAddon
									class="self-stretch rounded-l-md bg-muted px-2 py-0 font-mono text-2sm"
								>
									{{ VARIABLE_KEY_PREFIX }}
								</ShadcnUiInputGroupAddon>
								<ShadcnUiFormControl>
									<ShadcnUiInputGroupInput
										v-bind="{ ...componentField, ...NO_AUTOFILL }"
										:disabled="builtin"
										class="px-2 font-mono text-2sm md:text-2sm"
										@input="keyEdited = true"
									/>
								</ShadcnUiFormControl>
							</ShadcnUiInputGroup>
							<Icon
								v-if="builtin"
								name="mingcute:lock-line"
								class="pointer-events-none absolute top-1/2 right-2 size-3.5 -translate-y-1/2 text-muted-foreground"
							/>
						</div>
						<ShadcnUiFormMessage
							class="text-xs break-all text-destructive-foreground"
						/>
						<i18n-t
							v-if="!errorMessage && !builtin && value"
							scope="global"
							keypath="variable.variable-form.key-hint"
							tag="p"
							class="text-xs text-muted-foreground"
						>
							<template #reference>
								<span class="font-mono font-semibold break-all">
									{{ variableReference(String(value)) }}
								</span>
							</template>
						</i18n-t>
					</ShadcnUiFormItem>
				</ShadcnUiFormField>
				<ShadcnUiFormField v-slot="{ componentField }" name="groupId">
					<ShadcnUiFormItem class="gap-1">
						<ShadcnUiFormLabel
							disable-destructive-effect
							class="text-2sm font-medium text-foreground"
						>
							{{ t("variable.variable-form.group-label") }}
						</ShadcnUiFormLabel>
						<div v-if="builtin" class="relative">
							<ShadcnUiFormControl>
								<ShadcnUiInput
									disable-focus-effect
									:model-value="groupName"
									disabled
									class="h-[1.775rem] px-2 text-2sm md:text-2sm"
								/>
							</ShadcnUiFormControl>
							<Icon
								name="mingcute:lock-line"
								class="pointer-events-none absolute top-1/2 right-2 size-3.5 -translate-y-1/2 text-muted-foreground"
							/>
						</div>
						<ShadcnUiSelect v-else v-bind="componentField">
							<ShadcnUiFormControl>
								<ShadcnUiSelectTrigger class="w-full text-2sm" size="custom">
									<span class="flex min-w-0 items-center gap-2">
										<span
											v-if="selectedGroup"
											class="size-2.5 shrink-0 rounded-full"
											:style="{
												backgroundColor: paletteColor(selectedGroup.color),
											}"
										/>
										<ShadcnUiSelectValue />
									</span>
								</ShadcnUiSelectTrigger>
							</ShadcnUiFormControl>
							<ShadcnUiSelectContent
								class="max-h-[40dvh]"
								side="bottom"
								align="start"
								inside-sheet
							>
								<ShadcnUiSelectItem
									v-for="group in ownGroups"
									:key="group.id"
									:value="group.id"
									class="text-2sm"
								>
									<span
										class="size-2.5 shrink-0 rounded-full"
										:style="{ backgroundColor: paletteColor(group.color) }"
									/>
									{{ group.groupName }}
								</ShadcnUiSelectItem>
							</ShadcnUiSelectContent>
						</ShadcnUiSelect>
						<ShadcnUiFormMessage class="text-xs text-destructive-foreground" />
					</ShadcnUiFormItem>
				</ShadcnUiFormField>
				<ShadcnUiFormField v-slot="{ value, handleChange }" name="type">
					<div class="flex flex-col gap-1">
						<span
							:id="typeLabelId"
							class="text-2sm font-medium text-foreground"
						>
							{{ t("variable.variable-form.type-label") }}
						</span>
						<div v-if="builtin" class="relative">
							<ShadcnUiInput
								disable-focus-effect
								:model-value="typeLabel"
								:aria-labelledby="typeLabelId"
								disabled
								class="h-[1.775rem] px-2 text-2sm md:text-2sm"
							/>
							<Icon
								name="mingcute:lock-line"
								class="pointer-events-none absolute top-1/2 right-2 size-3.5 -translate-y-1/2 text-muted-foreground"
							/>
						</div>
						<div
							v-else
							role="group"
							:aria-labelledby="typeLabelId"
							class="flex gap-0.5 rounded-lg bg-muted p-0.5"
						>
							<button
								v-for="option in typeOptions"
								:key="option.type"
								type="button"
								:aria-pressed="value === option.type"
								:class="
									cn(
										'flex h-6 min-w-0 flex-auto cursor-pointer items-center justify-center gap-1 rounded-md px-1.5 text-xs whitespace-nowrap text-muted-foreground transition-colors duration-150 outline-none',
										value === option.type &&
											'bg-background font-semibold text-foreground shadow-xs',
									)
								"
								@click="handleChange(option.type)"
							>
								<Icon :name="option.icon" class="size-3 shrink-0" />
								{{ option.label }}
							</button>
						</div>
					</div>
				</ShadcnUiFormField>
				<ShadcnUiFormField
					v-if="type === 'list' && !builtin"
					v-slot="{ value, handleChange }"
					name="options"
				>
					<ShadcnUiFormItem class="gap-1">
						<span
							:id="optionsLabelId"
							class="text-2sm font-medium text-foreground"
						>
							{{ t("variable.variable-form.options-label") }}
						</span>
						<VariablesOptionsField
							:model-value="value"
							:label-id="optionsLabelId"
							@update:model-value="handleChange"
						/>
						<ShadcnUiFormMessage class="text-xs text-destructive-foreground" />
						<p class="text-xs text-muted-foreground">
							{{ t("variable.variable-form.options-hint") }}
						</p>
					</ShadcnUiFormItem>
				</ShadcnUiFormField>
				<ShadcnUiFormField
					v-else
					v-slot="{ componentField, value, handleChange }"
					:name="defaultFieldName"
				>
					<ShadcnUiFormItem class="gap-1">
						<ShadcnUiFormLabel
							disable-destructive-effect
							class="text-2sm font-medium text-foreground"
						>
							{{ t("variable.variable-form.default-label") }}
						</ShadcnUiFormLabel>
						<ShadcnUiSelect v-if="type === 'list'" v-bind="componentField">
							<ShadcnUiFormControl>
								<ShadcnUiSelectTrigger class="w-full text-2sm" size="custom">
									<ShadcnUiSelectValue />
								</ShadcnUiSelectTrigger>
							</ShadcnUiFormControl>
							<ShadcnUiSelectContent
								class="max-h-[40dvh]"
								side="bottom"
								align="start"
								inside-sheet
							>
								<ShadcnUiSelectItem
									v-for="option in variable?.options ?? []"
									:key="option.id"
									:value="option.id"
									class="text-2sm"
								>
									{{
										variable
											? optionLabel(variable, option.value, t)
											: option.value
									}}
								</ShadcnUiSelectItem>
							</ShadcnUiSelectContent>
						</ShadcnUiSelect>
						<ShadcnUiFormControl v-else-if="type === 'time-range'">
							<VariablesTimeRangeSelect v-bind="componentField" />
						</ShadcnUiFormControl>
						<ShadcnUiFormControl v-else-if="type === 'number'">
							<!--the field's own input handler would store the raw text, and
								the schema wants the number this input parses-->
							<NumberInput
								disable-focus-effect
								v-bind="NO_AUTOFILL"
								:model-value="value"
								decimal
								class="h-[1.775rem] px-2 text-2sm tabular-nums md:text-2sm"
								@update:model-value="handleChange"
							/>
						</ShadcnUiFormControl>
						<ShadcnUiFormControl v-else>
							<ShadcnUiTextarea
								v-bind="componentField"
								class="min-h-14 px-2 text-2sm focus-visible:border-input focus-visible:ring-0 md:text-2sm"
							/>
						</ShadcnUiFormControl>
						<ShadcnUiFormMessage class="text-xs text-destructive-foreground" />
						<p class="text-xs text-muted-foreground">
							{{
								type === "list" || type === "time-range"
									? t("variable.variable-form.default-preset-hint")
									: t("variable.variable-form.default-empty-hint")
							}}
						</p>
					</ShadcnUiFormItem>
				</ShadcnUiFormField>
			</div>
			<template v-if="variable && !builtin">
				<div class="px-4 pt-1 pb-2.5">
					<ShadcnUiSeparator />
				</div>
				<ShadcnUiButton
					type="button"
					variant="ghost-plain-destructive"
					size="custom"
					class="mx-4 h-7 gap-1.5 self-center text-2sm"
					@click="emit('delete', variable)"
				>
					<Icon name="mingcute:delete-2-line" class="size-3.5" />
					{{ t("variable.variable-form.delete-button") }}
				</ShadcnUiButton>
				<i18n-t
					scope="global"
					keypath="variable.variable-form.delete-hint"
					tag="p"
					class="px-4 text-center text-xs text-muted-foreground"
				>
					<template #reference>
						<span class="font-mono font-semibold break-all">
							{{ variableReference(variable.key) }}
						</span>
					</template>
				</i18n-t>
			</template>
		</form>
	</div>
</template>
