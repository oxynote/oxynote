<script lang="ts" setup>
const editorStore = useEditorStore()
const { useFetchBranchViews } = useDocumentAPI()
const { fetchOrganization } = useAuthSession()
const fetchViews = useFetchBranchViews(
	() => editorStore.activeDocumentId,
	() => editorStore.activeBranchId,
)

// views of users who left the workspace have no member to name them
const viewers = computed(() =>
	(fetchViews.state.value.data ?? []).flatMap((view) => {
		const member = fetchOrganization.state.value.data?.data?.members.find(
			(m) => m.userId === view.userId,
		)
		if (!member) {
			return []
		}

		return [
			{
				id: member.userId,
				name: member.user.name,
				url: member.user.image,
				viewedAt: view.viewedAt,
			},
		]
	}),
)
</script>
<template>
	<div v-if="viewers.length" class="flex flex-col gap-1.5">
		<div class="font-bold">
			{{ $t("editor.navbar.timestamp.viewed-by") }}
		</div>
		<ul class="flex max-h-[min(40dvh,15rem)] flex-col gap-1 overflow-y-auto">
			<li
				v-for="viewer in viewers"
				:key="viewer.id"
				class="flex items-center gap-1.5"
			>
				<ShadcnUiAvatar class="size-5 border">
					<ShadcnUiAvatarImage
						v-if="viewer.url"
						:src="viewer.url"
						:alt="$t('settings.profile.image-alt')"
					/>
					<ShadcnUiAvatarFallback class="rounded-md text-2xs">
						<LazyDefaultAvatar kind="user" :seed="viewer.id" decorative />
					</ShadcnUiAvatarFallback>
				</ShadcnUiAvatar>
				<span class="min-w-0 flex-1 truncate">{{ viewer.name }}</span>
				<time :datetime="viewer.viewedAt" class="opacity-70">
					{{ $d(viewer.viewedAt, "month-day-short") }}
				</time>
			</li>
		</ul>
	</div>
</template>
