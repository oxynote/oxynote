<script lang="ts" setup>
import type { Node as PMNode } from "@tiptap/pm/model"
import {
	collectAttributeChanges,
	displayValue,
	formatChange,
	type FieldChange,
} from "~/components/editor/diff/change-count"
import DiffBeforeAfter from "~/components/editor/diff/DiffBeforeAfter.vue"
import DiffFieldChanges from "~/components/editor/diff/DiffFieldChanges.vue"

const props = defineProps<{
	node: PMNode
}>()

const { t } = useI18n({ useScope: "global" })

const changes = computed(() => collectAttributeChanges(props.node))
const srcChange = computed(() =>
	changes.value.find((change) => change.name === "src"),
)
const rows = computed<FieldChange[]>(() => {
	const labels: Record<string, string> = {
		width: t("editor.figma.diff.width"),
		height: t("editor.figma.diff.height"),
	}

	return changes.value.flatMap((change) => {
		const label = labels[change.name]

		return label
			? [
					{
						label: label,
						...formatChange(
							change,
							pixels,
							t("editor.diff-change-marker.auto"),
						),
					},
				]
			: []
	})
})

function pixels(value: unknown): string {
	return t("editor.diff-change-marker.pixels", { value: displayValue(value) })
}

// a figma URL ends in a slug of the file's name, such as
// /design/<key>/Checkout-Flow. The raw URL stands in when it has none.
function describe(src: unknown) {
	if (typeof src !== "string") {
		return undefined
	}

	let name = src
	try {
		const slug = new URL(src).pathname.split("/").filter(Boolean)[2]
		if (slug) {
			name = decodeURIComponent(slug).replaceAll("-", " ")
		}
	} catch {
		// an unparsable URL keeps its raw text as the name
	}

	return { name: name, src: src }
}
</script>

<template>
	<div class="flex w-80 flex-col gap-3">
		<DiffBeforeAfter
			v-if="srcChange"
			v-slot="{ value }"
			:before="describe(srcChange.oldValue)"
			:after="describe(srcChange.newValue)"
			:empty-label="t('editor.figma.empty')"
		>
			<div class="flex w-full min-w-0 items-center gap-2 p-2">
				<Icon :name="BLOCK_ICONS.figmaBlock" class="size-4 shrink-0" />
				<span class="flex min-w-0 flex-col">
					<span class="line-clamp-6 text-xs font-medium break-words">
						{{ value.name }}
					</span>
					<span class="truncate text-xs text-muted-foreground">
						{{ value.src }}
					</span>
				</span>
			</div>
		</DiffBeforeAfter>
		<DiffFieldChanges v-if="rows.length" :rows="rows" />
	</div>
</template>
