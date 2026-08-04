import { mergeAttributes, Node } from "@tiptap/core"
import Paragraph from "@tiptap/extension-paragraph"
import { CommentMark, AddedMark, DeletedMark } from "./marks.js"

export const ParameterListItemHeaderType = Node.create({
	name: "splitDocumentationParameterListItemHeaderType",
	group: "block",
	content: `text*`,
	marks: `${CommentMark.name} ${AddedMark.name} ${DeletedMark.name}`,
	defining: true,
	isolating: true,
	selectable: false,
	parseHTML() {
		return [
			{
				tag: `div[data-type="split-documentation-parameter-list-item-header-type"]`,
			},
		]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type":
					"split-documentation-parameter-list-item-header-type",
			}),
			0,
		]
	},
})

export const ParameterListItemHeaderTitle = Node.create({
	name: "splitDocumentationParameterListItemHeaderTitle",
	group: "block",
	content: `text*`,
	marks: `${CommentMark.name} ${AddedMark.name} ${DeletedMark.name}`,
	defining: true,
	isolating: true,
	selectable: false,
	parseHTML() {
		return [
			{
				tag: `div[data-type="split-documentation-parameter-list-item-header-title"]`,
			},
		]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type":
					"split-documentation-parameter-list-item-header-title",
			}),
			0,
		]
	},
})

export const ParameterListItemHeader = Node.create({
	name: "splitDocumentationParameterListItemHeader",
	group: "block",
	content: `${ParameterListItemHeaderTitle.name} ${ParameterListItemHeaderType.name}`,
	defining: true,
	isolating: true,
	selectable: false,
	parseHTML() {
		return [
			{
				tag: `div[data-type="split-documentation-parameter-list-item-header"]`,
			},
		]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type":
					"split-documentation-parameter-list-item-header",
			}),
			0,
		]
	},
})

export const ParameterListItem = Node.create({
	name: "splitDocumentationParameterListItem",
	group: "block",
	content: `${ParameterListItemHeader.name} ${Paragraph.name}`,
	defining: true,
	isolating: true,
	selectable: false,
	parseHTML() {
		return [
			{
				tag: `div[data-type="split-documentation-parameter-list-item"]`,
			},
		]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type":
					"split-documentation-parameter-list-item",
			}),
			0,
		]
	},
})

export const ParameterListHeader = Node.create({
	name: "splitDocumentationParameterListHeader",
	content: `text*`,
	group: "block",
	marks: `${CommentMark.name} ${AddedMark.name} ${DeletedMark.name}`,
	defining: true,
	isolating: true,
	selectable: false,
	parseHTML() {
		return [
			{
				tag: `div[data-type="split-documentation-parameter-list-header"]`,
			},
		]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type":
					"split-documentation-parameter-list-header",
			}),
			0,
		]
	},
})

export const ParameterList = Node.create({
	name: "splitDocumentationParameterList",
	group: "block",
	content: `${ParameterListHeader.name} ${ParameterListItem.name}+`,
	defining: true,
	isolating: true,
	selectable: false,
	parseHTML() {
		return [
			{
				tag: `div[data-type="split-documentation-parameter-list"]`,
			},
		]
	},
	renderHTML({ HTMLAttributes }) {
		return [
			"div",
			mergeAttributes(HTMLAttributes, {
				"data-type":
					"split-documentation-parameter-list",
			}),
			0,
		]
	},
})
