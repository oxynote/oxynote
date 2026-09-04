import { describe, it, vi, type Mock } from "vitest"
import * as Y from "yjs"
import {
	createDocumentHooks,
	createHocuspocus,
	parseDocumentName,
	resetConnections,
	resolveBranchId,
	type AuthSession,
} from "./hocuspocus.js"
import { replaceYdocContent, transformer } from "./ydocument.js"
import {
	fragmentXml,
	stubCore,
	stubLog,
	type StubCore,
} from "./test-helpers.js"

type ConnectedDocument = Y.Doc & { broadcastStateless: Mock }

const SESSION: AuthSession = { user: { id: "user-1" } }

function stubAuth(session: AuthSession | null = SESSION) {
	return { getSession: vi.fn().mockResolvedValue(session) }
}

// the hocuspocus Document adds the stateless broadcast the store hook
// reaches for when a persist fails
function connectedDocument(): ConnectedDocument {
	const doc = new Y.Doc() as ConnectedDocument
	doc.broadcastStateless = vi.fn()

	return doc
}

function seededDocument(name = "Runbook"): ConnectedDocument {
	const doc = connectedDocument()
	replaceYdocContent(doc, {
		name,
		content: {
			type: "doc",
			content: [
				{
					type: "paragraph",
					attrs: { uid: "p1" },
					content: [
						{
							type: "text",
							text: "Restart it",
						},
					],
				},
			],
		},
		icon: "lucide:file",
	})

	return doc
}

function hooksWith(core: StubCore, session: AuthSession | null = SESSION) {
	return createDocumentHooks({ auth: stubAuth(session), core })
}

describe("parseDocumentName", () => {
	it.for([
		{
			name: "a concrete branch id",
			input: "doc1-branch1",
			expected: {
				documentId: "doc1",
				branchIdentifier: "branch1",
			},
		},
		{
			name: "the default branch marker",
			input: "doc1-default",
			expected: {
				documentId: "doc1",
				branchIdentifier: "default",
			},
		},
		{
			name: "a branch identifier containing dashes",
			input: "doc1-feature-x-2",
			expected: {
				documentId: "doc1",
				branchIdentifier: "feature-x-2",
			},
		},
	])(
		"splits $name on the first dash",
		({ input, expected }, { expect }) => {
			expect(parseDocumentName(input)).toEqual(expected)
		},
	)
})

describe("resolveBranchId", () => {
	it("returns a concrete branch identifier without asking core", async ({
		expect,
	}) => {
		const core = stubCore()

		const branchId = await resolveBranchId(core, "doc1", "branch1")

		expect(branchId).toBe("branch1")
		expect(core.fetchBranches).toHaveBeenCalledTimes(0)
	})

	// the main branch is identified by its default flag, not by being
	// first in the list or by its name
	it("resolves 'default' to the branch core flags as default", async ({
		expect,
	}) => {
		const core = stubCore()
		core.fetchBranches.mockResolvedValue([
			{
				branchId: "branch-2",
				default: false,
				protected: false,
			},
			{
				branchId: "branch-1",
				default: true,
				protected: false,
			},
		])

		const branchId = await resolveBranchId(core, "doc1", "default")

		expect(branchId).toBe("branch-1")
		expect(core.fetchBranches).toHaveBeenCalledWith("doc1")
	})

	it("throws when no branch is flagged as default", async ({
		expect,
	}) => {
		const core = stubCore()
		core.fetchBranches.mockResolvedValue([
			{
				branchId: "branch-2",
				default: false,
				protected: false,
			},
		])

		await expect(
			resolveBranchId(core, "doc1", "default"),
		).rejects.toThrow("no default branch found for document doc1")
	})

	it("propagates a failed branch lookup", async ({ expect }) => {
		const failure = new Error("core unreachable")
		const core = stubCore()
		core.fetchBranches.mockRejectedValue(failure)

		await expect(
			resolveBranchId(core, "doc1", "default"),
		).rejects.toBe(failure)
	})
})

describe("createDocumentHooks", () => {
	describe("onAuthenticate", () => {
		it("returns the session for an authenticated connection", async ({
			expect,
		}) => {
			const core = stubCore()
			const auth = stubAuth()
			const hooks = createDocumentHooks({ auth, core })

			const result = await hooks.onAuthenticate({
				connectionConfig: { readOnly: false },
				documentName: "doc1-branch1",
				request: {},
				requestHeaders: { cookie: "auth.session=abc" },
				token: "",
			})

			expect(result).toEqual({ session: SESSION })
			expect(auth.getSession).toHaveBeenCalledTimes(1)
		})

		// keeping an edit out of the document is the only way to keep it
		// out of the whole-document state a later persist carries
		it("locks the connection to a protected branch", async ({
			expect,
		}) => {
			const core = stubCore()
			core.fetchBranches.mockResolvedValue([
				{
					branchId: "branch1",
					default: false,
					protected: true,
				},
			])
			const hooks = hooksWith(core)
			const connectionConfig = { readOnly: false }

			await hooks.onAuthenticate({
				connectionConfig,
				documentName: "doc1-branch1",
				request: {},
				requestHeaders: { cookie: "auth.session=abc" },
				token: "",
			})

			expect(connectionConfig.readOnly).toBe(true)
		})

		// the document name may carry "default" rather than a branch id,
		// which is the branch core flags as the document's own
		it("locks the connection when the default branch is protected", async ({
			expect,
		}) => {
			const core = stubCore()
			core.fetchBranches.mockResolvedValue([
				{
					branchId: "branch-2",
					default: false,
					protected: false,
				},
				{
					branchId: "branch-1",
					default: true,
					protected: true,
				},
			])
			const hooks = hooksWith(core)
			const connectionConfig = { readOnly: false }

			await hooks.onAuthenticate({
				connectionConfig,
				documentName: "doc1-default",
				request: {},
				requestHeaders: { cookie: "auth.session=abc" },
				token: "",
			})

			expect(connectionConfig.readOnly).toBe(true)
		})

		it.for([
			{
				name: "the branch takes writes",
				branches: [
					{
						branchId: "branch1",
						default: false,
						protected: false,
					},
				],
			},
			{ name: "the branch is not known", branches: [] },
		])(
			"leaves the connection writable when $name",
			async ({ branches }, { expect }) => {
				const core = stubCore()
				core.fetchBranches.mockResolvedValue(branches)
				const hooks = hooksWith(core)
				const connectionConfig = { readOnly: false }

				await hooks.onAuthenticate({
					connectionConfig,
					documentName: "doc1-branch1",
					request: {},
					requestHeaders: {
						cookie: "auth.session=abc",
					},
					token: "",
				})

				expect(connectionConfig.readOnly).toBe(false)
			},
		)

		it("verifies the user's document access with their own headers", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = hooksWith(core)

			await hooks.onAuthenticate({
				connectionConfig: { readOnly: false },
				documentName: "doc1-branch1",
				request: {},
				requestHeaders: { cookie: "auth.session=abc" },
				token: "",
			})

			expect(core.verifyDocumentAccess).toHaveBeenCalledTimes(
				1,
			)
			const [documentId, options] =
				core.verifyDocumentAccess.mock.calls[0] ?? []
			expect(documentId).toBe("doc1")
			expect(options?.headers?.get("cookie")).toBe(
				"auth.session=abc",
			)
			// the branch is what says whether the connection may write
			expect.soft(core.fetchBranches).toHaveBeenCalledTimes(1)
		})

		it("rejects a connection with no session", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = hooksWith(core, null)

			await expect(
				hooks.onAuthenticate({
					connectionConfig: { readOnly: false },
					documentName: "doc1-branch1",
					request: {},
					requestHeaders: {},
					token: "",
				}),
			).rejects.toThrow("not authenticated")
			expect(core.verifyDocumentAccess).toHaveBeenCalledTimes(
				0,
			)
		})

		// the e2e suite uses this token to drive the failure path
		// without having to break the session
		it("rejects the force-error token even with a valid session", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = hooksWith(core)

			await expect(
				hooks.onAuthenticate({
					connectionConfig: { readOnly: false },
					documentName: "doc1-branch1",
					request: {},
					requestHeaders: {},
					token: "force-error",
				}),
			).rejects.toThrow("not authenticated")
			expect(core.verifyDocumentAccess).toHaveBeenCalledTimes(
				0,
			)
		})

		it("propagates a rejected access check", async ({ expect }) => {
			const failure = new Error("document not accessible")
			const core = stubCore()
			core.verifyDocumentAccess.mockRejectedValue(failure)
			const hooks = hooksWith(core)

			await expect(
				hooks.onAuthenticate({
					connectionConfig: { readOnly: false },
					documentName: "doc1-branch1",
					request: {},
					requestHeaders: {},
					token: "",
				}),
			).rejects.toBe(failure)
		})
	})

	describe("beforeHandleMessage", () => {
		// a placeholder for re-checking the session mid-connection; it
		// must still resolve, because hocuspocus awaits it on every
		// inbound message
		it("lets every message through", async ({ expect }) => {
			const hooks = hooksWith(stubCore())

			await expect(
				hooks.beforeHandleMessage(),
			).resolves.toBeUndefined()
		})
	})

	describe("onChange", () => {
		it("records the editing user as a maintainer of the document", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = hooksWith(core)

			await hooks.onChange({
				context: { session: SESSION },
				documentName: "doc1-branch1",
			})
			await hooks.onStoreDocument({
				documentName: "doc1-branch1",
				document: seededDocument(),
			})

			expect(
				core.storeBranchContent.mock.calls[0]?.[2]
					.maintainers,
			).toEqual(["user-1"])
		})

		it("records each editor once", async ({ expect }) => {
			const core = stubCore()
			const hooks = hooksWith(core)

			await hooks.onChange({
				context: { session: SESSION },
				documentName: "doc1-branch1",
			})
			await hooks.onChange({
				context: { session: SESSION },
				documentName: "doc1-branch1",
			})
			await hooks.onChange({
				context: {
					session: { user: { id: "user-2" } },
				},
				documentName: "doc1-branch1",
			})
			await hooks.onStoreDocument({
				documentName: "doc1-branch1",
				document: seededDocument(),
			})

			expect(
				core.storeBranchContent.mock.calls[0]?.[2]
					.maintainers,
			).toEqual(["user-1", "user-2"])
		})

		it("ignores a change carrying no session", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = hooksWith(core)

			await hooks.onChange({
				context: null,
				documentName: "doc1-branch1",
			})
			await hooks.onStoreDocument({
				documentName: "doc1-branch1",
				document: seededDocument(),
			})

			expect(
				core.storeBranchContent.mock.calls[0]?.[2]
					.maintainers,
			).toEqual([])
		})

		it("keeps each document's editors separate", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = hooksWith(core)

			await hooks.onChange({
				context: { session: SESSION },
				documentName: "doc1-branch1",
			})
			await hooks.onStoreDocument({
				documentName: "doc2-branch1",
				document: seededDocument(),
			})

			expect(
				core.storeBranchContent.mock.calls[0]?.[2]
					.maintainers,
			).toEqual([])
		})
	})

	describe("onLoadDocument", () => {
		it("restores the stored Yjs state when core has one", async ({
			expect,
		}) => {
			const stored = seededDocument("Stored name")
			const core = stubCore()
			core.fetchBranchContent.mockResolvedValue({
				documentName: "Stored name",
				content: { type: "doc", content: [] },
				icon: "lucide:file",
				rawContent: Buffer.from(
					Y.encodeStateAsUpdate(stored),
				).toString("base64"),
			})
			const hooks = hooksWith(core)

			const ydoc = await hooks.onLoadDocument({
				documentName: "doc1-branch1",
			})

			expect(fragmentXml(ydoc, "content")).toContain(
				"Restart it",
			)
			// the state is already persisted; writing it back would
			// only churn the row
			expect(core.storeBranchContent).toHaveBeenCalledTimes(0)
		})

		it("seeds a branch that has never been opened from its stored content", async ({
			expect,
		}) => {
			const core = stubCore()
			core.fetchBranchContent.mockResolvedValue({
				documentName: "Runbook",
				content: {
					type: "doc",
					content: [
						{
							type: "paragraph",
							attrs: { uid: "p1" },
							content: [
								{
									type: "text",
									text: "From core",
								},
							],
						},
					],
				},
				icon: "lucide:file",
				rawContent: null,
			})
			const hooks = hooksWith(core)

			const ydoc = await hooks.onLoadDocument({
				documentName: "doc1-branch1",
			})

			expect(fragmentXml(ydoc, "content")).toContain(
				"From core",
			)
			expect(ydoc.getText("icon").toJSON()).toBe(
				"lucide:file",
			)
		})

		// without this the next restart would build a second Y.Doc with
		// a different clientID, whose state CRDT-merges with the
		// reconnecting clients' and duplicates every block
		it("persists the seeded state immediately as a system write", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = hooksWith(core)

			await hooks.onLoadDocument({
				documentName: "doc1-branch1",
			})

			expect(core.storeBranchContent).toHaveBeenCalledTimes(1)
			const [documentId, branchId, update] =
				core.storeBranchContent.mock.calls[0] ?? []
			expect(documentId).toBe("doc1")
			expect(branchId).toBe("branch1")
			expect(update?.system).toBe(true)
			expect(update?.maintainers).toEqual([])
			expect(update?.rawContent).not.toBe("")
		})

		it("falls back to a placeholder when the stored content is not a document", async ({
			expect,
		}) => {
			const core = stubCore()
			core.fetchBranchContent.mockResolvedValue({
				documentName: "Runbook",
				content: { type: "fragment" },
				icon: "lucide:file",
				rawContent: null,
			})
			const hooks = hooksWith(core)

			const ydoc = await hooks.onLoadDocument({
				documentName: "doc1-branch1",
			})

			expect(fragmentXml(ydoc, "content")).toContain(
				"Hello World!",
			)
		})

		// a row with no content at all is broken, not empty. Seeding a
		// placeholder would persist over whatever it should have held.
		it.for([
			{ name: "null", input: null },
			{ name: "absent", input: undefined },
		])(
			"fails the load when the stored content is $name",
			async ({ input }, { expect }) => {
				const core = stubCore()
				core.fetchBranchContent.mockResolvedValue({
					documentName: "Runbook",
					content: input,
					icon: "lucide:file",
					rawContent: null,
				})
				const hooks = hooksWith(core)

				await expect(
					hooks.onLoadDocument({
						documentName: "doc1-branch1",
					}),
				).rejects.toThrow(
					"document doc1 branch branch1 has no stored content",
				)
				expect(
					core.storeBranchContent,
				).toHaveBeenCalledTimes(0)
			},
		)

		it("resolves the default branch before asking for its content", async ({
			expect,
		}) => {
			const core = stubCore()
			core.fetchBranches.mockResolvedValue([
				{
					branchId: "branch-1",
					default: true,
					protected: false,
				},
			])
			const hooks = hooksWith(core)

			await hooks.onLoadDocument({
				documentName: "doc1-default",
			})

			expect(core.fetchBranchContent).toHaveBeenCalledWith(
				"doc1",
				"branch-1",
			)
		})

		// the editor is already usable at this point; failing the load
		// would disconnect the client over a write it can retry
		it("still returns the document when persisting the seed fails", async ({
			expect,
		}) => {
			const core = stubCore()
			core.storeBranchContent.mockRejectedValue(
				new Error("core unreachable"),
			)
			const hooks = hooksWith(core)

			const ydoc = await hooks.onLoadDocument({
				documentName: "doc1-branch1",
			})

			expect(ydoc).toBeInstanceOf(Y.Doc)
		})

		it("propagates a failed content fetch", async ({ expect }) => {
			const failure = new Error("core unreachable")
			const core = stubCore()
			core.fetchBranchContent.mockRejectedValue(failure)
			const hooks = hooksWith(core)

			await expect(
				hooks.onLoadDocument({
					documentName: "doc1-branch1",
				}),
			).rejects.toBe(failure)
		})
	})

	describe("onStoreDocument", () => {
		it("persists the document's name, icon and content as a user write", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = hooksWith(core)

			await hooks.onStoreDocument({
				documentName: "doc1-branch1",
				document: seededDocument("Runbook"),
			})

			expect(core.storeBranchContent).toHaveBeenCalledTimes(1)
			const [documentId, branchId, update] =
				core.storeBranchContent.mock.calls[0] ?? []
			expect(documentId).toBe("doc1")
			expect(branchId).toBe("branch1")
			expect(update?.name).toBe("Runbook")
			expect(update?.icon).toBe("lucide:file")
			expect(update?.system).toBe(false)
			expect(update?.rawContent).not.toBe("")
		})

		it("persists content the transformer can read back", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = hooksWith(core)
			const document = seededDocument()

			await hooks.onStoreDocument({
				documentName: "doc1-branch1",
				document,
			})

			expect(
				core.storeBranchContent.mock.calls[0]?.[2]
					.content,
			).toEqual(transformer.fromYdoc(document, "content"))
		})

		// the maintainers field is who edited during this persist, not
		// the document's whole maintainer set, so it has to start empty
		// again after every write
		it("sends no maintainers on a second persist with no edits between", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = hooksWith(core)

			await hooks.onChange({
				context: { session: SESSION },
				documentName: "doc1-branch1",
			})
			await hooks.onStoreDocument({
				documentName: "doc1-branch1",
				document: seededDocument(),
			})
			await hooks.onStoreDocument({
				documentName: "doc1-branch1",
				document: seededDocument(),
			})

			expect(
				core.storeBranchContent.mock.calls[1]?.[2]
					.maintainers,
			).toEqual([])
		})

		it("names an untitled document rather than persisting an empty name", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = hooksWith(core)

			await hooks.onStoreDocument({
				documentName: "doc1-branch1",
				document: connectedDocument(),
			})

			expect(
				core.storeBranchContent.mock.calls[0]?.[2].name,
			).toBe("Untitled Document")
		})

		it("persists as core's own write when only core changed the document", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = hooksWith(core)
			const document = seededDocument()

			await hooks.onChange({
				context: { system: true },
				documentName: "doc1-branch1",
			})
			await hooks.onStoreDocument({
				documentName: "doc1-branch1",
				document,
			})

			expect(
				core.storeBranchContent.mock.calls[0]?.[2]
					.system,
			).toBe(true)
		})

		// core's permission covers the change core made, not whatever
		// else happened to be pending in the same window
		it("persists as an ordinary write when an edit shares the window", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = hooksWith(core)
			const document = seededDocument()

			await hooks.onChange({
				context: { system: true },
				documentName: "doc1-branch1",
			})
			await hooks.onChange({
				context: { session: SESSION },
				documentName: "doc1-branch1",
			})
			await hooks.onStoreDocument({
				documentName: "doc1-branch1",
				document,
			})

			expect(
				core.storeBranchContent.mock.calls[0]?.[2]
					.system,
			).toBe(false)
		})

		it("persists the next window on its own provenance", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = hooksWith(core)
			const document = seededDocument()

			await hooks.onChange({
				context: { system: true },
				documentName: "doc1-branch1",
			})
			await hooks.onStoreDocument({
				documentName: "doc1-branch1",
				document,
			})
			await hooks.onChange({
				context: { session: SESSION },
				documentName: "doc1-branch1",
			})
			await hooks.onStoreDocument({
				documentName: "doc1-branch1",
				document,
			})

			expect(
				core.storeBranchContent.mock.calls[1]?.[2]
					.system,
			).toBe(false)
		})

		// the editors keep their unsaved work either way; telling them
		// the persist failed is the only thing left to do
		it("warns the connected editors instead of throwing when the persist fails", async ({
			expect,
		}) => {
			const core = stubCore()
			core.storeBranchContent.mockRejectedValue(
				new Error("core unreachable"),
			)
			const hooks = hooksWith(core)
			const document = seededDocument()

			await hooks.onStoreDocument({
				documentName: "doc1-branch1",
				document,
			})

			expect(
				document.broadcastStateless,
			).toHaveBeenCalledWith(
				JSON.stringify({
					type: "error",
					code: "hocuspocus.store_failed",
				}),
			)
		})

		it("warns the connected editors when the branch cannot be resolved", async ({
			expect,
		}) => {
			const core = stubCore()
			core.fetchBranches.mockResolvedValue([])
			const hooks = hooksWith(core)
			const document = seededDocument()

			await hooks.onStoreDocument({
				documentName: "doc1-default",
				document,
			})

			expect(
				document.broadcastStateless,
			).toHaveBeenCalledTimes(1)
			expect(core.storeBranchContent).toHaveBeenCalledTimes(0)
		})
	})

	describe("flushDocument", () => {
		// a server holding one open document whose store may be pending
		// or running. executeNow stands in for hocuspocus running the
		// debounced store: it calls the hook the way the server would.
		function stubServer(
			hooks: ReturnType<typeof hooksWith>,
			document: ConnectedDocument | null,
			state: "pending" | "running" | "idle",
		) {
			const documents = new Map<string, ConnectedDocument>()
			if (document) {
				documents.set("doc1-branch1", document)
			}

			const executeNow = vi.fn(() =>
				document
					? hooks.onStoreDocument({
							documentName:
								"doc1-branch1",
							document,
						})
					: undefined,
			)

			return {
				server: {
					documents,
					debouncer: {
						isDebounced: () =>
							state === "pending",
						isCurrentlyExecuting: () =>
							state === "running",
						executeNow,
					},
				},
				executeNow,
			}
		}

		it("runs the pending store now and resolves once core has it", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = hooksWith(core)
			const { server, executeNow } = stubServer(
				hooks,
				seededDocument("Runbook"),
				"pending",
			)

			await hooks.flushDocument(server, "doc1-branch1")

			expect(executeNow).toHaveBeenCalledWith(
				"onStoreDocument-doc1-branch1",
			)
			expect(core.storeBranchContent).toHaveBeenCalledTimes(1)
			const [, , update] =
				core.storeBranchContent.mock.calls[0] ?? []
			expect(update?.name).toBe("Runbook")
		})

		it("leaves a document that is not open alone", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = hooksWith(core)
			const { server, executeNow } = stubServer(
				hooks,
				null,
				"pending",
			)

			await hooks.flushDocument(server, "doc1-branch1")

			expect(executeNow).toHaveBeenCalledTimes(0)
			expect(core.storeBranchContent).toHaveBeenCalledTimes(0)
		})

		it("leaves a document with nothing pending alone", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = hooksWith(core)
			const { server, executeNow } = stubServer(
				hooks,
				seededDocument(),
				"idle",
			)

			await hooks.flushDocument(server, "doc1-branch1")

			expect(executeNow).toHaveBeenCalledTimes(0)
			expect(core.storeBranchContent).toHaveBeenCalledTimes(0)
		})

		// the store hook swallows the failure to warn the editors; the
		// flush is the one caller that needs to see it.
		it("rejects with the store's failure and still warns the editors", async ({
			expect,
		}) => {
			const core = stubCore()
			core.storeBranchContent.mockRejectedValue(
				new Error("core unreachable"),
			)
			const hooks = hooksWith(core)
			const document = seededDocument()
			const { server } = stubServer(
				hooks,
				document,
				"pending",
			)

			await expect(
				hooks.flushDocument(server, "doc1-branch1"),
			).rejects.toThrow("core unreachable")
			expect(
				document.broadcastStateless,
			).toHaveBeenCalledWith(
				JSON.stringify({
					type: "error",
					code: "hocuspocus.store_failed",
				}),
			)
		})

		it("waits for a store already in flight", async ({
			expect,
		}) => {
			const core = stubCore()
			let finish: () => void = () => undefined
			core.storeBranchContent.mockReturnValue(
				new Promise<void>((resolve) => {
					finish = resolve
				}),
			)
			const hooks = hooksWith(core)
			const document = seededDocument()
			const { server, executeNow } = stubServer(
				hooks,
				document,
				"running",
			)
			const inFlight = hooks.onStoreDocument({
				documentName: "doc1-branch1",
				document,
			})

			let flushed = false
			const flush = hooks
				.flushDocument(server, "doc1-branch1")
				.then(() => {
					flushed = true
				})
			await Promise.resolve()

			expect(flushed).toBe(false)
			finish()
			await Promise.all([inFlight, flush])
			expect(flushed).toBe(true)
			expect(executeNow).toHaveBeenCalledTimes(0)
			expect(core.storeBranchContent).toHaveBeenCalledTimes(1)
		})

		// a persist that failed before the flush was asked for is not
		// the flush's answer: nothing is pending, so nothing is awaited.
		it("does not report a failure that predates it", async ({
			expect,
		}) => {
			const core = stubCore()
			core.storeBranchContent.mockRejectedValueOnce(
				new Error("core unreachable"),
			)
			const hooks = hooksWith(core)
			const document = seededDocument()
			await hooks.onStoreDocument({
				documentName: "doc1-branch1",
				document,
			})
			const { server } = stubServer(hooks, document, "idle")

			await expect(
				hooks.flushDocument(server, "doc1-branch1"),
			).resolves.toBeUndefined()
		})
	})
})

describe("resetConnections", () => {
	function socket() {
		return { webSocket: { close: vi.fn() } }
	}

	// the provider reopens a document after losing its socket, which is
	// what makes it authenticate again and pick up the branch's new
	// permission
	it("closes every client's socket with the reset code", ({ expect }) => {
		const first = socket()
		const second = socket()
		const server = {
			documents: new Map([
				[
					"doc1-branch1",
					{
						connections: new Map([
							[first, {}],
							[second, {}],
						]),
					},
				],
			]),
		}

		resetConnections(server, "doc1-branch1")

		expect(first.webSocket.close).toHaveBeenCalledWith(
			4205,
			"Reset Connection",
		)
		expect(second.webSocket.close).toHaveBeenCalledWith(
			4205,
			"Reset Connection",
		)
	})

	it("leaves other documents' sockets alone", ({ expect }) => {
		const other = socket()
		const server = {
			documents: new Map([
				[
					"doc1-branch2",
					{ connections: new Map([[other, {}]]) },
				],
			]),
		}

		resetConnections(server, "doc1-branch1")

		expect(other.webSocket.close).toHaveBeenCalledTimes(0)
	})
})

describe("createHocuspocus", () => {
	it("sends the hocuspocus logger's per-connection lines out at DEBUG", ({
		expect,
	}) => {
		const log = stubLog()
		const { instance } = createHocuspocus({
			auth: stubAuth(),
			core: stubCore(),
			log,
		})
		// the extension keeps its sink in its own configuration; the
		// server exposes no other way to reach it.
		const logger = instance.configuration.extensions[0] as {
			configuration: { log: (...args: unknown[]) => void }
		}

		logger.configuration.log("onStoreDocument", "doc-1")

		expect(log.debug.mock.calls).toEqual([
			["onStoreDocument doc-1"],
		])
		expect(log.info.mock.calls).toEqual([])
	})

	it("wires the document hooks onto the server", ({ expect }) => {
		const { instance } = createHocuspocus({
			auth: stubAuth(),
			core: stubCore(),
			log: stubLog(),
		})

		expect(typeof instance.configuration.onAuthenticate).toBe(
			"function",
		)
		expect(typeof instance.configuration.onLoadDocument).toBe(
			"function",
		)
		expect(typeof instance.configuration.onStoreDocument).toBe(
			"function",
		)
	})

	// the flush is bound to the server it came with: a document the
	// server does not hold cannot have a pending store to run.
	it("flushes against its own server", async ({ expect }) => {
		const core = stubCore()
		const { flushDocument } = createHocuspocus({
			auth: stubAuth(),
			core,
			log: stubLog(),
		})

		await flushDocument("doc1-branch1")

		expect(core.storeBranchContent).toHaveBeenCalledTimes(0)
	})
	it("resets against its own server", ({ expect }) => {
		const { resetConnections } = createHocuspocus({
			auth: stubAuth(),
			core: stubCore(),
			log: stubLog(),
		})

		expect(() => {
			resetConnections("doc1-branch1")
		}).not.toThrow()
	})
})
