<script setup lang="ts">
import type { Component, ComputedRef } from "vue"
import GitHubTrackingConfigMenu from "./github-tracking/ConfigMenu.vue"
import ScheduledReminderConfigMenu from "./scheduled-reminder/ConfigMenu.vue"
import URLWatcherConfigMenu from "./url-watcher/ConfigMenu.vue"
import ContainerImageWatcherConfigMenu from "./container-image-watcher/ConfigMenu.vue"
import { compareAsc } from "date-fns"
import { DiffStatus } from "../diff/position-map"
import { diffHooks, type HookDiffEntry } from "./hook-diff"

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

const matchingActiveHooks = computed(() =>
	listedHooks(entries.value.filter((e) => Number(e.hook.score) !== 0)),
)
const matchingTriggeredHooks = computed(() =>
	listedHooks(entries.value.filter((e) => Number(e.hook.score) === 0)),
)

function listedHooks(matching: HookDiffEntry[]) {
	return matching
		.sort((a, b) =>
			compareAsc(
				a.hook.updatedAt ?? new Date(0),
				b.hook.updatedAt ?? new Date(0),
			),
		)
		.map((e) => {
			return {
				id: e.hook.id,
				comp: hookComponent(e.hook.type),
				props: hookProps(e),
			}
		})
}

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
	<div class="max-w-[18rem]">
		<template v-if="matchingTriggeredHooks.length">
			<component
				:is="h.comp"
				v-for="h in matchingTriggeredHooks"
				:key="h.id"
				v-bind="h.props.value"
			/>
			<ShadcnUiDropdownMenuSeparator
				v-if="matchingActiveHooks.length || !isReadOnlyOrDiff"
			/>
		</template>
		<template v-if="matchingActiveHooks.length">
			<component
				:is="h.comp"
				v-for="h in matchingActiveHooks"
				:key="h.id"
				v-bind="h.props.value"
			/>
			<ShadcnUiDropdownMenuSeparator v-if="!isReadOnlyOrDiff" />
		</template>
		<div
			v-if="isReadOnlyOrDiff && !entries.length"
			class="px-2 py-1.25 text-xs text-muted-foreground"
		>
			{{
				props.nodeId
					? $t("editor.hooks.empty-block")
					: $t("editor.hooks.empty-document")
			}}
		</div>
		<ShadcnUiDropdownMenuSub v-if="!isReadOnlyOrDiff" v-model:open="isSubOpen">
			<ShadcnUiDropdownMenuSubTrigger>
				<Icon name="mingcute:leaf-line" class="shrink-0" />
				<span>
					{{ $t("editor.hooks.add-new") }}
				</span>
			</ShadcnUiDropdownMenuSubTrigger>
			<ShadcnUiDropdownMenuSubContent side="right" align="start" loop>
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
	</div>
</template>
