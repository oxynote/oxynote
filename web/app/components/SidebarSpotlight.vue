<script lang="ts" setup>
const props = defineProps<{
	title: string
	text: string
	action: { label: string; link: string; icon: string }
}>()
const emit = defineEmits<{
	(e: "dismiss" | "action"): void
}>()

const { t } = useI18n({ useScope: "global" })
</script>
<template>
	<div class="rounded-md border bg-card p-3 text-card-foreground">
		<!--the close button sits on the title's first line, so a title that
			wraps keeps it at the top-->
		<div class="flex items-start justify-between gap-2 text-2sm">
			<p class="font-semibold">
				{{ props.title }}
			</p>
			<div class="flex h-lh shrink-0 items-center">
				<ShadcnUiButton
					variant="ghost"
					size="icon-xsm"
					class="text-muted-foreground"
					@click="emit('dismiss')"
				>
					<Icon name="mingcute:close-line" class="size-3.5" />
					<span class="sr-only">
						{{ t("sidebar.spotlight.dismiss") }}
					</span>
				</ShadcnUiButton>
			</div>
		</div>
		<p class="mt-0.5 text-2sm text-pretty text-muted-foreground">
			{{ props.text }}
		</p>
		<ShadcnUiButton variant="outline" size="2sm" class="mt-3 w-full" as-child>
			<a
				:href="props.action.link"
				target="_blank"
				rel="noopener noreferrer"
				@click="emit('action')"
			>
				<Icon :name="props.action.icon" class="size-3.5" />
				{{ props.action.label }}
				<span class="sr-only">
					{{ t("sidebar.spotlight.new-tab-hint") }}
				</span>
			</a>
		</ShadcnUiButton>
	</div>
</template>
