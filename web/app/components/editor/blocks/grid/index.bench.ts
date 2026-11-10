import { getSchema, type JSONContent } from "@tiptap/core"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import type { Node as PMNode } from "@tiptap/pm/model"
import { EditorState, type Plugin } from "@tiptap/pm/state"
import { EditorView } from "@tiptap/pm/view"
import {
	prosemirrorToYDoc,
	ySyncPlugin,
	ySyncPluginKey,
} from "@tiptap/y-tiptap"
import { describe, type TestContext, test } from "vitest"
import * as Y from "yjs"
import { ContentDocument } from "../document"
import { BLOCK_GRID_NAME, METRIC_BLOCK_NAME } from "../node-names"
import { GridMetricBlockStub, nodeType } from "~/components/editor/test-helpers"
import { BlockGrid } from "."

// a grid block with a title that a collaborator can edit
const TitledBlock = GridMetricBlockStub.extend({
	addAttributes() {
		return { ...this.parent?.(), title: { default: "" } }
	},
	renderHTML() {
		return ["div"]
	},
})

const schema = getSchema([
	ContentDocument,
	Paragraph,
	Text,
	BlockGrid,
	TitledBlock,
])

const SENTENCE =
	"The chart runs its query when the page opens, and the text next to it explains the numbers."

// each benchmark compares an editor with the grid plugins to one without
// them. A collaborator's edit reaches the editor through yjs, as in the
// app. So y-tiptap hands it over as a document of all new nodes.
describe("BlockGrid", () => {
	for (const paragraphs of [300, 3000]) {
		describe(`in a document of ${paragraphs} paragraphs`, () => {
			test("a local keystroke in a paragraph", async ({
				bench,
				onTestFinished,
			}) => {
				const [plain, grid] = await editorPair(paragraphs, onTestFinished)

				await bench.compare(
					bench("without grid plugins", plain.localKeystroke),
					bench("with grid plugins", grid.localKeystroke),
				)
			})

			test("a collaborator's keystroke in a paragraph", async ({
				bench,
				onTestFinished,
			}) => {
				const [plain, grid] = await editorPair(paragraphs, onTestFinished)

				await bench.compare(
					bench("without grid plugins", plain.remoteKeystroke),
					bench("with grid plugins", grid.remoteKeystroke),
				)
			})

			test("a collaborator's edit of a grid block's title", async ({
				bench,
				onTestFinished,
			}) => {
				const [plain, grid] = await editorPair(paragraphs, onTestFinished)

				await bench.compare(
					bench("without grid plugins", plain.remoteTitle),
					bench("with grid plugins", grid.remoteTitle),
				)
			})

			test("the grid plugins alone, for a collaborator's keystroke", async ({
				bench,
			}) => {
				const doc = gridDocument(paragraphs)
				const state = EditorState.create({ doc: doc, plugins: gridPlugins() })
				const json = doc.toJSON() as JSONContent
				const text =
					json.content?.[Math.floor(doc.childCount / 2)]?.content?.[0]

				if (text) {
					text.text = `x${text.text ?? ""}`
				}

				const tr = state.tr
					.replaceWith(0, doc.content.size, schema.nodeFromJSON(json).content)
					.setMeta(ySyncPluginKey, { isChangeOrigin: true })

				await bench("grid plugins", () => {
					state.apply(tr)
				}).run()
			})
		})
	}
})

// two editors of the same document, the first without the grid plugins
function editorPair(
	paragraphs: number,
	onTestFinished: TestContext["onTestFinished"],
) {
	return Promise.all([
		collaborativeEditor(paragraphs, [], onTestFinished),
		collaborativeEditor(paragraphs, gridPlugins(), onTestFinished),
	])
}

// an editor whose document a collaborator also edits. Each action makes
// one edit: in the editor itself, or in the collaborator's copy.
async function collaborativeEditor(
	paragraphs: number,
	plugins: Plugin[],
	onTestFinished: TestContext["onTestFinished"],
) {
	const local = prosemirrorToYDoc(gridDocument(paragraphs), "content")
	const remote = new Y.Doc()
	const place = document.createElement("div")
	const rendered = Promise.withResolvers<undefined>()

	Y.applyUpdate(remote, Y.encodeStateAsUpdate(local))
	remote.on("update", (update: Uint8Array) => {
		Y.applyUpdate(local, update)
	})
	document.body.append(place)

	const view = new EditorView(place, {
		state: EditorState.create({
			schema: schema,
			plugins: [
				ySyncPlugin(local.getXmlFragment("content"), {
					onFirstRender: () => {
						rendered.resolve(undefined)
					},
				}),
				...plugins,
			],
		}),
	})

	onTestFinished(() => {
		view.destroy()
		place.remove()
	})

	await rendered.promise

	const root = remote.getXmlFragment("content")
	const middle = Math.floor(root.length / 2)
	const text = (root.get(middle) as Y.XmlElement).get(0) as Y.XmlText
	const grid = root
		.toArray()
		.slice(middle)
		.find((child) => (child as Y.XmlElement).nodeName === BLOCK_GRID_NAME)
	const block = (grid as Y.XmlElement).get(0) as Y.XmlElement
	let at = 0
	let titles = 0

	view.state.doc.forEach((_node, pos, i) => {
		if (i === middle) {
			at = pos + 5
		}
	})

	return {
		localKeystroke: () => {
			view.dispatch(view.state.tr.insertText("x", at))
		},
		remoteKeystroke: () => {
			text.insert(4, "x")
		},
		remoteTitle: () => {
			titles++
			block.setAttribute("title", `Requests ${titles}`)
		},
	}
}

// a document of paragraphs with a grid of 12 blocks after every 50th one
function gridDocument(paragraphs: number): PMNode {
	const children: PMNode[] = []

	for (let i = 1; i <= paragraphs; i++) {
		children.push(
			nodeType(schema, "paragraph").create(
				null,
				schema.text(`${i}. ${SENTENCE}`),
			),
		)

		if (i % 50 === 0) {
			children.push(
				nodeType(schema, BLOCK_GRID_NAME).create(
					null,
					Array.from({ length: 12 }, (_, b) =>
						nodeType(schema, METRIC_BLOCK_NAME).create({
							widePosX: (b % 2) * 12,
							widePosY: Math.floor(b / 2),
							stackPos: b,
						}),
					),
				),
			)
		}
	}

	return nodeType(schema, "doc").create(null, children)
}

function gridPlugins(): Plugin[] {
	return (
		BlockGrid.config.addProseMirrorPlugins?.call({
			editor: { isEditable: true },
		} as never) ?? []
	)
}
