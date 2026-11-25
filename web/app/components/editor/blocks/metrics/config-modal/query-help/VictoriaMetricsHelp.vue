<script lang="ts" setup>
const config = useRuntimeConfig()
</script>
<template>
	<i18n-t
		scope="global"
		keypath="editor.metrics.config.query-explanations.victoriametrics.main"
		tag="div"
	>
		<template #metricsql-queries>
			<span class="font-semibold">
				{{
					$t(
						"editor.metrics.config.query-explanations.victoriametrics.main-placeholders.metricsql-queries",
					)
				}}
			</span>
		</template>
		<template #label-filter>
			<code class="inline-code">
				{{
					$t(
						"editor.metrics.config.query-explanations.victoriametrics.main-placeholders.label-filter",
					)
				}}
			</code>
		</template>
		<template #range-selector>
			<code class="inline-code">
				{{
					$t(
						"editor.metrics.config.query-explanations.victoriametrics.main-placeholders.range-selector",
					)
				}}
			</code>
		</template>
		<template #rate-function>
			<code class="inline-code">
				{{
					$t(
						"editor.metrics.config.query-explanations.victoriametrics.main-placeholders.rate-function",
					)
				}}
			</code>
		</template>
		<template #official-docs>
			<a
				:href="config.public.victoriametricsQueryGuideURL"
				target="_blank"
				rel="noopener noreferrer"
				class="link-primary"
			>
				{{
					$t(
						"editor.metrics.config.query-explanations.victoriametrics.main-placeholders.official-docs",
					)
				}}
			</a>
		</template>
	</i18n-t>
	<div class="mt-2 font-semibold">
		{{
			$t(
				"editor.metrics.config.query-explanations.victoriametrics.examples-title",
			)
		}}
	</div>
	<ul class="mt-1 flex list-disc flex-col gap-1 pl-4">
		<i18n-t
			scope="global"
			keypath="editor.metrics.config.query-explanations.victoriametrics.examples.example-1.description"
			tag="li"
		>
			<template #query>
				<code class="inline-code">
					{{
						$t(
							"editor.metrics.config.query-explanations.victoriametrics.examples.example-1.placeholders.query",
						)
					}}
				</code>
			</template>
			<template #method>
				<code class="inline-code">
					{{
						$t(
							"editor.metrics.config.query-explanations.victoriametrics.examples.example-1.placeholders.method",
						)
					}}
				</code>
			</template>
			<template #service>
				<code class="inline-code">
					{{
						$t(
							"editor.metrics.config.query-explanations.victoriametrics.examples.example-1.placeholders.service",
						)
					}}
				</code>
			</template>
		</i18n-t>
		<i18n-t
			scope="global"
			keypath="editor.metrics.config.query-explanations.victoriametrics.examples.example-2.description"
			tag="li"
		>
			<template #query>
				<code class="inline-code">
					{{
						$t(
							"editor.metrics.config.query-explanations.victoriametrics.examples.example-2.placeholders.query",
						)
					}}
				</code>
			</template>
			<template #service>
				<code class="inline-code">
					{{
						$t(
							"editor.metrics.config.query-explanations.victoriametrics.examples.example-2.placeholders.service",
						)
					}}
				</code>
			</template>
		</i18n-t>
	</ul>
</template>
