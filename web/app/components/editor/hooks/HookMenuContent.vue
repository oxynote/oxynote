<script setup lang="ts">
import GitHubTrackingConfigMenu from "./github-tracking/ConfigMenu.vue"
import ScheduledReminderConfigMenu from "./scheduled-reminder/ConfigMenu.vue"
import URLWatcherConfigMenu from "./url-watcher/ConfigMenu.vue"
import ContainerImageWatcherConfigMenu from "./container-image-watcher/ConfigMenu.vue"
import { compareAsc } from "date-fns"

const props = defineProps<{
	documentHooks: DocumentHook[]
	nodeId: string | null
}>()
const emit = defineEmits<{
	(e: "open-settings", target: "github"): void
}>()

const { gitHubConfigured } = useGitHubAPI()

const isSubOpen = ref(false)

const matchingActiveHooks = computed(() => {
	return props.documentHooks
		.filter((h) => {
			return h.blockId === props.nodeId && Number(h.score) !== 0
		})
		.sort((a, b) =>
			compareAsc(a.updatedAt ?? new Date(0), b.updatedAt ?? new Date(0)),
		)
		.map((h) => {
			return {
				id: h.id,
				comp: hookComponent(h.type),
				props: hookProps(h.type, h),
			}
		})
})
const matchingTriggeredHooks = computed(() => {
	return props.documentHooks
		.filter((h) => {
			return h.blockId === props.nodeId && Number(h.score) === 0
		})
		.sort((a, b) =>
			compareAsc(a.updatedAt ?? new Date(0), b.updatedAt ?? new Date(0)),
		)
		.map((h) => {
			return {
				id: h.id,
				comp: hookComponent(h.type),
				props: hookProps(h.type, h),
			}
		})
})

function hookComponent(type: string) {
	switch (type) {
		case DocumentHookType.ScheduledReminder:
			return ScheduledReminderConfigMenu
		case DocumentHookType.GitHubTracking:
			return GitHubTrackingConfigMenu
		case DocumentHookType.URLWatcher:
			return URLWatcherConfigMenu
		case DocumentHookType.ContainerImageWatcher:
			return ContainerImageWatcherConfigMenu
		default:
			return null
	}
}

function hookProps(type: string, hook: DocumentHook): any {
	switch (type) {
		case DocumentHookType.ScheduledReminder:
			return computed(() => ({
				nodeId: props.nodeId,
				hook: hook,
			}))
		case DocumentHookType.GitHubTracking:
			return computed(() => ({
				nodeId: props.nodeId,
				hook: hook,
				onOpenSettings: (target: "github") => emit("open-settings", target),
			}))
		case DocumentHookType.URLWatcher:
			return computed(() => ({
				nodeId: props.nodeId,
				hook: hook,
			}))
		case DocumentHookType.ContainerImageWatcher:
			return computed(() => ({
				nodeId: props.nodeId,
				hook: hook,
			}))
		default:
			return computed(() => ({}))
	}
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
			<ShadcnUiDropdownMenuSeparator />
		</template>
		<template v-if="matchingActiveHooks.length">
			<component
				:is="h.comp"
				v-for="h in matchingActiveHooks"
				:key="h.id"
				v-bind="h.props.value"
			/>
			<ShadcnUiDropdownMenuSeparator />
		</template>
		<ShadcnUiDropdownMenuSub v-model:open="isSubOpen">
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
