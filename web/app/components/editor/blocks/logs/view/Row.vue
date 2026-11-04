<script lang="ts" setup>
import { cn } from "#layers/ui/app/lib/utils"
import HighlightedText from "./HighlightedText.vue"
import LevelPill from "./LevelPill.vue"
import RowDetails from "./RowDetails.vue"
import UnsearchedPart from "./UnsearchedPart.vue"
import { displayValue, summarizeLog } from "../rows"
import type { LogFilter } from "../logql"
import { formatLogTime } from "../timestamp"
import type { LogTimestampFormat } from "../utils"

// the marks between a line's parts and between a key and its value
const PART_SEPARATOR = " · "
const PAIR_SEPARATOR = " "
const KEY_VALUE_SEPARATOR = "="

const props = defineProps<{
	log: Log
	timestampFormat: LogTimestampFormat
	search: string
}>()
const emit = defineEmits<{
	(e: "filter", filter: LogFilter): void
}>()
const expanded = defineModel<boolean>("expanded", { required: true })

const { d } = useI18n({ useScope: "global" })

const time = computed(() =>
	formatLogTime(props.log.time, props.timestampFormat, d),
)
const summary = computed(() => summarizeLog(props.log))
const searching = computed(() => props.search.trim().length > 0)
// the search runs on the line only, so only the message and the fields
// written in the line can match it. While a search is on, the other parts
// fade and a tooltip says why.
const groups = computed(() =>
	[
		{
			kind: "fields" as const,
			pairs: summary.value.fields,
			searched: summary.value.fieldsInLine,
		},
		{ kind: "labels" as const, pairs: summary.value.labels, searched: false },
	].filter((g) => g.pairs.length),
)
</script>
<template>
	<ShadcnUiCollapsible v-model:open="expanded">
		<ShadcnUiCollapsibleTrigger
			:class="
				cn(
					'group flex h-8 w-full cursor-pointer items-center gap-2 px-2.5 text-left hover:bg-muted',
					expanded && 'bg-muted',
				)
			"
		>
			<span
				v-if="time"
				class="shrink-0 text-2sm whitespace-nowrap tabular-nums"
			>
				<span class="text-muted-foreground/70">{{ time.date }}</span>
				<template v-if="time.date">
					{{ PAIR_SEPARATOR }}
				</template>
				<span class="text-muted-foreground">{{ time.time }}</span>
			</span>
			<LevelPill :level="props.log.level" />
			<span class="min-w-0 flex-1 truncate text-2sm text-foreground">
				<HighlightedText :text="summary.message" :search="props.search" />
				<template v-for="(group, groupIndex) in groups" :key="groupIndex">
					<span
						v-if="summary.message || groupIndex > 0"
						class="text-muted-foreground/70"
					>
						{{ PART_SEPARATOR }}
					</span>
					<component
						:is="searching && !group.searched ? UnsearchedPart : 'span'"
						v-bind="searching && !group.searched ? { kind: group.kind } : {}"
					>
						<template v-for="([key, value], index) in group.pairs" :key="key">
							<template v-if="index > 0">{{ PAIR_SEPARATOR }}</template>
							<span class="text-muted-foreground">
								<HighlightedText
									:text="key"
									:search="group.searched ? props.search : ''"
								/>
							</span>
							<span class="text-muted-foreground/70">
								{{ KEY_VALUE_SEPARATOR }}
							</span>
							<HighlightedText
								:text="displayValue(value)"
								:search="group.searched ? props.search : ''"
							/>
						</template>
					</component>
				</template>
			</span>
			<!-- the chevron takes no room while it is hidden, so the summary uses
			the full width until the row is hovered or open -->
			<Icon
				name="mingcute:right-line"
				:class="
					cn(
						'-ml-2 h-3.5 w-0 shrink-0 text-muted-foreground/70 opacity-0 transition-transform duration-150 group-hover:ml-0 group-hover:w-3.5 group-hover:opacity-100',
						expanded && 'ml-0 w-3.5 rotate-90 opacity-100',
					)
				"
			/>
		</ShadcnUiCollapsibleTrigger>
		<ShadcnUiCollapsibleContent
			class="overflow-hidden data-[state=closed]:animate-collapsible-up data-[state=open]:animate-collapsible-down"
		>
			<RowDetails
				:log="props.log"
				@filter="(filter) => emit('filter', filter)"
			/>
		</ShadcnUiCollapsibleContent>
	</ShadcnUiCollapsible>
</template>
