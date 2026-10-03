<script setup lang="ts">
import type { Component, ComputedRef } from "vue"
import GitHubTrackingConfigMenu from "./github-tracking/ConfigMenu.vue"
import ScheduledReminderConfigMenu from "./scheduled-reminder/ConfigMenu.vue"
import URLWatcherConfigMenu from "./url-watcher/ConfigMenu.vue"
import ContainerImageWatcherConfigMenu from "./container-image-watcher/ConfigMenu.vue"
import HookMenuFooter from "./HookMenuFooter.vue"
import HookMenuHeader from "./HookMenuHeader.vue"
import { compareAsc } from "date-fns"
import { DiffStatus } from "../diff/position-map"
import { diffHooks, type HookDiffEntry } from "./hook-diff"
import { HOOK_MENU_WIDTH_CLASS, HOOK_SUBMENU_WIDTH_CLASS } from "./hook-menu"
import { hookStatus } from "./hook-status"

const props = defineProps<{
	activeBranchHooks: DocumentHook[]
	// given only where the diff can be shown
	targetBranchHooks?: DocumentHook[]
	nodeId: string | null
}>()
const emit = defineEmits<{
	(e: "open-settings", target: "github"): void
}>()

const { gitHubConfigured } = useGitHubAPI()
const { isChangeDetectionEnabled } = useCapabilitiesAPI()
const { isReadOnlyOrDiff } = useEditorMeta()
const editorStore = useEditorStore()

const isSubOpen = ref(false)
const isDiffShown = computed(
	() => editorStore.reviewableDiffActive && !!props.targetBranchHooks,
)
// a sign marks a hook added or removed, so without one the rows need no
// room for it
const hasSigns = computed(
	() =>
		isDiffShown.value &&
		entries.value.some(
			(e) => e.status === DiffStatus.Added || e.status === DiffStatus.Removed,
		),
)

const entries = computed<HookDiffEntry[]>(() => {
	const activeHooks = props.activeBranchHooks.filter(
		(h) => h.blockId === props.nodeId,
	)
	const targetHooks = props.targetBranchHooks
	if (!editorStore.reviewableDiffActive || !targetHooks) {
		return activeHooks.map((h) => ({
			status: DiffStatus.Unchanged,
			hook: h,
			targetHook: null,
		}))
	}

	return diffHooks(
		activeHooks,
		targetHooks.filter((h) => h.blockId === props.nodeId),
	)
})
// a removed hook is gone from the branch, so it adds nothing to its status
const statuses = computed(() =>
	entries.value
		.filter((e) => e.status !== DiffStatus.Removed)
		.map((e) => hookStatus(e.hook)),
)

// rows keep the order their hooks were made in, and hooks made at the same
// moment go by id. Neither changes later, so a row never moves.
const listed = computed(() =>
	[...entries.value]
		.sort(
			(a, b) =>
				compareAsc(a.hook.createdAt, b.hook.createdAt) ||
				a.hook.id.localeCompare(b.hook.id),
		)
		.map((e) => ({
			id: e.hook.id,
			comp: hookComponent(e.hook.type),
			props: hookProps(e),
		})),
)

function hookComponent(type: DocumentHookType): Component | null {
	switch (type) {
		case DocumentHookType.ScheduledReminder:
			// eslint-disable-next-line @typescript-eslint/no-unsafe-return -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
			return ScheduledReminderConfigMenu
		case DocumentHookType.GitHubTracking:
			// eslint-disable-next-line @typescript-eslint/no-unsafe-return -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
			return GitHubTrackingConfigMenu
		case DocumentHookType.URLWatcher:
			// eslint-disable-next-line @typescript-eslint/no-unsafe-return -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
			return URLWatcherConfigMenu
		case DocumentHookType.ContainerImageWatcher:
			// eslint-disable-next-line @typescript-eslint/no-unsafe-return -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
			return ContainerImageWatcherConfigMenu
		default:
			return null
	}
}

function hookProps(entry: HookDiffEntry): ComputedRef<Record<string, unknown>> {
	return computed(() => ({
		nodeId: props.nodeId,
		hook: entry.hook,
		diff: isDiffShown.value
			? {
					status: entry.status,
					targetHook:
						entry.status === DiffStatus.Modified ? entry.targetHook : null,
					signColumn: hasSigns.value,
				}
			: null,
		...(entry.hook.type === DocumentHookType.GitHubTracking && {
			onOpenSettings: (target: "github") => {
				emit("open-settings", target)
			},
		}),
	}))
}
</script>
<template>
	<div :class="HOOK_MENU_WIDTH_CLASS">
		<HookMenuHeader :statuses="statuses" />
		<ShadcnUiDropdownMenuSeparator />
		<component
			:is="h.comp"
			v-for="h in listed"
			:key="h.id"
			v-bind="h.props.value"
		/>
		<ShadcnUiEmpty v-if="!entries.length" class="gap-0 p-3 md:p-3">
			<ShadcnUiEmptyHeader class="gap-1">
				<ShadcnUiEmptyMedia class="mb-1">
					<Icon
						name="mingcute:leaf-line"
						class="size-5 text-muted-foreground"
					/>
				</ShadcnUiEmptyMedia>
				<ShadcnUiEmptyTitle class="text-2sm">
					{{
						props.nodeId
							? $t("editor.hooks.empty-block")
							: $t("editor.hooks.empty-document")
					}}
				</ShadcnUiEmptyTitle>
				<!--
					the description ignores a class passed to it. So the text
					takes its size from a child
				-->
				<ShadcnUiEmptyDescription>
					<span class="block text-xs leading-4">
						{{
							props.nodeId
								? $t("editor.hooks.empty-block-description")
								: $t("editor.hooks.empty-document-description")
						}}
					</span>
				</ShadcnUiEmptyDescription>
			</ShadcnUiEmptyHeader>
		</ShadcnUiEmpty>
		<template v-if="!isReadOnlyOrDiff">
			<ShadcnUiDropdownMenuSeparator />
			<ShadcnUiDropdownMenuSub v-model:open="isSubOpen">
				<ShadcnUiDropdownMenuSubTrigger>
					<Icon name="mingcute:leaf-line" class="shrink-0" />
					<span>
						{{ $t("editor.hooks.add-new") }}
					</span>
				</ShadcnUiDropdownMenuSubTrigger>
				<ShadcnUiDropdownMenuSubContent
					side="right"
					align="start"
					loop
					:collision-padding="8"
					:class="HOOK_SUBMENU_WIDTH_CLASS"
				>
					<ScheduledReminderConfigMenu
						:node-id="nodeId"
						@force-close="isSubOpen = false"
					/>
					<GitHubTrackingConfigMenu
						v-if="gitHubConfigured"
						:node-id="nodeId"
						@force-close="isSubOpen = false"
						@open-settings="(target: 'github') => emit('open-settings', target)"
					/>
					<URLWatcherConfigMenu
						v-if="isChangeDetectionEnabled"
						:node-id="nodeId"
						@force-close="isSubOpen = false"
					/>
					<ContainerImageWatcherConfigMenu
						:node-id="nodeId"
						@force-close="isSubOpen = false"
					/>
				</ShadcnUiDropdownMenuSubContent>
			</ShadcnUiDropdownMenuSub>
		</template>
		<template v-else-if="editorStore.reviewableDiffActive">
			<ShadcnUiDropdownMenuSeparator />
			<HookMenuFooter mode="diff" />
		</template>
	</div>
</template>
