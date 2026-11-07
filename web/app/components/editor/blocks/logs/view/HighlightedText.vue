<script lang="ts" setup>
import { highlightParts, messageParts, type MessagePart } from "../rows"

const props = defineProps<{
	text: string
	search: string
	colourKeys?: boolean
}>()

// a plain line colours its logfmt keys, so the message reads like the
// pairs after it. JSON lines and pair keys and values keep their own
// rendering.
const parts = computed<MessagePart[]>(() =>
	props.colourKeys
		? messageParts(props.text, props.search)
		: highlightParts(props.text, props.search).map((part) => ({
				...part,
				kind: "plain" as const,
			})),
)
</script>
<template>
	<template v-for="(part, index) in parts" :key="index">
		<span
			v-if="part.kind !== 'plain'"
			:class="
				part.kind === 'key'
					? 'text-muted-foreground'
					: 'text-muted-foreground/70'
			"
		>
			<mark
				v-if="part.match"
				class="rounded-sm bg-search-highlight text-inherit"
			>
				{{ part.text }}
			</mark>
			<template v-else>{{ part.text }}</template>
		</span>
		<mark
			v-else-if="part.match"
			class="rounded-sm bg-search-highlight text-inherit"
		>
			{{ part.text }}
		</mark>
		<template v-else>{{ part.text }}</template>
	</template>
</template>
