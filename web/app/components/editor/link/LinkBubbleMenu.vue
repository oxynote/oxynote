<script lang="ts" setup>
import type { Editor } from "@tiptap/core"
import { Plugin, PluginKey, TextSelection } from "@tiptap/pm/state"
import type { EditorView } from "@tiptap/pm/view"
import { getMarkRange } from "@tiptap/core"
import Link from "@tiptap/extension-link"
import { computePosition, flip, offset, shift } from "@floating-ui/dom"
import { COMMENT_MARK_NAME } from "../mark-names"

const props = defineProps<{
	editor: Editor
	container: HTMLElement | null
	readOnly?: boolean
}>()

const { isEditable } = useEditorMeta()
const { openExternalLink } = useExternalLinks()
const isEditing = ref(false)
const editLinkText = ref("")
const editLinkUrl = ref("")
const renderPopover = ref(false)
const popoverPosition = ref({ top: 0, left: 0 })
const currentLink = ref<{
	href: string
	text: string
	from: number
	to: number
	// set when the diff view marks the target as changed
	oldHref?: string
} | null>(null)

let hoverTimeout: ReturnType<typeof setTimeout> | null = null
let closeTimeout: ReturnType<typeof setTimeout> | null = null
const isHoveringPopover = ref(false)
const popoverElem = useTemplateRef<HTMLDivElement>("link-bubble-popover")
const referenceElem = ref<Element | null>(null)
let isSyncingFromEditor = false

// a plain click opens a link. ProseMirror skips drags, so link text stays
// selectable.
const openOnClickKey = new PluginKey("openLinkOnClick")
const openOnClickPlugin = new Plugin({
	key: openOnClickKey,
	props: {
		handleClick: (view, _pos, event) => openClickedLink(view, event),
	},
})

function syncEditedLink() {
	if (!isEditing.value || !currentLink.value) {
		return
	}

	const { state } = props.editor
	const linkType = state.schema.marks.link
	if (!linkType) {
		return
	}

	// Check if the position is still valid
	if (currentLink.value.from >= state.doc.content.size) {
		// Position is out of bounds, cancel editing
		cancelEdit()
		return
	}

	// Resolve the position and get the current mark range
	const $pos = state.doc.resolve(currentLink.value.from + 1)
	const range = getMarkRange($pos, linkType)

	if (!range) {
		// Link was removed, cancel editing
		cancelEdit()
		return
	}

	// a link is not inclusive, so the marks at its end leave it out. A link
	// one character long ends at from + 1, so read its first character.
	const linkMark = state.doc
		.nodeAt(range.from)
		?.marks.find((mark) => mark.type === linkType)
	if (!linkMark) {
		// Link mark not found, cancel editing
		cancelEdit()
		return
	}

	const currentHref = linkMark.attrs.href as string
	const currentText = state.doc.textBetween(range.from, range.to, "")

	// Update refs if they differ from current state
	if (
		currentLink.value.href !== currentHref ||
		currentLink.value.text !== currentText ||
		currentLink.value.from !== range.from ||
		currentLink.value.to !== range.to
	) {
		isSyncingFromEditor = true
		currentLink.value = {
			href: currentHref,
			text: currentText,
			from: range.from,
			to: range.to,
		}
		editLinkText.value = currentText
		editLinkUrl.value = currentHref
		void nextTick(() => {
			isSyncingFromEditor = false
		})
	}
}

function updatePopoverPosition() {
	const popover = popoverElem.value
	const reference = referenceElem.value

	if (!popover || !reference || !renderPopover.value) {
		return
	}

	const commentMarkType =
		props.editor.state.schema.marks[COMMENT_MARK_NAME] ?? null

	const preferTop =
		currentLink.value &&
		commentMarkType &&
		!isEditing.value &&
		props.editor.state.doc.rangeHasMark(
			currentLink.value.from,
			currentLink.value.to,
			commentMarkType,
		)

	void computePosition(reference, popover, {
		placement: preferTop ? "top-start" : "bottom-start",
		strategy: "absolute",
		middleware: [
			offset({ mainAxis: 5 }),
			flip({
				boundary: props.container ?? undefined,
				fallbackPlacements: preferTop
					? ["top", "top-end"]
					: ["bottom", "bottom-end"],
				padding: 8,
			}),
			shift({
				boundary: props.container ?? undefined,
				padding: 8,
			}),
		],
	}).then(({ x, y }) => {
		popoverPosition.value = { left: x, top: y }
	})
}

function handleMouseEnter(event: MouseEvent) {
	const target = event.target as HTMLElement
	if (target.tagName !== "A") {
		return
	}

	// Clear any existing timeout
	if (hoverTimeout) {
		clearTimeout(hoverTimeout)
	}

	// leaving a touching link scheduled a close
	if (closeTimeout) {
		clearTimeout(closeTimeout)
		closeTimeout = null
	}

	// Show popover after a short delay
	hoverTimeout = setTimeout(() => {
		const { state, view } = props.editor

		const linkMarkType = state.schema.marks.link
		if (!linkMarkType) {
			return
		}

		let pos: number

		try {
			pos = view.posAtDOM(target, 0)
		} catch {
			return
		}

		const $pos = state.doc.resolve(pos)

		const range = getMarkRange($pos, linkMarkType)
		if (!range) {
			return
		}

		const rangeNode = state.doc.nodeAt(range.from)
		const linkMark =
			rangeNode?.marks.find((mark) => mark.type === linkMarkType) ??
			$pos.marks().find((mark) => mark.type === linkMarkType)

		if (!linkMark) {
			return
		}

		const href = linkMark.attrs.href as string
		const text = state.doc.textBetween(range.from, range.to, "")

		currentLink.value = {
			href,
			text,
			from: range.from,
			to: range.to,
			oldHref:
				target.querySelector<HTMLElement>("[data-old-href]")?.dataset.oldHref,
		}

		// Store reference element and show popover
		referenceElem.value = target
		renderPopover.value = true

		// Position the popover using floating-ui
		void nextTick(updatePopoverPosition)
	}, 200)
}

function handleMouseLeave(event: MouseEvent) {
	const target = event.target as HTMLElement
	if (target.tagName !== "A") {
		return
	}

	if (hoverTimeout) {
		clearTimeout(hoverTimeout)
		hoverTimeout = null
	}

	// Delay hiding to allow moving to popover
	closeTimeout = setTimeout(() => {
		if (!isEditing.value && !isHoveringPopover.value) {
			renderPopover.value = false
			currentLink.value = null
			referenceElem.value = null
		}
	}, 500)
}

function handlePopoverMouseEnter() {
	isHoveringPopover.value = true

	// Keep popover open when hovering over it
	if (hoverTimeout) {
		clearTimeout(hoverTimeout)
		hoverTimeout = null
	}

	if (closeTimeout) {
		clearTimeout(closeTimeout)
		closeTimeout = null
	}
}

function handlePopoverMouseLeave() {
	isHoveringPopover.value = false

	if (!isEditing.value) {
		renderPopover.value = false
		currentLink.value = null
		referenceElem.value = null
	}
}

// focus moving between the popover's own controls is not a leave
function handlePopoverFocusOut(e: FocusEvent) {
	const popover = e.currentTarget as HTMLElement
	if (e.relatedTarget instanceof Node && popover.contains(e.relatedTarget)) {
		return
	}

	handlePopoverMouseLeave()
}

function containsTarget(target: Node) {
	return !!popoverElem.value?.contains(target)
}

function handleClickOutside(event?: MouseEvent) {
	if (!isEditing.value || !popoverElem.value) {
		return
	}

	const target = event?.target as Node | null | undefined
	if (!target || !popoverElem.value.contains(target)) {
		closeEdit()
	}
}

onMounted(() => {
	const editorElement = props.editor.view.dom
	editorElement.addEventListener("mouseenter", handleMouseEnter, true)
	editorElement.addEventListener("mouseleave", handleMouseLeave, true)

	// Listen to editor updates to sync edit refs with current state
	props.editor.on("update", syncEditedLink)
	props.editor.registerPlugin(openOnClickPlugin)
})

onBeforeUnmount(() => {
	props.editor.off("update", syncEditedLink)
	props.editor.unregisterPlugin(openOnClickKey)
	document.removeEventListener("mousedown", handleClickOutside)
	if (hoverTimeout) {
		clearTimeout(hoverTimeout)
	}

	if (closeTimeout) {
		clearTimeout(closeTimeout)
	}
})

function editLink() {
	if (!currentLink.value) {
		return
	}

	isEditing.value = true
	updatePopoverPosition()

	editLinkText.value = currentLink.value.text
	editLinkUrl.value = currentLink.value.href
	document.addEventListener("mousedown", handleClickOutside)
}

function cancelEdit() {
	isEditing.value = false
	editLinkText.value = ""
	editLinkUrl.value = ""
	renderPopover.value = false
	currentLink.value = null
	isHoveringPopover.value = false
	referenceElem.value = null
	document.removeEventListener("mousedown", handleClickOutside)
}

// closing leaves the fields, so it does what leaving them does. An empty
// field removes the link. A click outside has already moved the focus, so
// nothing here takes it back.
function closeEdit() {
	if (!isEditing.value || !currentLink.value) {
		cancelEdit()
		return
	}

	const { from, to } = currentLink.value

	if (editLinkText.value === "" || editLinkUrl.value === "") {
		props.editor.commands.command(({ tr, state }) => {
			const linkType = state.schema.marks.link
			if (!linkType) {
				return false
			}

			tr.removeMark(from, to, linkType)
			return true
		})
	} else {
		void updateLink(editLinkText.value, withDefaultScheme(editLinkUrl.value))
	}

	cancelEdit()
}

// the scheme is added once the address is left, never while it is typed
function addMissingScheme() {
	editLinkUrl.value = withDefaultScheme(editLinkUrl.value)
}

function removeLink() {
	if (!currentLink.value) {
		return
	}

	const { from, to } = currentLink.value

	props.editor.chain().focus().setTextSelection({ from, to }).unsetLink().run()

	cancelEdit()
}

// shift-click extends the selection. A read-only editor follows links
// itself, and a file block's anchor has its own handler.
function openClickedLink(view: EditorView, event: MouseEvent): boolean {
	const anchor = (event.target as Element).closest("a")
	const href = anchor?.getAttribute("href")
	const linkType = view.state.schema.marks.link
	if (
		event.button !== 0 ||
		event.shiftKey ||
		!view.editable ||
		!anchor ||
		!href ||
		!linkType
	) {
		return false
	}

	const $pos = view.state.doc.resolve(view.posAtDOM(anchor, 0))
	if (!getMarkRange($pos, linkType)) {
		return false
	}

	openExternalLink(href)
	return true
}

function openLink() {
	if (!currentLink.value) {
		return
	}

	openExternalLink(currentLink.value.href)
}

async function updateLink(newText: string, newHref: string) {
	if (
		!currentLink.value ||
		(newText === currentLink.value.text && newHref === currentLink.value.href)
	) {
		return
	}

	// the document keeps the old link while a field is empty. Filling it in
	// again or closing the popover settles it.
	if (newText === "" || newHref === "") {
		return
	}

	const from = currentLink.value.from
	const newTo = from + newText.length
	const oldText = currentLink.value.text

	props.editor.commands.command(({ tr, state }) => {
		const linkType = state.schema.marks.link
		if (!linkType) {
			return false
		}

		if (newText !== oldText) {
			let start = 0
			let oldEnd = oldText.length
			let newEnd = newText.length

			while (
				start < oldEnd &&
				start < newEnd &&
				oldText[start] === newText[start]
			) {
				start += 1
			}

			while (
				oldEnd > start &&
				newEnd > start &&
				oldText[oldEnd - 1] === newText[newEnd - 1]
			) {
				oldEnd -= 1
				newEnd -= 1
			}

			const replaceFrom = from + start
			const replaceTo = from + oldEnd
			const insertedText = newText.slice(start, newEnd)

			tr.insertText(insertedText, replaceFrom, replaceTo)
		}

		tr.removeMark(from, newTo, linkType)
		tr.addMark(from, newTo, linkType.create({ href: newHref }))
		tr.setSelection(TextSelection.create(tr.doc, from, newTo))

		return true
	})

	await nextTick()

	// the popover closed while the update ran
	if (!isEditing.value) {
		return
	}

	// Recompute the new link range from the updated doc
	const { state } = props.editor

	const linkType = state.schema.marks.link
	if (!linkType) {
		return
	}

	const $pos = state.doc.resolve(from + 1)
	const range = getMarkRange($pos, linkType)

	if (range) {
		currentLink.value = {
			href: newHref,
			text: newText,
			from: range.from,
			to: range.to,
		}

		return
	}

	// Fall back to the selection if getMarkRange fails (e.g., user typed weird markup)
	currentLink.value = {
		href: newHref,
		text: newText,
		from,
		to: newTo,
	}
}

watch(editLinkUrl, (val) => {
	if (!isEditing.value || isSyncingFromEditor) {
		return
	}

	void updateLink(editLinkText.value, val)
})

watch(editLinkText, (val) => {
	if (!isEditing.value || isSyncingFromEditor) {
		return
	}

	void updateLink(val, editLinkUrl.value)
})

function editSelection() {
	// Get the current selection position
	const { state } = props.editor
	const { from } = state.selection

	// Find the link mark at the current position
	const linkType = state.schema.marks.link
	if (!linkType) {
		return
	}

	// Resolve the position and get the mark range
	const $pos = state.doc.resolve(from)
	const range = getMarkRange($pos, linkType)

	if (!range) {
		return
	}

	// Get the link mark to extract the href
	const linkMark = $pos.marks().find((mark) => mark.type.name === Link.name)
	if (!linkMark) {
		return
	}

	const href = linkMark.attrs.href as string
	const text = state.doc.textBetween(range.from, range.to, "")

	// Set current link data
	currentLink.value = {
		href,
		text,
		from: range.from,
		to: range.to,
	}

	// Create virtual reference element from editor coordinates
	const { view } = props.editor
	const start = view.coordsAtPos(range.from)
	const end = view.coordsAtPos(range.to)

	referenceElem.value = {
		getBoundingClientRect: () => ({
			width: end.left - start.left,
			height: start.bottom - start.top,
			x: start.left,
			y: start.top,
			top: start.top,
			left: start.left,
			bottom: start.bottom,
			right: end.left,
		}),
	} as Element

	// Show the popover and enter edit mode
	renderPopover.value = true
	isEditing.value = true
	editLinkText.value = text
	editLinkUrl.value = href
	document.addEventListener("mousedown", handleClickOutside)

	// Position the popover using floating-ui
	void nextTick(updatePopoverPosition)
}

onKeyStroke("Escape", () => {
	closeEdit()
})

defineExpose({
	editSelection,
	containsTarget,
})
</script>
<template>
	<Teleport to="body">
		<Transition
			enter-from-class="fade-in-0"
			enter-active-class="animate-in"
			enter-to-class="fade-in-100"
			leave-from-class="fade-in-100"
			leave-active-class="animate-out"
			leave-to-class="fade-out-0"
		>
			<!-- eslint-disable-next-line vuejs-accessibility/no-static-element-interactions -- the handlers only track whether the pointer or focus is inside the popover so it stays open; the popover is a container, not a control -->
			<div
				v-if="renderPopover && currentLink"
				ref="link-bubble-popover"
				:style="{
					top: `${popoverPosition.top}px`,
					left: `${popoverPosition.left}px`,
				}"
				class="absolute z-popover max-w-[20rem] min-w-40 gap-2 rounded-lg border border-border bg-background py-1 shadow-md"
				@mouseenter="handlePopoverMouseEnter"
				@mouseleave="handlePopoverMouseLeave"
				@focusin="handlePopoverMouseEnter"
				@focusout="handlePopoverFocusOut"
			>
				<div v-if="!isEditing" class="flex h-full min-w-0 flex-col gap-1.5">
					<div class="flex h-full min-w-0 items-center gap-1 pr-1 pl-1.75">
						<div class="flex min-w-0 flex-1 items-center gap-1.25 pr-0.5">
							<Icon
								name="mingcute:earth-2-line"
								class="size-4 text-muted-foreground"
							/>
							<div
								v-if="currentLink.oldHref"
								class="flex min-w-0 flex-1 flex-col items-start gap-0.5 text-2sm"
							>
								<span class="sr-only">{{ $t("editor.link.previous") }}</span>
								<span
									class="max-w-full truncate rounded-sm bg-diff-text-removed px-1 line-through"
								>
									{{ currentLink.oldHref }}
								</span>
								<span
									class="max-w-full truncate rounded-sm bg-diff-text-added px-1"
								>
									{{ currentLink.href }}
								</span>
							</div>
							<span
								v-else
								class="flex-1 truncate text-2sm text-muted-foreground"
							>
								{{ currentLink.href }}
							</span>
						</div>
						<div class="w-0.25 shrink-0 self-stretch bg-border" />
						<div class="flex items-center gap-0.25">
							<ShadcnUiButton
								size="icon-sm"
								variant="ghost"
								:title="$t('editor.link.open')"
								@click="openLink"
							>
								<Icon name="lucide:external-link" />
							</ShadcnUiButton>
							<ShadcnUiButton
								v-if="isEditable && !props.readOnly"
								size="icon-sm"
								variant="ghost"
								:title="$t('editor.link.edit')"
								@click="editLink"
							>
								<Icon name="lucide:pencil-line" />
							</ShadcnUiButton>
						</div>
					</div>
				</div>
				<div v-else class="flex flex-col gap-1.5">
					<div class="flex flex-col gap-1.5 px-1.75">
						<div class="flex flex-col gap-1">
							<ShadcnUiSelectLabel>
								<span class="text-2sm">
									{{ $t("editor.link.page-or-url") }}
								</span>
							</ShadcnUiSelectLabel>
							<ShadcnUiInput
								v-model="editLinkUrl"
								:placeholder="$t('editor.link.page-or-url-placeholder')"
								class="h-[1.775rem] px-2 text-2sm md:text-2sm"
								disable-focus-effect
								type="url"
								@blur="addMissingScheme"
								@keydown.enter.prevent="closeEdit"
							/>
						</div>
						<div class="flex flex-col gap-1">
							<ShadcnUiSelectLabel>
								<span class="text-2sm">
									{{ $t("editor.link.title") }}
								</span>
							</ShadcnUiSelectLabel>
							<ShadcnUiInput
								v-model="editLinkText"
								:placeholder="$t('editor.link.title-placeholder')"
								class="h-[1.775rem] px-2 text-2sm md:text-2sm"
								disable-focus-effect
								type="text"
								@keydown.enter.prevent="closeEdit"
							/>
						</div>
					</div>
					<ShadcnUiSeparator />
					<div class="flex px-1.75">
						<ShadcnUiButton
							class="flex-1 gap-1"
							variant="secondary"
							size="2sm"
							@click="removeLink"
						>
							<Icon name="mingcute:delete-2-line" />
							{{ $t("editor.link.delete") }}
						</ShadcnUiButton>
					</div>
				</div>
			</div>
		</Transition>
	</Teleport>
</template>
