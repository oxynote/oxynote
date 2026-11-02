<script lang="ts" setup>
import { cn } from "#layers/ui/app/lib/utils"

const search = defineModel<string>({ required: true })

const input = useTemplateRef<HTMLInputElement>("input")
// a search with text stays open, so the header always shows it is applied
const open = ref(!!search.value)

async function openSearch() {
	open.value = true
	await nextTick()
	input.value?.focus()
}

// the first escape clears the text and the next one closes the field
function handleEscape() {
	if (search.value) {
		search.value = ""

		return
	}

	open.value = false
}

function handleBlur() {
	if (!search.value) {
		open.value = false
	}
}

function clear() {
	search.value = ""
	input.value?.focus()
}
</script>
<template>
	<div
		:class="
			cn(
				'flex h-6 shrink-0 items-center overflow-hidden rounded-md transition-[width] duration-150',
				open ? 'w-65 max-w-full border bg-card' : 'w-6',
			)
		"
	>
		<ShadcnUiButton
			v-if="!open"
			variant="ghost-plain"
			size="icon-sm"
			class="size-6"
			@click="openSearch"
		>
			<Icon name="mingcute:search-line" />
			<span class="sr-only">{{ $t("editor.logs.view.search-label") }}</span>
		</ShadcnUiButton>
		<template v-else>
			<Icon
				name="mingcute:search-line"
				class="ml-1.5 size-3.5 shrink-0 text-muted-foreground"
			/>
			<input
				ref="input"
				v-model="search"
				type="text"
				:aria-label="$t('editor.logs.view.search-label')"
				:placeholder="$t('editor.logs.view.search-placeholder')"
				class="min-w-0 flex-1 bg-transparent px-1.5 font-mono text-xs text-foreground caret-primary outline-none select-text placeholder:text-muted-foreground"
				@keydown.esc.prevent.stop="handleEscape"
				@keydown.enter.prevent
				@blur="handleBlur"
			/>
			<button
				v-if="search"
				type="button"
				class="mr-1 flex size-4 shrink-0 cursor-pointer items-center justify-center rounded-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
				@mousedown.prevent
				@click="clear"
			>
				<Icon name="mingcute:close-line" class="size-3" />
				<span class="sr-only">{{ $t("editor.logs.view.clear-search") }}</span>
			</button>
		</template>
	</div>
</template>
