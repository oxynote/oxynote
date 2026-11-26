<script lang="ts" setup>
import { cn } from "#layers/ui/app/lib/utils"

const props = withDefaults(
	defineProps<{
		dataSource: DataSource | null
		size?: "6.5" | "8.5"
	}>(),
	{
		size: "8.5",
	},
)
const icon = computed((): { name: string; class?: string } => {
	if (!props.dataSource) {
		return { name: "devicon:prometheus" }
	}

	switch (props.dataSource.type) {
		case DataSourceType.Prometheus:
			return { name: "devicon:prometheus" }
		case DataSourceType.VictoriaMetrics:
			return {
				name: "simple-icons:victoriametrics",
				class: "text-data-source-victoriametrics",
			}
		case DataSourceType.Loki:
			return { name: "custom-icons:loki" }
		case DataSourceType.VictoriaLogs:
			return { name: "custom-icons:victoria-logs" }
		case DataSourceType.PostgreSQL:
			return { name: "devicon:postgresql" }
		case DataSourceType.Supabase:
			return { name: "devicon:supabase" }
		case DataSourceType.MySQL:
			return { name: "devicon:mysql" }
		case DataSourceType.MariaDB:
			return { name: "devicon:mariadb" }
		default:
			// a newer server may send a type this build does not know
			return { name: "devicon:prometheus" }
	}
})
</script>
<template>
	<div
		v-if="!props.dataSource"
		:class="
			cn(
				'shrink-0 rounded-full border-2 border-dashed border-border',
				props.size === '6.5' ? 'size-6.5' : 'size-8.5',
			)
		"
	/>
	<div
		v-else
		:class="
			cn(
				'relative shrink-0 rounded-full border-2 border-border',
				props.size === '6.5' ? 'size-6.5' : 'size-8.5',
				props.dataSource.status === DataSourceStatus.Success
					? 'border-status-success'
					: 'border-status-error',
			)
		"
	>
		<Icon
			:name="icon.name"
			:class="
				cn(
					'size-full shrink-0 rounded-full border border-background',
					icon.class,
				)
			"
		/>
		<div
			:class="
				cn(
					'absolute flex size-3 items-center justify-center rounded-full border border-background',
					props.size === '8.5'
						? '-right-0.5 -bottom-0.5'
						: '-right-0.75 -bottom-0.75',
					props.dataSource.status === DataSourceStatus.Success
						? 'bg-status-success'
						: 'bg-status-error',
				)
			"
		>
			<Icon
				:name="
					props.dataSource.status === DataSourceStatus.Success
						? 'lucide:check'
						: 'lucide:x'
				"
				class="size-2 text-background"
			/>
		</div>
	</div>
</template>
