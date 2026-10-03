<script lang="ts" setup>
import { cn } from "~/lib/utils"
import DiffCountPill from "../diff/DiffCountPill.vue"
import { DiffStatus } from "../diff/position-map"
import { hookChangeCount, type HookDiffContext } from "./hook-diff"
import { HOOK_STATUS_DOT_CLASS, type HookStatus } from "./hook-status"

const props = defineProps<{
	hook?: DocumentHook | null
	// set only while the diff is shown
	diff?: HookDiffContext | null
	// null on a row that offers a new hook
	status?: HookStatus | null
	icon: string
}>()

const diffStatus = computed(() => props.diff?.status ?? null)
const count = computed(() =>
	props.hook && props.diff?.targetHook
		? hookChangeCount(props.hook, props.diff.targetHook)
		: null,
)

const rowClass = computed(() =>
	cn(
		"gap-2.5 py-1.5",
		// a tinted row deepens its own tint on hover, rather than taking the
		// grey the other rows use
		diffStatus.value === DiffStatus.Added &&
			"bg-diff-added/30 focus:bg-diff-added/50 data-[state=open]:bg-diff-added/50",
		diffStatus.value === DiffStatus.Removed &&
			"bg-diff-removed/30 focus:bg-diff-removed/50 data-[state=open]:bg-diff-removed/50",
	),
)
</script>

<template>
	<ShadcnUiDropdownMenuSubTrigger :class="rowClass">
		<!-- every row keeps the sign's column so the icons line up -->
		<span
			v-if="props.diff?.signColumn"
			aria-hidden="true"
			:class="
				cn(
					'mr-0.5 w-2 shrink-0 text-center font-semibold',
					diffStatus === DiffStatus.Added && 'text-diff-added-foreground',
					diffStatus === DiffStatus.Removed && 'text-diff-removed-foreground',
				)
			"
		>
			<template v-if="diffStatus === DiffStatus.Added">
				{{ $t("editor.hooks.diff.added-sign") }}
			</template>
			<template v-else-if="diffStatus === DiffStatus.Removed">
				{{ $t("editor.hooks.diff.removed-sign") }}
			</template>
		</span>
		<Icon :name="props.icon" class="size-3.75 shrink-0" />
		<span class="flex min-w-0 flex-1 flex-col">
			<span class="flex min-w-0 items-center gap-1.5 leading-4.25 font-medium">
				<span
					:class="
						cn(
							'truncate',
							diffStatus === DiffStatus.Removed &&
								'text-muted-foreground line-through',
						)
					"
				>
					<slot name="title" />
				</span>
				<span
					v-if="props.status"
					aria-hidden="true"
					:class="
						cn(
							'size-1.75 shrink-0 rounded-full',
							HOOK_STATUS_DOT_CLASS[props.status],
						)
					"
				/>
			</span>
			<span
				v-if="$slots.subtitle"
				class="text-xs leading-4 break-words text-muted-foreground"
			>
				<i18n-t
					v-if="$slots['subtitle-detail']"
					scope="global"
					keypath="editor.hooks.subtext-detail"
					tag="span"
				>
					<template #subtext>
						<slot name="subtitle" />
					</template>
					<template #detail>
						<slot name="subtitle-detail" />
					</template>
				</i18n-t>
				<slot v-else name="subtitle" />
			</span>
		</span>
		<span v-if="props.status === 'triggered'" class="sr-only">
			{{ $t("editor.hooks.row-status.triggered") }}
		</span>
		<span v-else-if="props.status === 'needs-attention'" class="sr-only">
			{{ $t("editor.hooks.row-status.needs-attention") }}
		</span>
		<span v-if="diffStatus === DiffStatus.Added" class="sr-only">
			{{ $t("editor.hooks.diff.added") }}
		</span>
		<span v-else-if="diffStatus === DiffStatus.Removed" class="sr-only">
			{{ $t("editor.hooks.diff.removed") }}
		</span>
		<DiffCountPill
			v-if="diffStatus === DiffStatus.Modified && count"
			:removed="count.removed"
			:added="count.added"
			class="shrink-0"
		/>
	</ShadcnUiDropdownMenuSubTrigger>
</template>
