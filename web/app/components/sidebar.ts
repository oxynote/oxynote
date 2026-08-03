export type PlainActionFn = (() => void) | (() => Promise<void>)

export const SIDEBAR_ITEM_PLACEHOLDER_ID = "placeholder"

export function sidebarItemPlaceholder(name: string): SidebarItem {
	return {
		id: SIDEBAR_ITEM_PLACEHOLDER_ID,
		name: name,
		url: null,
		partOfDocumentTree: true,
		icon: null,
		active: false,
		draggable: false,
		children: [],
	}
}

export interface SidebarItem {
	id: string | typeof SIDEBAR_ITEM_PLACEHOLDER_ID
	name: string
	url?: string | null
	onClick?: (() => void) | (() => Promise<void>) // URL takes precedence over onClick
	partOfDocumentTree: boolean
	icon?: string | null
	count?: number | null
	active: boolean
	draggable: boolean
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

export interface SidebarItemDelete {
	id: string
}

export interface SidebarItemDuplicate {
	id: string
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
): SidebarItem[] {
	const res: SidebarItem[] = []

	data.forEach((v) => {
		res.push({
			id: v.id,
			name: v.documentName,
			icon: v.icon,
			partOfDocumentTree: true,
			active: v.id === activeDocId,
			draggable: true,
			url: !v.localOptimisticInsert
				? `/${createNameSlug(orgName)}/${createNameSlugWithId(v.documentName, v.id)}`
				: "#",
			prefetchUrlOnInteraction: true,
			localOptimisticInsert: v.localOptimisticInsert,
			children: v.children
				? processDocumentTree(v.children, activeDocId, placeholderName, orgName)
				: [sidebarItemPlaceholder(placeholderName)],
		})
	})

	if (res.length === 0) {
		res.push(sidebarItemPlaceholder(placeholderName))
	}

	return res
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
