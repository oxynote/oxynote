<script lang="ts" setup>
import type { Editor } from "@tiptap/vue-3"

const props = defineProps<{
	editor: Editor
}>()

const emit = defineEmits<{
	(e: "close"): void
}>()

const linkUrl = ref("")

function createLink() {
	if (!linkUrl.value.trim()) {
		return
	}

	props.editor
		.chain()
		.focus()
		.setLink({ href: ensureHttps(linkUrl.value.trim()) })
		.run()

	// Reset and close
	linkUrl.value = ""
	emit("close")
}

function cancel() {
	linkUrl.value = ""
	emit("close")
}

function handleKeydown(event: KeyboardEvent) {
	if (event.key === "Enter") {
		event.preventDefault()
		createLink()
	} else if (event.key === "Escape") {
		event.preventDefault()
		cancel()
	}
}
</script>
<template>
	<div class="flex items-center gap-0.75">
		<ShadcnUiInput
			v-model="linkUrl"
			:placeholder="$t('editor.link.page-or-url-placeholder')"
			class="w-64 border-none px-2 text-2sm shadow-none md:text-2sm"
			autofocus
			disable-focus-effect
			@keydown="handleKeydown"
		/>
		<div class="h-full w-[0.0625rem] shrink-0 bg-border" />
		<ShadcnUiButton size="icon-sm" variant="ghost" @click="cancel">
			<Icon name="lucide:x" />
		</ShadcnUiButton>
	</div>
</template>
