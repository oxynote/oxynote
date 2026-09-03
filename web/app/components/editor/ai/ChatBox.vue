<script lang="ts" setup>
import DOMPurify from "dompurify"
import MarkdownIt from "markdown-it"

const props = defineProps<{
	mobile?: boolean
}>()
const emit = defineEmits<{
	(event: "close-chat-box"): void
}>()

const { t } = useI18n({ useScope: "global" })
const inputText = ref("")
const messageContainer = ref<HTMLElement | null>(null)
const markdown = new MarkdownIt({
	html: false,
	linkify: false,
	breaks: true,
})

const editorStore = useEditorStore()

const {
	messages,
	isConnected,
	isStreaming,
	toolStatus,
	streamingText,
	pendingConfirm,
	connect,
	disconnect,
	sendMessage,
	setActiveDocument,
	answerConfirm,
	resetChat,
} = useAIChat()

// the model resolves "this document" from whatever the user has open,
// branch included, so every navigation is reported.
watch(
	() => [editorStore.activeDocumentId, editorStore.activeBranchId] as const,
	([id, branchId]) => {
		setActiveDocument(id, branchId)
	},
	{ immediate: true },
)

const defaultLinkRenderer =
	markdown.renderer.rules.link_open ??
	((tokens, idx, options, _env, self) => self.renderToken(tokens, idx, options))

markdown.renderer.rules.link_open = (tokens, idx, options, env, self) => {
	const token = tokens[idx]
	if (!token) {
		return defaultLinkRenderer(tokens, idx, options, env, self)
	}

	token.attrSet("target", "_blank")
	token.attrSet("rel", "noopener noreferrer")

	return defaultLinkRenderer(tokens, idx, options, env, self)
}

function sanitizeMarkdownHtml(html: string): string {
	if (typeof window === "undefined") {
		return html.replace(/<[^>]*>/g, "")
	}

	return DOMPurify.sanitize(html)
}

function renderMarkdown(text: string): string {
	if (!text) {
		return ""
	}

	return sanitizeMarkdownHtml(markdown.render(text))
}

function handleSend() {
	const text = inputText.value.trim()
	if (!text || isStreaming.value || pendingConfirm.value) return

	sendMessage(text)
	inputText.value = ""
}

function handleKeydown(e: KeyboardEvent) {
	if (e.key === "Enter" && !e.shiftKey) {
		e.preventDefault()
		handleSend()
	}
}

watch(
	() => messages.value.length,
	() => {
		void nextTick(() => {
			if (messageContainer.value) {
				messageContainer.value.scrollTop = messageContainer.value.scrollHeight
			}
		})
	},
)

// Also scroll when the last message text updates (streaming)
watch(
	() => messages.value[messages.value.length - 1]?.text,
	() => {
		void nextTick(() => {
			if (messageContainer.value) {
				messageContainer.value.scrollTop = messageContainer.value.scrollHeight
			}
		})
	},
)

onMounted(() => {
	connect()
})
onUnmounted(() => {
	disconnect()
})
</script>
<template>
	<aside class="flex h-full max-h-svh shrink-0 flex-col">
		<header
			class="top-0 z-navbar h-10 shrink-0 border-b border-border pr-3 pl-4"
		>
			<div class="flex h-full w-full items-center justify-between gap-4">
				<div class="flex items-center gap-2 text-sm">
					<Icon name="lucide:sparkles" class="size-4" />
					{{ t("editor.ai-chat.title") }}
				</div>
				<div class="flex items-center gap-2.5">
					<span v-if="isConnected" class="size-1.5 rounded-full bg-green-500" />
					<span v-else class="size-1.5 rounded-full bg-red-500" />
					<template v-if="props.mobile">
						<div class="w-px self-stretch bg-border" />
						<ShadcnUiButton
							size="icon-sm"
							variant="ghost"
							class="shrink-0"
							@click.stop="emit('close-chat-box')"
						>
							<Icon name="lucide:x" />
						</ShadcnUiButton>
					</template>
				</div>
			</div>
		</header>
		<div
			ref="messageContainer"
			class="flex flex-1 flex-col gap-3 overflow-y-auto p-3"
			:class="{
				'justify-center': !messages.length,
			}"
		>
			<div
				v-if="!messages.length"
				class="mx-auto flex flex-col items-center gap-2 px-5 py-10 text-center"
			>
				<div class="text-base text-muted-foreground">
					{{ t("editor.ai-chat.title") }}
				</div>
				<div class="text-sm text-muted-foreground">
					{{ t("editor.ai-chat.empty-state-description") }}
				</div>
			</div>
			<template v-else>
				<div
					v-for="(msg, i) in messages"
					:key="i"
					class="flex"
					:class="{
						'justify-end': msg.role === ChatMessageRole.User,
						'justify-start': msg.role === ChatMessageRole.Assistant,
					}"
				>
					<div
						class="max-w-[85%] min-w-0 rounded-lg px-3 py-2 text-sm"
						:class="{
							'bg-primary text-primary-foreground':
								msg.role === ChatMessageRole.User,
							'bg-muted text-foreground':
								msg.role === ChatMessageRole.Assistant,
							'bg-destructive/10 text-destructive':
								msg.role === ChatMessageRole.Error,
						}"
					>
						<!-- eslint-disable vue/no-v-html -->
						<div
							class="prose prose-sm max-w-none [overflow-wrap:anywhere] break-words prose-headings:my-2 prose-p:my-2 prose-p:break-words prose-a:break-all prose-a:text-current prose-a:underline prose-a:decoration-current/60 hover:prose-a:decoration-current prose-code:text-inherit prose-code:before:content-none prose-code:after:content-none prose-pre:my-2 prose-pre:overflow-x-auto prose-pre:bg-black/10 prose-ol:my-2 prose-ul:my-2 [&_code]:break-words"
							:class="{
								'[--tw-prose-body:theme(colors.primary.foreground)] [--tw-prose-bold:theme(colors.primary.foreground)] [--tw-prose-bullets:theme(colors.primary.foreground)] [--tw-prose-code:theme(colors.primary.foreground)] [--tw-prose-counters:theme(colors.primary.foreground)] [--tw-prose-headings:theme(colors.primary.foreground)]':
									msg.role === ChatMessageRole.User,
							}"
							v-html="renderMarkdown(msg.text)"
						/>
						<!-- eslint-enable vue/no-v-html -->
					</div>
				</div>
			</template>
			<div v-if="streamingText" class="flex justify-start">
				<div
					class="max-w-[85%] min-w-0 rounded-lg bg-muted px-3 py-2 text-sm text-foreground"
				>
					<!-- eslint-disable vue/no-v-html -->
					<div
						class="prose prose-sm max-w-none [overflow-wrap:anywhere] break-words prose-headings:my-2 prose-p:my-2 prose-p:break-words prose-a:break-all prose-a:text-current prose-a:underline prose-code:text-inherit prose-code:before:content-none prose-code:after:content-none prose-pre:my-2 prose-pre:overflow-x-auto prose-ol:my-2 prose-ul:my-2"
						v-html="renderMarkdown(streamingText)"
					/>
					<!-- eslint-enable vue/no-v-html -->
				</div>
			</div>
			<div v-if="pendingConfirm" class="flex flex-col gap-2">
				<div class="rounded-lg border border-border bg-background p-3">
					<div class="mb-2 flex items-center gap-2 text-sm font-medium">
						<Icon name="lucide:shield-question" class="size-4" />
						{{ t("editor.ai-chat.confirm.title") }}
					</div>
					<ul class="mb-3 flex flex-col gap-1.5">
						<li
							v-for="(action, i) in pendingConfirm.actions"
							:key="i"
							class="flex flex-col text-sm"
						>
							<span class="text-foreground">{{ action.summary }}</span>
							<span
								v-if="action.documentName"
								class="text-xs text-muted-foreground"
							>
								{{ action.documentName }}
							</span>
						</li>
					</ul>
					<div class="flex flex-wrap gap-2">
						<ShadcnUiButton size="2sm" @click="answerConfirm(true)">
							{{ t("editor.ai-chat.confirm.approve") }}
						</ShadcnUiButton>
						<ShadcnUiButton
							size="2sm"
							variant="outline"
							@click="answerConfirm(true, true)"
						>
							{{ t("editor.ai-chat.confirm.approve-all") }}
						</ShadcnUiButton>
						<ShadcnUiButton
							size="2sm"
							variant="ghost"
							@click="answerConfirm(false)"
						>
							{{ t("editor.ai-chat.confirm.decline") }}
						</ShadcnUiButton>
					</div>
				</div>
			</div>
			<div v-if="toolStatus" class="flex justify-start">
				<div
					class="rounded-lg bg-muted px-3 py-2 text-xs text-muted-foreground italic"
				>
					{{ toolStatus }}
				</div>
			</div>
			<div
				v-if="isStreaming && !toolStatus && !streamingText"
				class="flex justify-start"
			>
				<div class="flex gap-1 rounded-lg bg-muted px-3 py-2">
					<span
						class="size-1.5 animate-bounce rounded-full bg-muted-foreground [animation-delay:-0.3s]"
					/>
					<span
						class="size-1.5 animate-bounce rounded-full bg-muted-foreground [animation-delay:-0.15s]"
					/>
					<span
						class="size-1.5 animate-bounce rounded-full bg-muted-foreground"
					/>
				</div>
			</div>
		</div>
		<div class="border-t border-border p-3">
			<div class="flex flex-col gap-2">
				<textarea
					v-model="inputText"
					class="flex-1 resize-none rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:ring-1 focus-visible:ring-ring focus-visible:outline-none"
					:placeholder="t('editor.ai-chat.input-placeholder')"
					rows="2"
					:disabled="!isConnected || isStreaming || !!pendingConfirm"
					@keydown="handleKeydown"
				/>
				<div class="flex justify-between">
					<ShadcnUiButton
						size="2sm"
						variant="outline"
						:disabled="!isConnected || isStreaming || !!pendingConfirm"
						@click="resetChat"
					>
						{{ t("editor.ai-chat.new-chat") }}
					</ShadcnUiButton>
					<ShadcnUiButton
						size="2sm"
						:disabled="
							!inputText.trim() ||
							!isConnected ||
							isStreaming ||
							!!pendingConfirm
						"
						@click="handleSend"
					>
						{{ t("editor.ai-chat.send") }}
					</ShadcnUiButton>
				</div>
			</div>
		</div>
	</aside>
</template>
