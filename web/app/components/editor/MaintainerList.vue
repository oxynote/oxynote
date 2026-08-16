<script setup lang="ts">
import IconStack, { type IconMetadata } from "./IconStack.vue"

const { useFetchDocumentMaintainersByDocId } = useDocumentAPI()
const { fetchOrganization } = useAuthSession()
const editorStore = useEditorStore()
const fetchMaintainers = useFetchDocumentMaintainersByDocId(
	() => editorStore.activeDocumentId,
)
const wsState = useWebSocketStateStore()
let unsubWsMaintainersChange: (() => void) | null | undefined = null

watchImmediate(
	() => editorStore.activeDocumentId,
	(newV) => {
		if (!newV) {
			return
		}

		if (unsubWsMaintainersChange) {
			unsubWsMaintainersChange()
			unsubWsMaintainersChange = null
		}

		unsubWsMaintainersChange = wsState.state?.subscribe(
			makeWsDocumentMaintainersChangeTopic(newV),
			() => {
				void fetchMaintainers.refetch()
			},
		)
	},
)

const maintainers = computed<IconMetadata[]>(() => {
	return (
		fetchOrganization.state.value.data?.data?.members
			// filter only members whose ID is in maintainerIDs
			.filter((m) => fetchMaintainers.state.value.data?.includes(m.userId))
			.map((m) => ({
				id: m.userId,
				name: m.user.name,
				url: m.user.image,
			})) ?? []
	)
})
</script>

<template>
	<ShadcnUiPopover v-if="maintainers.length">
		<ShadcnUiPopoverTrigger as-child>
			<div class="flex">
				<IconStack
					:title="$t('editor.name-editor.maintainers')"
					clickable
					:icons="maintainers"
				/>
			</div>
		</ShadcnUiPopoverTrigger>
		<ShadcnUiPopoverContent
			v-if="maintainers.length"
			side="bottom"
			align="end"
			class="max-w-50 p-2"
		>
			<ul class="flex flex-col gap-1.5">
				<li
					v-for="maintainer in maintainers"
					:key="`${maintainer.id}`"
					class="flex items-center gap-1.5"
				>
					<ShadcnUiAvatar class="size-6 border">
						<ShadcnUiAvatarImage
							v-if="maintainer.url"
							:src="maintainer.url"
							:alt="$t('settings.profile.image-alt')"
						/>
						<ShadcnUiAvatarFallback v-else class="rounded-md text-2xs">
							{{ extractInitials(maintainer.name || "", 2) }}
						</ShadcnUiAvatarFallback>
					</ShadcnUiAvatar>
					<span class="truncate text-2sm text-foreground">
						{{ maintainer.name }}
					</span>
				</li>
			</ul>
		</ShadcnUiPopoverContent>
	</ShadcnUiPopover>
</template>
