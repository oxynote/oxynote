<script lang="ts" setup>
import type { DocumentSearchDocument } from "~/utils"

const props = defineProps<{
	document: Pick<DocumentSearchDocument, "title" | "titleHtml" | "icon">
	href: string
	breadcrumbs: string[]
	meta: string
	metaTooltip: string
	branchLabel: string | null
	current: boolean
}>()
const { t } = useI18n({ useScope: "global" })
const breadcrumbLine = useTemplateRef("breadcrumb-line")
const { width: breadcrumbLineWidth } = useElementSize(breadcrumbLine)

const TAG_CLASS =
	"flex h-4.5 shrink-0 items-center gap-1 rounded bg-accent px-1.5 text-2xs font-semibold text-muted-foreground"

const collapseStep = ref(0)
const breadcrumbPath = computed(() => props.breadcrumbs.join("/"))
// the ways to write the path, from the full one to the shortest. The
// middle steps go first, then the workspace.
const breadcrumbCandidates = computed(() => {
	const crumbs = props.breadcrumbs
	const collapsed = t("sidebar.search.breadcrumb-collapsed")
	const candidates = [crumbs]

	for (let kept = crumbs.length - 2; kept >= 1; kept--) {
		candidates.push([...crumbs.slice(0, 1), collapsed, ...crumbs.slice(-kept)])
	}

	if (crumbs.length > 1) {
		candidates.push([collapsed, ...crumbs.slice(-1)])
	}

	return candidates
})
const visibleBreadcrumbs = computed(
	() => breadcrumbCandidates.value[collapseStep.value] ?? [],
)

watch([breadcrumbLineWidth, breadcrumbPath], () => {
	collapseStep.value = 0
})

// runs after each render of the line and takes the next shorter path
// while the line still overflows. The last path is cut off by css.
watch(
	[breadcrumbLineWidth, breadcrumbPath, collapseStep],
	() => {
		const line = breadcrumbLine.value
		if (
			line &&
			line.scrollWidth > line.clientWidth &&
			collapseStep.value < breadcrumbCandidates.value.length - 1
		) {
			collapseStep.value++
		}
	},
	{ flush: "post" },
)
</script>
<template>
	<NuxtLink
		:href="props.href"
		:prefetch="false"
		class="flex items-start gap-1.25 p-2"
	>
		<div class="flex size-5 shrink-0 items-center justify-center">
			<Icon :name="props.document.icon" class="size-4 text-foreground" />
		</div>
		<div class="flex min-w-0 flex-1 flex-col gap-0.5">
			<div class="flex h-5 min-w-0 items-center gap-2">
				<!-- eslint-disable vue/no-v-html -->
				<div
					v-if="props.document.titleHtml"
					class="min-w-0 flex-1 truncate text-2sm font-semibold text-foreground [&_mark]:rounded-xs [&_mark]:bg-comment-highlight/40 [&_mark]:text-inherit"
					v-html="sanitizeSearchResult(props.document.titleHtml)"
				/>
				<!-- eslint-enable vue/no-v-html -->
				<div
					v-else
					class="min-w-0 flex-1 truncate text-2sm font-semibold text-foreground"
				>
					{{ props.document.title }}
				</div>
				<div v-if="props.current" :class="TAG_CLASS">
					{{ t("sidebar.search.current-page") }}
				</div>
				<ShadcnUiTooltip :delay-duration="300">
					<ShadcnUiTooltipTrigger as-child>
						<div class="shrink-0 text-2xs text-muted-foreground/70">
							{{ props.meta }}
						</div>
					</ShadcnUiTooltipTrigger>
					<ShadcnUiTooltipContent side="bottom" align="center">
						{{ props.metaTooltip }}
					</ShadcnUiTooltipContent>
				</ShadcnUiTooltip>
			</div>
			<div
				class="flex min-w-0 items-center gap-0.75 text-xs text-muted-foreground/70"
			>
				<div v-if="props.branchLabel" :class="TAG_CLASS">
					<Icon name="mingcute:git-branch-line" class="size-3" />
					{{ props.branchLabel }}
				</div>
				<div ref="breadcrumb-line" class="min-w-0 flex-1 truncate">
					<template v-for="(crumb, index) in visibleBreadcrumbs" :key="index">
						<span v-if="index" class="mx-1 text-muted-foreground/40">
							{{ t("sidebar.search.breadcrumb-separator") }}
						</span>
						<span>{{ crumb }}</span>
					</template>
				</div>
			</div>
		</div>
	</NuxtLink>
</template>
