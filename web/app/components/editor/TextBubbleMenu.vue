<script lang="ts" setup>
import { BubbleMenu } from "@tiptap/vue-3/menus"
import type { Editor } from "@tiptap/vue-3"
import type { Editor as CEditor } from "@tiptap/core"
import type { EditorState } from "@tiptap/pm/state"
import type { EditorView } from "@tiptap/pm/view"
import { isTextSelection } from "@tiptap/core"
import {
	isBubbleMenuItemAllowedByContext,
	shouldShowBubbleMenu,
} from "./bubble-menu"
import Bold from "@tiptap/extension-bold"
import Code from "@tiptap/extension-code"
import Italic from "@tiptap/extension-italic"
import Strike from "@tiptap/extension-strike"
import Blockquote from "@tiptap/extension-blockquote"
import Underline from "@tiptap/extension-underline"
import Link from "@tiptap/extension-link"
import { COMMENT_MARK_NAME } from "./mark-names"
import { ref } from "vue"
import LinkCreateMenu from "./link/LinkCreateMenu.vue"
import { CODE_BLOCK_NAME } from "./blocks/node-names"
import type { PositionMap } from "./diff/position-map"

const props = defineProps<{
	editor: Editor
	container: HTMLElement | null
	/** when provided, the bubble menu operates in comment-only diff mode */
	diffContext?: {
		positionMap: PositionMap
	}
}>()
const emit = defineEmits<{
	(e: "add-thread" | "edit-link"): void
}>()

const isDiffMode = computed(() => !!props.diffContext)
const showBubbleMenu = ref(false)
const showLinkCreateMenu = ref(false)

function handleLinkButtonClick() {
	// Check if the selection already has a link
	if (props.editor.isActive(Link.name)) {
		emit("edit-link")
	} else {
		showLinkCreateMenu.value = true
	}
}

function closeLinkCreateMenu() {
	showLinkCreateMenu.value = false
}

function isInsideCommentMark(state: EditorState): boolean {
	const markType = state.schema.marks[COMMENT_MARK_NAME]
	if (!markType) {
		return false
	}
	const { $from } = state.selection
	return !!markType.isInSet($from.marks())
}

function shouldShowDiffBubbleMenu(opts: {
	editor: CEditor
	state: EditorState
	view: EditorView
	from: number
	to: number
}): boolean {
	const { editor, view, state, from, to } = opts
	const { doc, selection } = state
	const { empty } = selection

	const isEmptyTextBlock =
		!doc.textBetween(from, to).length || !isTextSelection(selection)

	if (!view.hasFocus() || empty || isEmptyTextBlock || !editor.isEditable) {
		return false
	}

	if (isInsideCommentMark(state)) {
		return false
	}

	return true
}

function shouldShowBubbleMenuWrapped(opts: {
	editor: CEditor
	state: EditorState
	view: EditorView
	from: number
	to: number
}): boolean {
	if (isDiffMode.value) {
		return shouldShowDiffBubbleMenu(opts)
	}

	const shouldShow = shouldShowBubbleMenu(opts)

	if (!shouldShow) {
		closeLinkCreateMenu()
	}

	return shouldShow
}

function showMenu() {
	showBubbleMenu.value = true
}

function hideMenu() {
	showBubbleMenu.value = false
	closeLinkCreateMenu()
}
</script>
<template>
	<BubbleMenu
		v-if="props.editor"
		:editor="props.editor"
		:should-show="shouldShowBubbleMenuWrapped"
		:update-delay="250"
		:options="{
			strategy: 'absolute',
			placement: 'top',
			offset: {
				mainAxis: 5,
			},
			flip: {
				boundary: props.container || undefined,
				fallbackPlacements: ['top-start', 'top-end'],
				padding: 8,
			},
			shift: {
				boundary: props.container || undefined,
				padding: 8,
			},
			onShow: showMenu,
			onHide: hideMenu,
		}"
		class="z-5"
	>
		<div
			:data-visible="showBubbleMenu ? '' : undefined"
			class="flex gap-0.75 rounded-lg border border-border bg-background p-0.75 opacity-0 shadow-xs transition-opacity duration-150 data-visible:opacity-100"
		>
			<!-- diff mode: comment-only -->
			<template v-if="isDiffMode">
				<ShadcnUiButton
					size="custom"
					variant="ghost"
					class="h-6.5 gap-1 px-2 text-2sm font-medium"
					@click="emit('add-thread')"
				>
					<Icon name="lucide:message-square-text" />
					{{ $t("editor.bubble-menu.comment-label") }}
				</ShadcnUiButton>
			</template>
			<!-- normal mode: full formatting toolbar -->
			<template v-else>
				<LinkCreateMenu
					v-if="showLinkCreateMenu"
					:editor="props.editor"
					@close="closeLinkCreateMenu"
				/>
				<template v-else>
					<ShadcnUiButton
						v-if="
							isBubbleMenuItemAllowedByContext(props.editor.state, Bold.name)
						"
						size="icon-sm"
						variant="ghost"
						:active="props.editor.isActive(Bold.name)"
						@click="props.editor.chain().focus().toggleBold().run()"
					>
						<Icon name="lucide:bold" />
					</ShadcnUiButton>
					<ShadcnUiButton
						v-if="
							isBubbleMenuItemAllowedByContext(props.editor.state, Italic.name)
						"
						size="icon-sm"
						variant="ghost"
						:active="props.editor.isActive(Italic.name)"
						@click="props.editor.chain().focus().toggleItalic().run()"
					>
						<Icon name="lucide:italic" />
					</ShadcnUiButton>
					<ShadcnUiButton
						v-if="
							isBubbleMenuItemAllowedByContext(
								props.editor.state,
								Underline.name,
							)
						"
						size="icon-sm"
						variant="ghost"
						:active="props.editor.isActive(Underline.name)"
						@click="props.editor.chain().focus().toggleUnderline().run()"
					>
						<Icon name="lucide:underline" />
					</ShadcnUiButton>
					<ShadcnUiButton
						v-if="
							isBubbleMenuItemAllowedByContext(props.editor.state, Strike.name)
						"
						size="icon-sm"
						variant="ghost"
						:active="props.editor.isActive(Strike.name)"
						@click="props.editor.chain().focus().toggleStrike().run()"
					>
						<Icon name="lucide:strikethrough" />
					</ShadcnUiButton>
					<ShadcnUiButton
						v-if="
							isBubbleMenuItemAllowedByContext(props.editor.state, Link.name)
						"
						size="icon-sm"
						variant="ghost"
						:active="props.editor.isActive(Link.name)"
						@click="handleLinkButtonClick"
					>
						<Icon name="lucide:link" />
					</ShadcnUiButton>
					<ShadcnUiButton
						v-if="
							isBubbleMenuItemAllowedByContext(
								props.editor.state,
								Blockquote.name,
							)
						"
						size="icon-sm"
						variant="ghost"
						:active="props.editor.isActive(Blockquote.name)"
						@click="props.editor.chain().focus().toggleBlockquote().run()"
					>
						<Icon name="lucide:quote" />
					</ShadcnUiButton>
					<ShadcnUiButton
						v-if="
							isBubbleMenuItemAllowedByContext(props.editor.state, Code.name)
						"
						size="icon-sm"
						variant="ghost"
						:active="props.editor.isActive(Code.name)"
						@click="props.editor.chain().focus().toggleCode().run()"
					>
						<Icon name="lucide:square-code" />
					</ShadcnUiButton>
					<ShadcnUiButton
						v-if="
							isBubbleMenuItemAllowedByContext(
								props.editor.state,
								CODE_BLOCK_NAME,
							)
						"
						size="icon-sm"
						variant="ghost"
						:active="props.editor.isActive(CODE_BLOCK_NAME)"
						@click="
							() => {
								const editor = props.editor

								if (!editor) {
									return
								}

								const { from, to } = editor.state.selection
								const selectedText = editor.state.doc.textBetween(
									from,
									to,
									'\n',
								)

								editor
									.chain()
									.focus()
									.insertContent({
										type: CODE_BLOCK_NAME,
										content: selectedText
											? [{ type: 'text', text: selectedText }]
											: [],
									})
									.run()
							}
						"
					>
						<Icon name="lucide:code" />
					</ShadcnUiButton>
					<template
						v-if="
							isBubbleMenuItemAllowedByContext(
								props.editor.state,
								COMMENT_MARK_NAME,
							)
						"
					>
						<div
							class="w-[0.0625rem] shrink-0 self-stretch bg-border first:hidden"
						/>
						<ShadcnUiButton
							size="custom"
							variant="ghost"
							class="h-6.5 gap-1 px-2 text-2sm font-medium"
							@click="$emit('add-thread')"
						>
							<Icon name="lucide:message-square-text" />
							{{ $t("editor.bubble-menu.comment-label") }}
						</ShadcnUiButton>
					</template>
				</template>
			</template>
		</div>
	</BubbleMenu>
</template>
