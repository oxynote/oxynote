<script
	setup
	lang="ts"
	generic="T extends { toDate(tz: string): Date } | undefined"
>
import type { DateValue } from "@internationalized/date"
import { getLocalTimeZone } from "@internationalized/date"
import { isBefore, startOfTomorrow } from "date-fns"
import { cn } from "~/lib/utils"

const props = withDefaults(
	defineProps<{
		placeholder: string
		iconClass?: string
		buttonClass?: string
		buttonSize?: "default" | "2sm" | "sm" | "lg" | "custom"
		availableFromTomorrow?: boolean
		modelValue?: T
	}>(),
	{
		iconClass: cn("size-3.5"),
		buttonClass: cn("text-2sm px-1.5 py-1.25"),
		buttonSize: "custom",
	},
)
const emit = defineEmits<{
	"update:modelValue": [value: T]
}>()
const value = computed({
	get: () => props.modelValue as DateValue | undefined,
	set: (v) => emit("update:modelValue", v as T),
})
const open = ref(false)

function isDateValueUnavailable(date: DateValue) {
	if (props.availableFromTomorrow) {
		const tomorrow = startOfTomorrow()
		return isBefore(date.toDate(getLocalTimeZone()), tomorrow)
	}

	return false
}

watch(value, () => {
	open.value = false
})
</script>

<template>
	<ShadcnUiPopover v-model:open="open">
		<ShadcnUiPopoverTrigger as-child>
			<ShadcnUiButton
				variant="outline"
				:size="props.buttonSize"
				:class="
					cn(
						'items-center justify-start gap-1 text-left font-normal',
						props.buttonClass,
						!value && 'text-muted-foreground',
					)
				"
			>
				<Icon name="lucide:calendar" :class="props.iconClass" />
				<span class="mt-[0.05rem]">
					{{
						value
							? $d(value.toDate(getLocalTimeZone()), "long")
							: props.placeholder
					}}
				</span>
			</ShadcnUiButton>
		</ShadcnUiPopoverTrigger>
		<ShadcnUiPopoverContent class="w-auto p-0">
			<ShadcnUiCalendar
				v-model="value"
				initial-focus
				:is-date-disabled="isDateValueUnavailable"
			/>
		</ShadcnUiPopoverContent>
	</ShadcnUiPopover>
</template>
