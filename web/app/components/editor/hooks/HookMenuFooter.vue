<script lang="ts" setup>
const props = withDefaults(
	defineProps<{
		mode: "create" | "edit" | "read-only" | "diff"
		// replaces the default label of the button that saves an existing hook
		submitLabel?: string
		submitIcon?: string
		submitDisabled?: boolean
	}>(),
	{
		submitLabel: undefined,
		submitIcon: "mingcute:save-2-line",
	},
)
const emit = defineEmits<{
	(e: "submit" | "delete"): void
}>()

const { isBranchProtected } = useEditorMeta()
</script>

<template>
	<div v-if="props.mode === 'create'" class="px-2 py-1.5">
		<ShadcnUiButton
			class="w-full gap-1"
			size="2sm"
			:disabled="props.submitDisabled"
			@click.stop="emit('submit')"
		>
			<Icon name="mingcute:check-fill" />
			{{ $t("editor.hooks.create") }}
		</ShadcnUiButton>
	</div>
	<div v-else-if="props.mode === 'edit'" class="flex gap-1.5 px-2 py-1.5">
		<ShadcnUiButton
			class="flex-1 gap-1"
			size="2sm"
			:disabled="props.submitDisabled"
			@click.stop="emit('submit')"
		>
			<Icon :name="props.submitIcon" />
			{{ props.submitLabel ?? $t("editor.hooks.update") }}
		</ShadcnUiButton>
		<ShadcnUiButton
			class="flex-1 gap-1"
			variant="secondary"
			size="2sm"
			@click.stop="emit('delete')"
		>
			<Icon name="mingcute:delete-2-line" />
			{{ $t("editor.hooks.delete") }}
		</ShadcnUiButton>
	</div>
	<div
		v-else
		class="flex items-start gap-1.5 px-2 py-1.5 text-xs leading-4.5 text-muted-foreground"
	>
		<Icon
			:name="
				props.mode === 'read-only'
					? 'mingcute:lock-line'
					: 'mingcute:git-compare-line'
			"
			class="mt-0.5 size-3.5 shrink-0"
		/>
		<!--
			the line wraps to the menu's width. Without a width of its own, it
			would stretch the menu instead
		-->
		<span v-if="props.mode === 'read-only'" class="w-0 flex-1">
			{{
				isBranchProtected
					? $t("editor.hooks.read-only-protected")
					: $t("editor.hooks.read-only-mode")
			}}
		</span>
		<i18n-t
			v-else
			scope="global"
			keypath="editor.hooks.showing-changes"
			tag="span"
			class="w-0 flex-1"
		>
			<template #toggle>
				<span class="font-semibold">
					{{ $t("editor.name-editor.review-workflow.show-diff") }}
				</span>
			</template>
		</i18n-t>
	</div>
</template>
