<script lang="ts" setup>
const config = useRuntimeConfig()
</script>
<template>
	<i18n-t
		scope="global"
		keypath="editor.metrics.config.query-explanations.mariadb.main"
		tag="div"
	>
		<template #mariadb-queries>
			<span class="font-semibold">
				{{
					$t(
						"editor.metrics.config.query-explanations.mariadb.main-placeholders.mariadb-queries",
					)
				}}
			</span>
		</template>
		<template #time-column>
			<code class="inline-code">
				{{
					$t(
						"editor.metrics.config.query-explanations.mariadb.main-placeholders.time-column",
					)
				}}
			</code>
		</template>
		<template #numeric-column>
			<code class="font-semibold">
				{{
					$t(
						"editor.metrics.config.query-explanations.mariadb.main-placeholders.numeric-column",
					)
				}}
			</code>
		</template>
		<template #string-column>
			<code class="font-semibold">
				{{
					$t(
						"editor.metrics.config.query-explanations.mariadb.main-placeholders.string-column",
					)
				}}
			</code>
		</template>
		<template #service-column>
			<code class="inline-code">
				{{
					$t(
						"editor.metrics.config.query-explanations.mariadb.main-placeholders.service-column",
					)
				}}
			</code>
		</template>
		<template #environment-column>
			<code class="inline-code">
				{{
					$t(
						"editor.metrics.config.query-explanations.mariadb.main-placeholders.environment-column",
					)
				}}
			</code>
		</template>
		<template #where-keyword>
			<code class="inline-code">
				{{
					$t(
						"editor.metrics.config.query-explanations.mariadb.main-placeholders.where-keyword",
					)
				}}
			</code>
		</template>
		<template #group-by-keyword>
			<code class="inline-code">
				{{
					$t(
						"editor.metrics.config.query-explanations.mariadb.main-placeholders.group-by-keyword",
					)
				}}
			</code>
		</template>
		<template #select-keyword>
			<code class="inline-code">
				{{
					$t(
						"editor.metrics.config.query-explanations.mariadb.main-placeholders.select-keyword",
					)
				}}
			</code>
		</template>
		<template #docs-link>
			<a
				:href="config.public.mariadbQueryGuideURL"
				target="_blank"
				rel="noopener noreferrer"
				class="link-primary"
			>
				{{
					$t(
						"editor.metrics.config.query-explanations.mariadb.main-placeholders.docs-link",
					)
				}}
			</a>
		</template>
	</i18n-t>
	<div class="mt-2 font-semibold">
		{{ $t("editor.metrics.config.query-explanations.mariadb.examples-title") }}
	</div>
	<ul class="mt-1 flex list-disc flex-col gap-1 pl-4">
		<li>
			<div class="flex flex-col gap-1">
				<div class="font-semibold">
					{{
						$t(
							"editor.metrics.config.query-explanations.mariadb.examples.example-1.title",
						)
					}}
				</div>
				<code class="inline-code">
					{{
						$t(
							"editor.metrics.config.query-explanations.mariadb.examples.example-1.query",
						)
					}}
				</code>
				<i18n-t
					scope="global"
					keypath="editor.metrics.config.query-explanations.mariadb.examples.example-1.description"
					tag="div"
				>
					<template #result>
						<code class="inline-code">
							{{
								$t(
									"editor.metrics.config.query-explanations.mariadb.examples.example-1.placeholders.result",
								)
							}}
						</code>
					</template>
				</i18n-t>
			</div>
		</li>
		<li>
			<div class="flex flex-col gap-1">
				<div class="font-semibold">
					{{
						$t(
							"editor.metrics.config.query-explanations.mariadb.examples.example-2.title",
						)
					}}
				</div>
				<code class="inline-code">
					{{
						$t(
							"editor.metrics.config.query-explanations.mariadb.examples.example-2.query",
						)
					}}
				</code>
				<i18n-t
					scope="global"
					keypath="editor.metrics.config.query-explanations.mariadb.examples.example-2.description"
					tag="div"
				>
					<template #service-column>
						<code class="inline-code">
							{{
								$t(
									"editor.metrics.config.query-explanations.mariadb.examples.example-2.placeholders.service-column",
								)
							}}
						</code>
					</template>
					<template #result>
						<code class="inline-code">
							{{
								$t(
									"editor.metrics.config.query-explanations.mariadb.examples.example-2.placeholders.result",
								)
							}}
						</code>
					</template>
				</i18n-t>
			</div>
		</li>
	</ul>
</template>
