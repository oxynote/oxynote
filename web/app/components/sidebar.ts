export type PlainActionFn = (() => void) | (() => Promise<void>)

// SidebarItemAction is one entry of a row's dropdown menu. A row with no
// actions renders no menu at all.
export interface SidebarItemAction {
	id: string
	name: string
	icon: string
	fn: PlainActionFn
}

export const SIDEBAR_ITEM_PLACEHOLDER_ID = "placeholder"
const TAG_DRAG_GROUP = "tags"
const TAG_DOCUMENT_DRAG_GROUP = "tag-documents"

function sidebarItemPlaceholder(name: string): SidebarItem {
	return {
		id: SIDEBAR_ITEM_PLACEHOLDER_ID,
		name: name,
		url: null,
		acceptsChildren: false,
		icon: null,
		active: false,
		draggable: false,
		actions: [],
		children: [],
	}
}

export interface SidebarItem {
	id: string
	name: string
	url?: string | null
	onClick?: (() => void) | (() => Promise<void>) // URL takes precedence over onClick
	// an item dropped onto this one becomes its child; an item that takes
	// no children can still be reordered against its siblings by edge drop
	acceptsChildren: boolean
	icon?: string | null
	dotColor?: string // a coloured dot rendered in place of the icon
	count?: number | null
	active: boolean
	draggable: boolean
	// items may only be dropped onto or next to items sharing this group,
	// which keeps each section's rows out of every other section
	dragGroup?: string
	actions: SidebarItemAction[]
	prefetchUrlOnInteraction?: boolean
	localOptimisticInsert?: boolean // used to indicate an item that is being optimistically inserted on the client
	shortcutTooltip?: {
		keyboardKey: { macOS: string; other: string }
		i18nKey: string | null
	}
	children: SidebarItem[] | null // all normal pages need to have at least an empty array
}

export interface SidebarItemCreate {
	parentId: string | null
}

export interface SidebarItemLocationUpdate {
	id: string
	parentId: string | null
	insertBeforeId: string | null
}

export interface DocumentBreadcrumb {
	id: string
	name: string
	href: string
	icon: string
}

export function processDocumentTree(
	data: DocumentTreeResponse,
	activeDocId: string | null,
	placeholderName: string,
	orgName: string,
	actions: (documentId: string) => SidebarItemAction[],
): SidebarItem[] {
	const res: SidebarItem[] = []

	data.forEach((v) => {
		res.push({
			id: v.id,
			name: v.documentName,
			icon: v.icon,
			acceptsChildren: true,
			active: v.id === activeDocId,
			draggable: true,
			url: !v.localOptimisticInsert
				? `/${createNameSlug(orgName)}/${createNameSlugWithId(v.documentName, v.id)}`
				: "#",
			prefetchUrlOnInteraction: true,
			localOptimisticInsert: v.localOptimisticInsert,
			actions: actions(v.id),
			children: v.children
				? processDocumentTree(
						v.children,
						activeDocId,
						placeholderName,
						orgName,
						actions,
					)
				: [sidebarItemPlaceholder(placeholderName)],
		})
	})

	if (res.length === 0) {
		res.push(sidebarItemPlaceholder(placeholderName))
	}

	return res
}

export function processTagTree(
	data: TagTreeResponse,
	activeDocId: string | null,
	orgName: string,
	tagActions: (tag: TagTreeElement) => SidebarItemAction[],
	documentActions: (
		documentId: string,
		defaultBranchId: string,
		tagId: string,
	) => SidebarItemAction[],
): SidebarItem[] {
	return data
		.filter((v) => !v.hidden)
		.map((v) => ({
			id: v.id,
			name: v.tagName,
			dotColor: v.color,
			acceptsChildren: false,
			active: false,
			draggable: true,
			dragGroup: TAG_DRAG_GROUP,
			actions: tagActions(v),
			children: v.documents?.length
				? processTagDocuments(
						v.documents,
						activeDocId,
						orgName,
						v.id,
						documentActions,
					)
				: null,
		}))
}

// documents listed under a tag stay put: they are a view of the tag's
// membership, not the tree that decides where a document lives.
//
// Only the documents at the top of that list carry the tag, on their default
// branch. The ones below them are a document's own subtree, which the tag
// says nothing about, so the menu that detaches a document from the tag
// stops at the first level — and skips a row still waiting for the server
// to answer, which has no branch to detach yet.
function processTagDocuments(
	data: DocumentTreeElement[],
	activeDocId: string | null,
	orgName: string,
	tagId: string,
	documentActions: (
		documentId: string,
		defaultBranchId: string,
		tagId: string,
	) => SidebarItemAction[],
	carriesTag = true,
): SidebarItem[] {
	return data.map((v) => ({
		id: v.id,
		name: v.documentName,
		icon: v.icon,
		acceptsChildren: true,
		active: v.id === activeDocId,
		draggable: false,
		dragGroup: TAG_DOCUMENT_DRAG_GROUP,
		url: `/${createNameSlug(orgName)}/${createNameSlugWithId(v.documentName, v.id)}`,
		prefetchUrlOnInteraction: true,
		actions:
			carriesTag && v.defaultBranchId
				? documentActions(v.id, v.defaultBranchId, tagId)
				: [],
		children: v.children?.length
			? processTagDocuments(
					v.children,
					activeDocId,
					orgName,
					tagId,
					documentActions,
					false,
				)
			: null,
	}))
}

export function extractDocumentTreeElement(
	tree: DocumentTreeElement[],
	targetID: string,
): DocumentTreeElement | null {
	for (const elem of tree) {
		if (elem.id === targetID) {
			return elem
		}

		if (elem.children) {
			const found = extractDocumentTreeElement(elem.children, targetID)
			if (found) {
				return found
			}
		}
	}

	return null
}

// documentTreeBreadcrumbs returns an array of tree elements that
// represent a path to the target document. The last element (length-1)
// in the array is the target document.
export function documentTreeBreadcrumbs(
	tree: DocumentTreeElement[],
	targetId: string,
): DocumentBreadcrumb[] {
	for (const elem of tree) {
		if (elem.id === targetId) {
			return [
				{
					id: elem.id,
					name: elem.documentName,
					icon: elem.icon,
					href: `/${createNameSlugWithId(elem.documentName, elem.id)}`,
				},
			]
		}

		if (elem.children) {
			const subBreadcrumbs = documentTreeBreadcrumbs(elem.children, targetId)
			if (subBreadcrumbs.length) {
				subBreadcrumbs.unshift({
					id: elem.id,
					name: elem.documentName,
					icon: elem.icon,
					href: `/${createNameSlugWithId(elem.documentName, elem.id)}`,
				})

				return subBreadcrumbs
			}
		}
	}

	return []
}
