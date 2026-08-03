<script lang="ts" setup>
import { cn } from "~/lib/utils"

const emit = defineEmits<{
	(e: "data-source-creation", type: DataSourceType): void
	(e: "data-source-update" | "data-source-removal", ds: DataSource): void
}>()

const { fetchDataSources } = useDataSourceAPI()
const { t } = useI18n({ useScope: "global" })

const sections = computed(() => {
	const res = [
		{
			key: DataSourceType.Prometheus,
			icon: "devicon:prometheus",
			items:
				fetchDataSources.state.value.data?.filter(
					(ds) =>
						ds.type === DataSourceType.Prometheus &&
						ds.status !== DataSourceStatus.LocalOptimisticInsert,
				) || [],
			create: () => emit("data-source-creation", DataSourceType.Prometheus),
			update: (dataSource: DataSource) =>
				emit("data-source-update", dataSource),
			delete: (dataSource: DataSource) =>
				emit("data-source-removal", dataSource),
		},
		{
			key: DataSourceType.PostgreSQL,
			icon: "devicon:postgresql",
			items:
				fetchDataSources.state.value.data?.filter(
					(ds) =>
						ds.type === DataSourceType.PostgreSQL &&
						ds.status !== DataSourceStatus.LocalOptimisticInsert,
				) || [],
			create: () => emit("data-source-creation", DataSourceType.PostgreSQL),
			update: (dataSource: DataSource) =>
				emit("data-source-update", dataSource),
			delete: (dataSource: DataSource) =>
				emit("data-source-removal", dataSource),
		},
		{
			key: DataSourceType.MySQL,
			icon: "devicon:mysql",
			items:
				fetchDataSources.state.value.data?.filter(
					(ds) =>
						ds.type === DataSourceType.MySQL &&
						ds.status !== DataSourceStatus.LocalOptimisticInsert,
				) || [],
			create: () => emit("data-source-creation", DataSourceType.MySQL),
			update: (dataSource: DataSource) =>
				emit("data-source-update", dataSource),
			delete: (dataSource: DataSource) =>
				emit("data-source-removal", dataSource),
		},
		{
			key: DataSourceType.MariaDB,
			icon: "devicon:mariadb",
			items:
				fetchDataSources.state.value.data?.filter(
					(ds) =>
						ds.type === DataSourceType.MariaDB &&
						ds.status !== DataSourceStatus.LocalOptimisticInsert,
				) || [],
			create: () => emit("data-source-creation", DataSourceType.MariaDB),
			update: (dataSource: DataSource) =>
				emit("data-source-update", dataSource),
			delete: (dataSource: DataSource) =>
				emit("data-source-removal", dataSource),
		},
	]

	return res
})
</script>
<template>
	<div class="flex flex-col">
		<template v-for="(section, index) in sections" :key="section.key">
			<div v-if="index > 0" class="my-3.5 h-px w-full bg-border" />
			<div
				v-if="!section.items.length"
				class="flex w-full items-center justify-between gap-2"
			>
				<div class="flex items-center gap-2">
					<Icon :name="section.icon" class="size-6.5 shrink-0" />
					<div class="flex flex-col">
						<div class="text-sm">
							{{ $t(`settings.data-sources.${section.key}.title`) }}
						</div>
						<div
							class="hidden text-2base text-muted-foreground md:block md:text-2sm"
						>
							{{ $t(`settings.data-sources.${section.key}.description`) }}
						</div>
					</div>
				</div>
				<div>
					<ShadcnUiButton
						type="button"
						variant="ghost-plain"
						class="gap-1.5 p-0 text-sm"
						@click="section.create()"
					>
						<Icon name="lucide:external-link" />
						{{ $t(`settings.data-sources.${section.key}.connect`) }}
					</ShadcnUiButton>
				</div>
			</div>
			<div v-else class="flex w-full flex-col gap-0.75">
				<div class="flex items-center justify-between">
					<div class="text-2base">
						{{ $t(`settings.data-sources.${section.key}.title`) }}
					</div>
					<ShadcnUiButton
						type="button"
						variant="ghost-plain"
						:class="cn('gap-1.5 p-0 text-sm')"
						@click="section.create()"
					>
						<Icon name="lucide:circle-plus" />
						{{ $t(`settings.data-sources.${section.key}.connect`) }}
					</ShadcnUiButton>
				</div>
				<table class="w-full table-fixed">
					<colgroup>
						<col class="w-auto md:w-25 lg:w-30" />
						<col class="hidden md:table-column md:w-9 lg:w-14" />
						<col class="w-5 lg:w-7" />
					</colgroup>
					<tbody class="divide-y divide-border/70 dark:divide-border/50">
						<tr v-for="item in section.items" :key="item.id">
							<td class="py-2">
								<div class="flex min-w-0 items-center gap-2">
									<ShadcnUiTooltip>
										<ShadcnUiTooltipTrigger as-child>
											<DataSourceStatusIcon :data-source="item" />
										</ShadcnUiTooltipTrigger>
										<ShadcnUiTooltipContent side="bottom">
											{{
												t(
													`settings.data-sources.${section.key}.status-messages.${item.status}`,
												)
											}}
										</ShadcnUiTooltipContent>
									</ShadcnUiTooltip>
									<div class="flex min-w-0 flex-col">
										<div class="truncate text-sm">
											{{ item.name }}
										</div>
										<div class="truncate text-2sm text-muted-foreground">
											{{ item.url }}
										</div>
									</div>
								</div>
							</td>
							<td class="hidden md:table-cell">
								<div class="flex min-w-0 items-center gap-2">
									<div class="flex min-w-0 flex-col">
										<div class="text-2sm text-muted-foreground">
											{{
												$t(
													`settings.data-sources.${section.key}.connected-label`,
												)
											}}
										</div>
										<div class="text-2sm text-muted-foreground">
											{{ $d(new Date(item.createdAt), "short") }}
										</div>
									</div>
								</div>
							</td>
							<td>
								<div
									class="flex h-full items-center justify-end whitespace-nowrap"
								>
									<ShadcnUiDropdownMenu>
										<ShadcnUiDropdownMenuTrigger as-child>
											<ShadcnUiButton
												type="button"
												variant="ghost"
												size="icon"
												:class="cn('size-5')"
											>
												<Icon name="lucide:ellipsis" />
												<span class="sr-only">
													{{
														$t(
															`settings.data-sources.${section.key}.option-button-screen-reader-hint`,
														)
													}}
												</span>
											</ShadcnUiButton>
										</ShadcnUiDropdownMenuTrigger>
										<ShadcnUiDropdownMenuContent
											side="bottom"
											align="end"
											loop
											inside-modal
										>
											<ShadcnUiDropdownMenuItem @click="section.update(item)">
												<Icon name="lucide:settings" />
												<span>
													{{
														$t(
															`settings.data-sources.${section.key}.options.update.title`,
														)
													}}
												</span>
											</ShadcnUiDropdownMenuItem>
											<ShadcnUiDropdownMenuItem @click="section.delete(item)">
												<Icon name="lucide:trash-2" />
												<span>
													{{
														$t(
															`settings.data-sources.${section.key}.options.delete.title`,
														)
													}}
												</span>
											</ShadcnUiDropdownMenuItem>
										</ShadcnUiDropdownMenuContent>
									</ShadcnUiDropdownMenu>
								</div>
							</td>
						</tr>
					</tbody>
				</table>
			</div>
		</template>
	</div>
</template>
