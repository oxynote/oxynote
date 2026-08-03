<script lang="ts" setup>
import { cn } from "~/lib/utils"

const props = defineProps<{
	allSectionsLoaded: boolean
	createLoading: boolean
}>()
const emit = defineEmits<{
	(e: "load-complete" | "create-document"): void
}>()

onMounted(() => {
	emit("load-complete")
})
</script>
<template>
	<Transition v-bind="defaultTransitionProps">
		<div
			v-show="props.allSectionsLoaded"
			class="flex flex-col items-center gap-4 pt-50"
		>
			<div class="flex max-w-80 flex-col gap-2 text-center">
				<h1 class="text-xl font-semibold text-foreground">
					{{ $t("editor.no-document-indicator.heading") }}
				</h1>
				<p class="text-base text-muted-foreground">
					{{ $t("editor.no-document-indicator.subheading") }}
				</p>
			</div>
			<ShadcnUiButton
				size="lg"
				:class="
					cn('h-[2.5rem] w-60', props.createLoading && 'pointer-events-none')
				"
				:disabled="props.createLoading"
				@click="emit('create-document')"
			>
				<Icon
					:name="
						props.createLoading
							? 'svg-spinners:blocks-shuffle-3'
							: 'mingcute:document-2-fill'
					"
					class="size-4"
				/>
				{{ $t("editor.no-document-indicator.create-document-button") }}
			</ShadcnUiButton>
		</div>
	</Transition>
</template>
