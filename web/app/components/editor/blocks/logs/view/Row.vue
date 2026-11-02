<script lang="ts" setup>
import { cn } from "#layers/ui/app/lib/utils"
import HighlightedText from "./HighlightedText.vue"
import LevelPill from "./LevelPill.vue"
import RowDetails from "./RowDetails.vue"
import { displayValue, summarizeLine } from "../lines"
import type { LogFilter } from "../logql"
import { formatLogTime } from "../timestamp"
import type { LogTimestampFormat } from "../utils"

// the marks between a line's parts and between a key and its value
const PART_SEPARATOR = " · "
const PAIR_SEPARATOR = " "
const KEY_VALUE_SEPARATOR = "="

const props = defineProps<{
	line: LogLine
	timestampFormat: LogTimestampFormat
	search: string
}>()
const emit = defineEmits<{
	(e: "filter", filter: LogFilter): void
}>()
const expanded = defineModel<boolean>("expanded", { required: true })

const { d } = useI18n({ useScope: "global" })

const time = computed(() =>
	formatLogTime(props.line.time, props.timestampFormat, d),
)
const summary = computed(() => summarizeLine(props.line))
const groups = computed(() =>
	[summary.value.fields, summary.value.labels].filter((g) => g.length),
)
</script>
<template>
	<ShadcnUiCollapsible v-model:open="expanded">
		<ShadcnUiCollapsibleTrigger
			:class="
				cn(
					'group flex h-8 w-full cursor-pointer items-center gap-3 px-2.5 text-left hover:bg-muted',
					expanded && 'bg-muted',
				)
			"
		>
			<span v-if="time" class="shrink-0 text-xs whitespace-nowrap tabular-nums">
				<span class="text-muted-foreground/70">{{ time.date }}</span>
				<template v-if="time.date">
					{{ PAIR_SEPARATOR }}
				</template>
				<span class="text-muted-foreground">{{ time.time }}</span>
			</span>
			<LevelPill :level="props.line.level" />
			<span class="min-w-0 flex-1 truncate font-mono text-xs text-foreground">
				<HighlightedText :text="summary.message" :search="props.search" />
				<template v-for="(group, groupIndex) in groups" :key="groupIndex">
					<span
						v-if="summary.message || groupIndex > 0"
						class="text-muted-foreground/70"
					>
						{{ PART_SEPARATOR }}
					</span>
					<template v-for="([key, value], index) in group" :key="key">
						<template v-if="index > 0">{{ PAIR_SEPARATOR }}</template>
						<span class="text-muted-foreground">{{ key }}</span>
						<span class="text-muted-foreground/70">
							{{ KEY_VALUE_SEPARATOR }}
						</span>
						<HighlightedText
							:text="displayValue(value)"
							:search="props.search"
						/>
					</template>
				</template>
			</span>
			<Icon
				name="mingcute:right-line"
				:class="
					cn(
						'size-3.5 shrink-0 text-muted-foreground/70 opacity-0 transition-[opacity,transform] duration-150 group-hover:opacity-100',
						expanded && 'rotate-90 opacity-100',
					)
				"
			/>
		</ShadcnUiCollapsibleTrigger>
		<ShadcnUiCollapsibleContent
			class="overflow-hidden data-[state=closed]:animate-collapsible-up data-[state=open]:animate-collapsible-down"
		>
			<RowDetails
				:line="props.line"
				@filter="(filter) => emit('filter', filter)"
			/>
		</ShadcnUiCollapsibleContent>
	</ShadcnUiCollapsible>
</template>
