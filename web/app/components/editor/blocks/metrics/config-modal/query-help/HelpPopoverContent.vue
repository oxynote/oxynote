<script lang="ts" setup>
import MariaDBHelp from "./MariaDBHelp.vue"
import MySQLHelp from "./MySQLHelp.vue"
import PostgreSQLHelp from "./PostgreSQLHelp.vue"
import PrometheusHelp from "./PrometheusHelp.vue"

const props = defineProps<{
	dataSourceType: DataSourceType
}>()
</script>
<template>
	<div
		class="flex max-h-[min(30rem,50dvh)] w-full flex-col overflow-y-auto text-2sm leading-4.5"
	>
		<PrometheusHelp v-if="props.dataSourceType === DataSourceType.Prometheus" />
		<PostgreSQLHelp
			v-else-if="props.dataSourceType === DataSourceType.PostgreSQL"
		/>
		<MySQLHelp v-else-if="props.dataSourceType === DataSourceType.MySQL" />
		<MariaDBHelp v-else-if="props.dataSourceType === DataSourceType.MariaDB" />
	</div>
</template>
