import type { Position } from "@vueuse/core"
import type { CSSProperties } from "vue"
import type { SidebarItem } from "~/components/sidebar"
import { cn } from "~/lib/utils"

export interface SidebarDraggableOptions {
	disabled?: MaybeRefOrGetter<boolean>
	minDistance?: number // minimum distance required to trigger a drag
}

const useSidebarDraggableStore = defineStore("sidebar-draggable", () => {
	const draggedElem = ref<{
		wrapper: HTMLElement
		item: SidebarItem
	} | null>(null)
	const dragTarget = ref<{
		trigger: "top-edge" | "item"
		triggerId: string
		parentId: string | null
		insertBeforeId: string | null // null indicates the very top
	} | null>(null)

	// we need this to ensure that we aren't highlighting the edge and
	// the item at the same time.
	// The value is the ID of the trigger.
	const edgeDraggedOn = ref<string | null>(null)

	function updateDraggedElem(
		v: {
			wrapper: HTMLElement
			item: SidebarItem
		} | null,
	) {
		draggedElem.value = v
	}

	function updateDragTarget(
		v: {
			trigger: "top-edge" | "item"
			triggerId: string
			parentId: string | null
			insertBeforeId: string | null
		} | null,
	) {
		dragTarget.value = v
	}

	function resetDragTarget(triggerId: string, trigger: "top-edge" | "item") {
		if (
			dragTarget.value?.triggerId === triggerId &&
			dragTarget.value.trigger === trigger
		) {
			dragTarget.value = null
		}
	}

	function updateEdgeDraggedOn(v: string | null) {
		edgeDraggedOn.value = v
	}

	function resetEdgeDraggedOn(triggerId: string) {
		if (edgeDraggedOn.value === triggerId) {
			edgeDraggedOn.value = null
		}
	}

	return {
		draggedElem,
		updateDraggedElem,
		dragTarget,
		updateDragTarget,
		resetDragTarget,
		edgeDraggedOn,
		updateEdgeDraggedOn,
		resetEdgeDraggedOn,
	}
})

export function useSidebarDraggable(
	elem: MaybeRef<HTMLElement | null | undefined>,
	elemSidebarItem: MaybeRefOrGetter<SidebarItem & { parentId: string | null }>,
	topEdgeElem: MaybeRef<HTMLElement | null | undefined>,
	wrapperElem: MaybeRefOrGetter<HTMLElement | null | undefined>,
	onConfirm: (data: {
		targetParentId: string | null
		targetInsertBeforeId: string | null
	}) => void,
	opts: SidebarDraggableOptions,
) {
	const store = useSidebarDraggableStore()
	let dragFirstPosition: Position | null = null

	const { x, y } = useDraggable(elem, {
		disabled: opts.disabled,
		axis: "both",
		onStart: () => {
			document.documentElement.classList.add(
				cn("select-none!"),
				cn("[&_*]:select-none!"),
			)
		},
		onMove: (pos) => {
			if (!dragFirstPosition) {
				dragFirstPosition = pos
				return
			}

			if (store.draggedElem) {
				return
			}

			if (Math.abs(dragFirstPosition.y - pos.y) < opts.minDistance!) {
				return
			}

			store.updateDraggedElem({
				wrapper: toValue(wrapperElem)!,
				item: toValue(elemSidebarItem),
			})
			document.documentElement.classList.add(
				cn("cursor-grabbing!"),
				cn("[&_*]:cursor-grabbing!"),
			)
		},
		onEnd: () => {
			if (store.dragTarget) {
				onConfirm({
					targetParentId: store.dragTarget.parentId,
					targetInsertBeforeId: store.dragTarget.insertBeforeId,
				})
			}

			dragFirstPosition = null
			store.updateDraggedElem(null)
			store.updateDragTarget(null)
			store.updateEdgeDraggedOn(null)
			document.documentElement.classList.remove(
				cn("select-none!"),
				cn("[&_*]:select-none!"),
				cn("cursor-grabbing!"),
				cn("[&_*]:cursor-grabbing!"),
			)
		},
	})
	const elemBounds = useElementBounding(elem)
	const elemMouse = useMouseInElement(elem, {
		handleOutside: false,
	})
	const topEdgeElemMouse = useMouseInElement(topEdgeElem, {
		handleOutside: false,
	})
	const canDragOn = computed(() => {
		return (
			store.draggedElem !== null &&
			toValue(elemSidebarItem).id !== store.draggedElem.item.id &&
			!isValidDescendent(store.draggedElem.wrapper, toValue(elem))
		)
	})
	const isItemDraggedOn = computed(() => {
		return (
			store.draggedElem !== null &&
			!elemMouse.isOutside.value &&
			store.edgeDraggedOn === null &&
			canDragOn.value
		)
	})
	const isTopEdgeDraggedOn = computed(() => {
		return (
			store.draggedElem !== null &&
			!topEdgeElemMouse.isOutside.value &&
			store.edgeDraggedOn !== null &&
			canDragOn.value
		)
	})

	// we need to have an additional ref/check for edges, because
	// edges and normal items can be dragged on at the same time.
	watch(
		() => !topEdgeElemMouse.isOutside.value,
		(newV, oldV) => {
			const item = toValue(elemSidebarItem)

			if (newV && !oldV) {
				store.updateEdgeDraggedOn(item.id)
			} else if (!newV && oldV) {
				store.resetEdgeDraggedOn(item.id)
			}
		},
	)

	watch(
		() => isItemDraggedOn.value && !isTopEdgeDraggedOn.value,
		(newV, oldV) => {
			const item = toValue(elemSidebarItem)

			if (newV && !oldV) {
				store.updateDragTarget({
					trigger: "item",
					triggerId: item.id,
					parentId: item.id,
					insertBeforeId: null,
				})
			} else if (!newV && oldV) {
				store.resetDragTarget(item.id, "item")
			}
		},
	)

	watch(isTopEdgeDraggedOn, (newV, oldV) => {
		const item = toValue(elemSidebarItem)

		if (newV && !oldV) {
			store.updateDragTarget({
				trigger: "top-edge",
				triggerId: item.id,
				parentId: item.parentId,
				insertBeforeId: item.id,
			})
		} else if (!newV && oldV) {
			store.resetDragTarget(item.id, "top-edge")
		}
	})

	return {
		isDraggingGlobal: computed(() => {
			return store.draggedElem !== null
		}),
		isDraggingSelf: computed(() => {
			return store.draggedElem?.item.id === toValue(elemSidebarItem).id
		}),
		draggedGhostStyle: computed<CSSProperties>(() => {
			if (store.draggedElem?.item.id !== toValue(elemSidebarItem).id) {
				return {}
			}

			return {
				top: `${y.value}px`,
				left: `${x.value}px`,
				width: `${elemBounds.width.value}px`,
				height: `${elemBounds.height.value}px`,
			}
		}),
		isItemDraggedOn,
		isTopEdgeDraggedOn,
	}
}
