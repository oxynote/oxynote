<script lang="ts" setup>
import { cn } from "~/lib/utils"

const props = defineProps<{
	type: "success" | "error" | "info" | "warning"
	title: string
	description?: string
}>()
const emit = defineEmits<{
	(event: "close"): void
}>()

const typeToIcon = {
	success: {
		icon: "mingcute:check-circle-fill",
		class: "text-status-success",
	},
	error: {
		icon: "mingcute:close-circle-fill",
		class: "text-status-error",
	},
	info: {
		icon: "mingcute:information-fill",
		class: "text-status-info",
	},
	warning: {
		icon: "mingcute:warning-fill",
		class: "text-status-warning",
	},
}

function closeToast() {
	emit("close")
}
</script>
<template>
	<div
		class="flex min-w-95 gap-1.75 rounded-lg border border-border bg-popover px-3 py-2.5 shadow-md"
	>
		<Icon
			:name="typeToIcon[props.type].icon"
			:class="cn('mt-px', typeToIcon[props.type].class)"
			size="1rem"
		/>
		<div class="flex flex-1 flex-col gap-0.5">
			<div class="text-2sm font-medium text-popover-foreground">
				{{ props.title }}
			</div>
			<div
				v-if="props.description"
				class="text-2sm font-normal text-muted-foreground"
			>
				{{ props.description }}
			</div>
		</div>
		<ShadcnUiButton
			variant="ghost-plain"
			class="mt-px size-4 p-0"
			@click="closeToast"
		>
			<Icon name="lucide:x" size="1rem" />
			<span class="sr-only">
				{{ $t("general.modal-close-screen-reader-hint") }}
			</span>
		</ShadcnUiButton>
	</div>
</template>
