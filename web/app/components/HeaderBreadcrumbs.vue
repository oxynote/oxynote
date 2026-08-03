<script lang="ts" setup>
import { cn } from "~/lib/utils"
import type { DocumentBreadcrumb } from "./sidebar"

/*
	This component shows 3 breadcrumb elements on >1024px screens, 2 on
	<1024px, and 1 on <550px
*/

const props = defineProps<{
	breadcrumbs: DocumentBreadcrumb[]
}>()

const processedBreadcrumbs = computed(() => {
	if (!props.breadcrumbs.length) {
		return null
	}

	switch (props.breadcrumbs.length) {
		case 1:
			return {
				first: null,
				collapsed: false,
				preLast: null,
				last: props.breadcrumbs[0]!,
			}
		case 2:
			return {
				first: props.breadcrumbs[0]!,
				collapsed: false,
				preLast: null,
				last: props.breadcrumbs[1]!,
			}
		case 3:
			return {
				first: props.breadcrumbs[0]!,
				collapsed: false,
				preLast: props.breadcrumbs[1]!,
				last: props.breadcrumbs[2]!,
			}
		default:
			return {
				first: props.breadcrumbs[0]!,
				collapsed: true,
				preLast: props.breadcrumbs[props.breadcrumbs.length - 2]!,
				last: props.breadcrumbs[props.breadcrumbs.length - 1]!,
			}
	}
})
</script>

<template>
	<ShadcnUiBreadcrumb v-if="processedBreadcrumbs" class="min-w-0">
		<ShadcnUiBreadcrumbList class="min-w-0">
			<template v-if="processedBreadcrumbs.first">
				<div
					class="hidden min-w-0 shrink-0 items-center min-[550px]:inline-flex"
				>
					<ShadcnUiBreadcrumbItem class="min-w-0">
						<ShadcnUiBreadcrumbLink as-child>
							<NuxtLink
								:href="processedBreadcrumbs.first.href"
								prefetch
								:prefetch-on="{ interaction: true, visibility: false }"
							>
								<div
									class="flex max-w-[9.5rem] min-w-0 items-center gap-1 lg:max-w-[12.5rem]"
								>
									<Icon
										class="shrink-0"
										:name="processedBreadcrumbs.first.icon"
									/>
									<span class="truncate text-sm">
										{{ processedBreadcrumbs.first.name }}
									</span>
								</div>
							</NuxtLink>
						</ShadcnUiBreadcrumbLink>
					</ShadcnUiBreadcrumbItem>
					<ShadcnUiBreadcrumbSeparator class="ml-1" />
				</div>
				<div
					v-if="processedBreadcrumbs.preLast || processedBreadcrumbs.collapsed"
					:data-collapsed-exist="
						processedBreadcrumbs.collapsed ? '' : undefined
					"
					:data-pre-last-exists="processedBreadcrumbs.preLast ? '' : undefined"
					:class="
						cn(
							'hidden items-center',
							'shrink-0 min-[550px]:data-collapsed-exist:inline-flex',
							'shrink-0 min-[550px]:max-lg:data-pre-last-exists:inline-flex',
						)
					"
				>
					<ShadcnUiBreadcrumbEllipsis />
					<ShadcnUiBreadcrumbSeparator class="ml-1" />
				</div>
			</template>
			<div
				v-if="processedBreadcrumbs.preLast"
				class="hidden min-w-0 shrink-0 lg:inline-flex"
			>
				<ShadcnUiBreadcrumbItem class="min-w-0">
					<ShadcnUiBreadcrumbLink as-child>
						<NuxtLink
							:href="processedBreadcrumbs.preLast.href"
							prefetch
							:prefetch-on="{ interaction: true, visibility: false }"
						>
							<div
								class="flex max-w-[9.5rem] min-w-0 items-center gap-1 lg:max-w-[12.5rem]"
							>
								<Icon
									class="shrink-0"
									:name="processedBreadcrumbs.preLast.icon"
								/>
								<span class="truncate text-sm">
									{{ processedBreadcrumbs.preLast.name }}
								</span>
							</div>
						</NuxtLink>
					</ShadcnUiBreadcrumbLink>
				</ShadcnUiBreadcrumbItem>
				<ShadcnUiBreadcrumbSeparator class="ml-1" />
			</div>
			<ShadcnUiBreadcrumbItem class="min-w-0">
				<ShadcnUiBreadcrumbPage class="min-w-0">
					<div class="flex min-w-0 items-center gap-1">
						<Icon class="shrink-0" :name="processedBreadcrumbs.last.icon" />
						<span class="truncate text-sm">
							{{ processedBreadcrumbs.last.name }}
						</span>
					</div>
				</ShadcnUiBreadcrumbPage>
			</ShadcnUiBreadcrumbItem>
		</ShadcnUiBreadcrumbList>
	</ShadcnUiBreadcrumb>
</template>
