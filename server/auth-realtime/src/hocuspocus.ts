import * as Sentry from "@sentry/node"
import * as Y from "yjs"
import { nanoid } from "nanoid"
import { Hocuspocus } from "@hocuspocus/server"
import { Logger } from "@hocuspocus/extension-logger"
// aliased because the extension above already claims the name Logger.
import type { Logger as ServiceLogger } from "./logging.js"
import type { CoreClient } from "./core.js"
import { toAxiosHeaders, toHeaders } from "./headers.js"
import { bestEffort, reported } from "./reporting.js"
import {
	isSystemContext,
	replaceYdocContent,
	transformer,
} from "./ydocument.js"

export interface AuthSession {
	user: { id: string }
}

// the one thing the document hooks ask of better-auth. Narrowing it here
// keeps the hooks drivable from a test with a two-line stub instead of a
// configured auth instance.
export interface SessionResolver {
	getSession(input: { headers: Headers }): Promise<AuthSession | null>
}

export interface DocumentHookDeps {
	auth: SessionResolver
	core: CoreClient
}

// hocuspocus's Document adds the stateless broadcast the store hook uses
// to tell connected editors that a persist failed.
interface ConnectedDocument extends Y.Doc {
	broadcastStateless(payload: string): void
}

// parseDocumentName splits a document name of the form
// "documentId-branchIdentifier" into its component parts. The split is on
// the first dash: xids carry none, but a branch identifier may.
export function parseDocumentName(documentName: string): {
	documentId: string
	branchIdentifier: string
} {
	const idx = documentName.indexOf("-")

	return {
		documentId: documentName.slice(0, idx),
		branchIdentifier: documentName.slice(idx + 1),
	}
}

// resolveBranchId resolves a branchIdentifier to a concrete branch ID.
// When the identifier is "default", it fetches the document's branches and
// returns the one flagged as the default.
export async function resolveBranchId(
	core: CoreClient,
	documentId: string,
	branchIdentifier: string,
): Promise<string> {
	if (branchIdentifier !== "default") {
		return branchIdentifier
	}

	const branches = await core.fetchBranches(documentId)
	const defaultBranch = branches.find((b) => b.default)

	if (!defaultBranch) {
		throw new Error(
			`no default branch found for document ${documentId}`,
		)
	}

	return defaultBranch.branchId
}

// branchIsProtected reports whether the branch takes writes from a person
// at all. A branch that does not is read-only on the wire: keeping an edit
// out of the document is the only way to keep it out of the whole-document
// state a later persist carries, whoever that persist belongs to.
async function branchIsProtected(
	core: CoreClient,
	documentId: string,
	branchIdentifier: string,
): Promise<boolean> {
	const branches = await core.fetchBranches(documentId)
	const branch = branches.find((b) =>
		branchIdentifier === "default"
			? b.default
			: b.branchId === branchIdentifier,
	)

	return branch?.protected ?? false
}

// the placeholder a branch gets when core has content for it that is not a
// prosemirror document — a branch that has never been written to.
function defaultDocumentContent() {
	return {
		type: "doc",
		content: [
			{
				type: "paragraph",
				attrs: { uid: nanoid() },
				content: [
					{ type: "text", text: "Hello World!" },
				],
			},
		],
	}
}

// core stores whatever the last persist wrote. Content that is not a
// prosemirror document belongs to a branch nobody has written to yet, and
// gets the placeholder — but a branch with no content at all is a broken
// row, and failing the load is what surfaces that. Seeding an empty
// document instead would persist over whatever the row should have held.
function seedContent(
	content: unknown,
	documentId: string,
	branchId: string,
): unknown {
	if (content === null || content === undefined) {
		throw new Error(
			`document ${documentId} branch ${branchId} has no stored content`,
		)
	}

	if (
		typeof content === "object" &&
		(content as { type?: unknown }).type === "doc"
	) {
		return content
	}

	return defaultDocumentContent()
}

function documentTitle(doc: Y.Doc): string {
	const name = transformer.fromYdoc(doc, "name") as {
		content?: { content?: { text?: string }[] }[]
	}

	return name.content?.[0]?.content?.[0]?.text || "Untitled Document"
}

function encodeState(doc: Y.Doc): string {
	return Buffer.from(Y.encodeStateAsUpdate(doc)).toString("base64")
}

export function createDocumentHooks({ auth, core }: DocumentHookDeps) {
	// who edited each open document since its last persist. Held per
	// hooks instance so nothing carries over between servers — or, in a
	// test, between cases.
	const documentMaintainers = new Map<string, Set<string>>()

	// whether every change to each open document since its last persist
	// came from core. A document nobody has changed is absent, and a
	// persist of one reads as an ordinary write: core's permission is
	// only ever lent to changes seen arriving under it.
	const documentSystemOnly = new Map<string, boolean>()

	return {
		async onAuthenticate({
			connectionConfig,
			documentName,
			request,
			requestHeaders,
			token,
		}: {
			connectionConfig: { readOnly: boolean }
			documentName: string
			request: any
			requestHeaders: any
			token: string
		}) {
			const { documentId, branchIdentifier } =
				parseDocumentName(documentName)

			const session = await auth.getSession({
				headers: toHeaders(request, requestHeaders),
			})
			if (!session || token === "force-error") {
				throw new Error("not authenticated")
			}

			// verify the user has access to this document through
			// core's session-authed access endpoint. The caller's
			// own headers are forwarded, so core rejects a document
			// the user's organization does not own.
			await reported(() =>
				core.verifyDocumentAccess(documentId, {
					headers: toAxiosHeaders(requestHeaders),
				}),
			)

			connectionConfig.readOnly = await reported(() =>
				branchIsProtected(
					core,
					documentId,
					branchIdentifier,
				),
			)

			return { session }
		},

		// TODO kick users if auth becomes invalid later
		beforeHandleMessage() {
			// periodically re-check better auth session and close
			// the socket if needed
			return Promise.resolve()
		},

		onChange({
			context,
			documentName,
		}: {
			context?: {
				session?: AuthSession | null
				system?: boolean
			} | null
			documentName: string
		}) {
			documentSystemOnly.set(
				documentName,
				(documentSystemOnly.get(documentName) ??
					true) &&
					isSystemContext(context),
			)

			const userId = context?.session?.user.id
			if (!userId) {
				return Promise.resolve()
			}

			let set = documentMaintainers.get(documentName)
			if (!set) {
				set = new Set()
				documentMaintainers.set(documentName, set)
			}

			set.add(userId)

			return Promise.resolve()
		},

		onLoadDocument({
			documentName,
		}: {
			documentName: string
		}): Promise<Y.Doc> {
			return reported(async () => {
				const { documentId, branchIdentifier } =
					parseDocumentName(documentName)
				const branchId = await resolveBranchId(
					core,
					documentId,
					branchIdentifier,
				)

				const branch = await core.fetchBranchContent(
					documentId,
					branchId,
				)

				if (branch.rawContent) {
					const ydoc = new Y.Doc()

					Y.applyUpdate(
						ydoc,
						new Uint8Array(
							Buffer.from(
								branch.rawContent,
								"base64",
							),
						),
					)

					return ydoc
				}

				const ydoc = new Y.Doc()

				replaceYdocContent(ydoc, {
					name: branch.documentName,
					content: seedContent(
						branch.content,
						documentId,
						branchId,
					),
					icon: branch.icon,
				})

				// persist rawContent immediately to prevent
				// content duplication. Without this, if the
				// server restarts before onStoreDocument runs,
				// onLoadDocument would create a new Y.Doc with a
				// different clientID, causing the reconnecting
				// client's existing state to merge with the new
				// state instead of being recognized as identical
				// — duplicating all content.
				await bestEffort(() =>
					core.storeBranchContent(
						documentId,
						branchId,
						{
							name: branch.documentName,
							icon: branch.icon,
							content: branch.content,
							maintainers: [],
							rawContent: encodeState(
								ydoc,
							),
							system: true,
						},
					),
				)

				return ydoc
			})
		},

		async onStoreDocument(data: {
			documentName: string
			document: ConnectedDocument
		}) {
			// both maps describe the window this persist closes, so
			// they are drained before anything that can fail: a
			// throw on the way to core must not leave one window's
			// answer standing for the next.
			const maintainers = Array.from(
				documentMaintainers.get(data.documentName) ??
					[],
			)
			documentMaintainers.delete(data.documentName)

			// nothing but core changed the document, so the persist
			// carries core's own permission and a protected branch
			// accepts it. One edit in the window is enough to make
			// it an ordinary write.
			const system =
				documentSystemOnly.get(data.documentName) ??
				false

			documentSystemOnly.delete(data.documentName)

			try {
				const { documentId, branchIdentifier } =
					parseDocumentName(data.documentName)
				const branchId = await resolveBranchId(
					core,
					documentId,
					branchIdentifier,
				)

				await core.storeBranchContent(
					documentId,
					branchId,
					{
						name: documentTitle(
							data.document,
						),
						icon: data.document
							.getText("icon")
							.toJSON(),
						content: transformer.fromYdoc(
							data.document,
							"content",
						) as unknown,
						maintainers,
						rawContent: encodeState(
							data.document,
						),
						system,
					},
				)
			} catch (err) {
				Sentry.captureException(err)

				data.document.broadcastStateless(
					JSON.stringify({
						type: "error",
						code: "hocuspocus.store_failed",
					}),
				)
			}
		},
	}
}

export function createHocuspocus({
	log,
	...hooks
}: DocumentHookDeps & { log: ServiceLogger }): Hocuspocus {
	return new Hocuspocus({
		// a line per connection, load, change and persist: a traffic
		// trace, so DEBUG.
		extensions: [
			new Logger({
				log: (...args: unknown[]) => {
					log.debug(
						args
							.map((arg) =>
								String(arg),
							)
							.join(" "),
					)
				},
			}),
		],
		...createDocumentHooks(hooks),
	})
}
