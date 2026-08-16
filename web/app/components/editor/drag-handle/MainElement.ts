import type { ComputePositionConfig } from "@floating-ui/dom"
import type { Editor } from "@tiptap/vue-3"
import type { HocuspocusProvider } from "@hocuspocus/provider"
import type { Node } from "@tiptap/pm/model"
import type { PropType } from "vue"
import { defineComponent, h, onBeforeUnmount, onMounted, ref, watch } from "vue"
import { PluginKey } from "@tiptap/pm/state"
import { DragHandlePlugin, type DragHandlePluginProps } from "./handle-plugin"

// NOTE: this plugin/component is based on:
// https://github.com/ueberdosis/tiptap/tree/develop/packages/extension-drag-handle/src
// docs: https://tiptap.dev/docs/editor/extensions/functionality/drag-handle

type Optional<T, K extends keyof T> = Pick<Partial<T>, K> & Omit<T, K>

export const defaultComputePositionConfig: ComputePositionConfig = {
	placement: "left-start",
	strategy: "absolute",
}

export type DragHandleProps = Omit<
	Optional<DragHandlePluginProps, "pluginKey">,
	"element"
> & {
	class?: string
	onNodeChange?: (data: {
		node: Node | null
		editor: Editor
		pos: number
	}) => void
	locked?: boolean
}

export const DragHandle = defineComponent({
	name: "DragHandleVue",

	props: {
		pluginKey: {
			type: [String, Object] as PropType<DragHandleProps["pluginKey"]>,
			default: new PluginKey("hello"),
		},

		editor: {
			type: Object as PropType<DragHandleProps["editor"]>,
			required: true,
		},

		provider: {
			type: Object as PropType<HocuspocusProvider | null | undefined>,
			default: null,
		},

		onNodeChange: {
			type: Function as PropType<DragHandleProps["onNodeChange"]>,
			default: null,
		},

		onDragCancel: {
			type: Function as PropType<() => void>,
			default: null,
		},

		class: {
			type: String as PropType<DragHandleProps["class"]>,
			default: "drag-handle",
		},

		locked: {
			type: Boolean as PropType<DragHandleProps["locked"]>,
			default: false,
		},
	},

	setup(props, { slots }) {
		const root = ref<HTMLElement | null>(null)

		onMounted(() => {
			const { editor, pluginKey, onNodeChange, onDragCancel, provider } = props

			editor.registerPlugin(
				DragHandlePlugin({
					editor,
					// eslint-disable-next-line @typescript-eslint/no-non-null-assertion -- the render function always attaches this ref, so it is set by onMounted
					element: root.value!,
					pluginKey,
					provider,
					onNodeChange,
					onDragCancel,
					locked: props.locked,
				}).plugin,
			)
		})

		onBeforeUnmount(() => {
			const { pluginKey, editor } = props

			editor.unregisterPlugin(pluginKey as string)
		})

		watch(
			() => props.locked,
			(v) => {
				props.editor.commands.setMeta("lockDragHandle", v)
			},
		)

		return () => {
			return h(
				"div",
				{
					ref: root,
					class: props.class,
				},
				slots.default?.(),
			)
		}
	},
})
