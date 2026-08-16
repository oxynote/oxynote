<script lang="ts" setup>
import type { DocumentSearchResult } from "~/utils"
import { extractDocumentTreeElement } from "./sidebar"

const open = defineModel<boolean>({ required: true })

const { t } = useI18n({ useScope: "global" })
const { searchDocuments, fetchDocumentTree } = useDocumentAPI()
const { fetchOrganization } = useAuthSession()
const resetLinkHighlightClearState = inject<() => void>(
	"resetLinkHighlightClearState",
)

const searchQuery = ref("")
const trimmedSearchQuery = computed(() => searchQuery.value.trim())
const debouncedSearchQuery = refDebounced(trimmedSearchQuery, 300)
const searchResults = ref<DocumentSearchResult[]>([])
const isSearching = ref(false)
const isTyping = computed(
	() => trimmedSearchQuery.value !== debouncedSearchQuery.value,
)
// ShadcnUiInput renders a bare <input>, so the ref resolves to the component
// instance whose $el is that element. Spelling the shape out keeps eslint's ts
// program — which cannot type .vue imports — from treating it as `any`, and
// keeps the focus fallback below type-checked
const inputElem = useTemplateRef<{
	$el?: HTMLInputElement
	focus?: () => void
}>("search-input")

watch(debouncedSearchQuery, async (query) => {
	if (!query) {
		searchResults.value = []
		isSearching.value = false

		return
	}

	isSearching.value = true

	try {
		const results = await searchDocuments(query)
		// eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- the core API serializes an empty result set as JSON null, which the declared array type does not admit
		searchResults.value = results || []
	} catch {
		searchResults.value = []
	} finally {
		isSearching.value = false
	}
})

watchImmediate(open, (open) => {
	if (open) {
		setTimeout(() => {
			const input = inputElem.value?.$el ?? inputElem.value
			if (input && typeof input.focus === "function") {
				input.focus()
			}
		}, 100)

		return
	}

	searchQuery.value = ""
	searchResults.value = []
	isSearching.value = false
})

function resultLink(result: DocumentSearchResult) {
	const orgName = fetchOrganization.data.value?.data?.name || ""
	const orgSlug = createNameSlug(orgName)

	const tree = fetchDocumentTree.data.value
	const doc = tree ? extractDocumentTreeElement(tree, result.documentId) : null
	const docSlug = doc
		? createNameSlugWithId(doc.documentName, result.documentId)
		: result.documentId

	let href = orgSlug ? `/${orgSlug}/${docSlug}` : `/${docSlug}`

	if (result.type !== "document") {
		href = href + `#${encodeURIComponent(result.id)}`
	}

	return href
}

function resultIcon(type: string) {
	switch (type) {
		case "document":
			return "lucide:file-text"
		case "heading":
			return "lucide:heading"
		default:
			return "lucide:text"
	}
}

function resultTypeText(type: string) {
	return type // TODO i18n
}
</script>
<template>
	<ShadcnUiDialog v-model:open="open">
		<ShadcnUiDialogContent
			class="top-[30%] flex max-h-[90dvh] w-150 max-w-[90dvw] translate-y-[-70%] flex-col overflow-hidden p-0"
			@interact-outside="open = false"
		>
			<ShadcnUiDialogTitle class="sr-only">
				{{ t("sidebar.search.title-screen-reader-hint") }}
			</ShadcnUiDialogTitle>
			<ShadcnUiDialogDescription class="sr-only">
				{{ t("sidebar.search.description-screen-reader-hint") }}
			</ShadcnUiDialogDescription>
			<div class="relative border-b p-3">
				<Icon
					name="lucide:search"
					class="absolute top-1/2 left-5 size-3.5 -translate-y-1/2 text-muted-foreground"
				/>
				<ShadcnUiInput
					ref="search-input"
					v-model="searchQuery"
					:placeholder="t('sidebar.search.input-placeholder')"
					class="py-2 pl-7 text-2base!"
					disable-focus-effect
					@keydown.esc="open = false"
				/>
			</div>
			<div class="flex max-h-full min-h-30 overflow-y-auto p-3">
				<div
					v-if="!trimmedSearchQuery"
					class="flex flex-1 flex-col items-center justify-center gap-3 text-center text-muted-foreground"
				>
					<Icon name="lucide:search" class="size-8 opacity-50" />
					<span class="text-2base">{{ t("sidebar.search.empty-state") }}</span>
				</div>
				<div
					v-else-if="isTyping || isSearching"
					class="flex flex-1 items-center justify-center text-muted-foreground"
				>
					<Icon
						name="svg-spinners:blocks-shuffle-3"
						class="size-6 opacity-50"
					/>
				</div>
				<div
					v-else-if="
						trimmedSearchQuery &&
						!isTyping &&
						!isSearching &&
						!searchResults.length
					"
					class="flex flex-1 flex-col items-center justify-center gap-3 text-center text-muted-foreground"
				>
					<Icon name="lucide:search-x" class="size-8 opacity-50" />
					<i18n-t
						keypath="sidebar.search.no-results"
						class="text-2base"
						tag="span"
					>
						<template #query>
							<span class="font-medium">{{ trimmedSearchQuery }}</span>
						</template>
					</i18n-t>
				</div>
				<div v-else class="flex w-full flex-col gap-2">
					<NuxtLink
						v-for="result in searchResults"
						:key="`${result.documentId}-${result.id}`"
						:href="resultLink(result)"
						:prefetch="false"
						class="flex w-full gap-1 rounded-md py-1 no-underline transition hover:bg-accent/50 active:bg-accent"
						@click="(resetLinkHighlightClearState?.(), (open = false))"
					>
						<Icon
							:name="resultIcon(result.type)"
							class="size-4 shrink-0 text-muted-foreground"
						/>
						<div class="flex flex-1 flex-col gap-0.5">
							<!-- eslint-disable vue/no-v-html -->
							<div
								class="text-2base font-medium text-foreground"
								v-html="sanitizeSearchResult(result.text)"
							/>
							<!-- eslint-enable vue/no-v-html -->
							<div class="text-2sm text-muted-foreground capitalize">
								{{ resultTypeText(result.type) }}
							</div>
						</div>
					</NuxtLink>
				</div>
			</div>
		</ShadcnUiDialogContent>
	</ShadcnUiDialog>
</template>
