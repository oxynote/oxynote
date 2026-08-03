import { Mark } from "@tiptap/core"

export const AddedMark = Mark.create({
	name: "added",
	inclusive: false,
	priority: 100,
	excludes: "deleted",
	parseHTML() {
		return [{ tag: "span[data-added]" }]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"span",
			{
				...HTMLAttributes,
				"data-added": "true",
			},
			0,
		]
	},
})

export const DeletedMark = Mark.create({
	name: "deleted",
	inclusive: false,
	priority: 100,
	excludes: "added",
	parseHTML() {
		return [{ tag: "span[data-deleted]" }]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"span",
			{
				...HTMLAttributes,
				"data-deleted": "true",
			},
			0,
		]
	},
})

export interface CommentAttrs {
	commentId: string
}

export const CommentMark = Mark.create({
	name: "comment",
	inclusive: false,
	priority: 1000,
	excludes: "",
	addAttributes() {
		return {
			commentId: {
				default: null,
				parseHTML: (element) => element.getAttribute("data-comment-id"),
				renderHTML: (attrs) => {
					return {
						"data-comment-id": attrs.commentId,
					}
				},
			},
		}
	},
	parseHTML() {
		return [{ tag: "span[data-comment-id]" }]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"span",
			{
				...HTMLAttributes,
			},
			0,
		]
	},
})
