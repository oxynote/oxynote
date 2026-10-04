<script lang="ts" setup>
import { cn } from "~/lib/utils"
import HookMenuFooter from "./HookMenuFooter.vue"
import HookNotice from "./HookNotice.vue"
import HookSubTrigger from "./HookSubTrigger.vue"
import { DiffStatus } from "../diff/position-map"
import type { HookDiffContext } from "./hook-diff"
import { HOOK_SUBMENU_WIDTH_CLASS } from "./hook-menu"
import { hookStatus, type HookStatus } from "./hook-status"

const props = defineProps<{
	hook?: DocumentHook | null // null/undefined means creating new
	// set only while the diff is shown
	diff?: HookDiffContext | null
	// the type's icon, on the row and on the notice
	icon: string
	// set when the notice tells about setup, like a missing github
	// connection, rather than about the hook
	noticeStatus?: HookStatus
	noticeIcon?: string
	// the button that dismisses a triggered hook, left out when the type
	// has no way to
	acknowledgeLabel?: string
	submitLabel?: string
	submitIcon?: string
	submitDisabled?: boolean
}>()
const emit = defineEmits<{
	(e: "submit" | "delete" | "acknowledge"): void
}>()
const open = defineModel<boolean>("open", { default: false })

const editorStore = useEditorStore()
const { isEditable } = useEditorMeta()

const status = computed(() => (props.hook ? hookStatus(props.hook) : null))
const isSettingUp = computed(() => props.hook?.status === "initializing")
const isRemoved = computed(() => props.diff?.status === DiffStatus.Removed)
const mode = computed(() => {
	if (editorStore.reviewableDiffActive) {
		return "diff"
	}

	if (!isEditable.value) {
		return "read-only"
	}

	return props.hook ? "edit" : "create"
})
const shownNoticeStatus = computed<HookStatus>(() => {
	if (isRemoved.value) {
		return "fresh"
	}

	return props.noticeStatus ?? status.value ?? "fresh"
})
const shownNoticeIcon = computed(() =>
	shownNoticeStatus.value === "needs-attention"
		? "mingcute:alert-fill"
		: (props.noticeIcon ?? props.icon),
)
// the diff shows two branches at once, so a hook is dismissed only once it
// is off
const canAcknowledge = computed(
	() => !!props.acknowledgeLabel && mode.value !== "diff",
)
const showsDismissHint = computed(
	() =>
		mode.value === "diff" && status.value === "triggered" && !isRemoved.value,
)
</script>

<template>
	<ShadcnUiDropdownMenuSub v-model:open="open">
		<HookSubTrigger
			:hook="props.hook"
			:diff="props.diff"
			:status="status"
			:icon="props.icon"
		>
			<template #title>
				<slot name="title" />
			</template>
			<template v-if="$slots.subtitle" #subtitle>
				<slot name="subtitle" />
			</template>
			<template v-if="$slots['subtitle-detail']" #subtitle-detail>
				<slot name="subtitle-detail" />
			</template>
		</HookSubTrigger>
		<!--
			a select inside turns pointer events off, and the sub menu then
			closes along with the select. So the content keeps them on
		-->
		<ShadcnUiDropdownMenuSubContent
			side="right"
			align="start"
			loop
			:collision-padding="8"
			class="pointer-events-auto!"
		>
			<div :class="cn('flex flex-col', HOOK_SUBMENU_WIDTH_CLASS)">
				<div class="flex flex-col gap-1.5 px-2 py-1.5">
					<p
						v-if="isSettingUp"
						class="flex items-center gap-1 text-2sm text-muted-foreground"
					>
						<Icon name="mingcute:loading-line" class="shrink-0 animate-spin" />
						{{ $t("editor.hooks.initializing") }}
					</p>
					<HookNotice
						:status="shownNoticeStatus"
						:icon="shownNoticeIcon"
						:action-label="canAcknowledge ? props.acknowledgeLabel : undefined"
						@action="emit('acknowledge')"
					>
						<template v-if="isRemoved">
							{{ $t("editor.hooks.removed") }}
						</template>
						<template v-else>
							<slot name="notice" />
							<i18n-t
								v-if="showsDismissHint"
								scope="global"
								keypath="editor.hooks.dismiss-in-diff"
								tag="p"
								class="mt-1"
							>
								<template #toggle>
									<span class="font-semibold">
										{{ $t("editor.name-editor.review-workflow.show-diff") }}
									</span>
								</template>
							</i18n-t>
						</template>
					</HookNotice>
					<slot />
				</div>
				<ShadcnUiDropdownMenuSeparator />
				<HookMenuFooter
					:mode="mode"
					:submit-label="props.submitLabel"
					:submit-icon="props.submitIcon"
					:submit-disabled="props.submitDisabled"
					@submit="emit('submit')"
					@delete="emit('delete')"
				/>
			</div>
		</ShadcnUiDropdownMenuSubContent>
	</ShadcnUiDropdownMenuSub>
</template>
