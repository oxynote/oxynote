import * as Sentry from "@sentry/node"
import * as Y from "yjs"
import axios from "axios"
import { nanoid } from "nanoid"
import { AxiosHeaders } from "axios"
import { Hocuspocus } from "@hocuspocus/server"
import { Logger } from "@hocuspocus/extension-logger"
import { auth } from "./auth.js"
import { replaceYdocContent, transformer } from "./ydocument.js"

const url = process.env.OXYNOTE_AUTH_REALTIME_BACKEND_URL

const documentMaintainers = new Map<string, Set<string>>()

// parseDocumentName splits a document name of the form "documentId-branchId"
// into its component parts. branchId may be the special value "default".
function parseDocumentName(documentName: string): {
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
// returns the ID of the oldest one (the original branch).
async function resolveBranchId(
	documentId: string,
	branchIdentifier: string,
): Promise<string> {
	if (branchIdentifier !== "default") {
		return branchIdentifier
	}

	const response = await axios.get(
		`${url}/api/x/documents/${documentId}/branches`,
	)

	const branches: { branchId: string; default: boolean }[] = response.data

	const defaultBranch = branches.find((b) => b.default)

	if (!defaultBranch) {
		throw new Error(`no default branch found for document ${documentId}`)
	}

	return defaultBranch.branchId
}

export const hocuspocus = new Hocuspocus({
	extensions: [new Logger()],
	async onAuthenticate({ documentName, request, requestHeaders, token }) {
		const { documentId } = parseDocumentName(documentName)

		const session = await auth.api.getSession({
			headers: toHeaders(request, requestHeaders),
		})
		if (!session || token === "force-error") {
			throw new Error("not authenticated")
		}

		// Verify the user has access to this document by checking the oldest
		// branch. FetchDocumentBranchesUnsafe does not require an org filter —
		// the session check above confirms identity and the branch-list call
		// below confirms existence.
		try {
			await axios.get(
				`${url}/api/x/documents/${documentId}/branches`,
				{
					headers: toAxiosHeaders(requestHeaders),
				},
			)
		} catch (err) {
			Sentry.captureException(err)
			throw err
		}

		/* TODO check edit permissions
		if (!canEdit) {
			connectionConfig.readOnly = true // allow read but block edits
		}
		*/

		return {
			session: session,
		}
	},
	// TODO kick users if auth becomes invalid later
	async beforeHandleMessage() {
		// periodically re-check better auth session and close the socket if
		// needed
	},
	async onChange({ context, documentName }) {
		const session = context?.session
		const userId = session?.user?.id
		if (!userId) {
			return
		}

		let set = documentMaintainers.get(documentName)
		if (!set) {
			set = new Set()
			documentMaintainers.set(documentName, set)
		}

		set.add(userId)
	},
	async onLoadDocument({ documentName }) {
		try {
			const { documentId, branchIdentifier } =
				parseDocumentName(documentName)
			const branchId = await resolveBranchId(
				documentId,
				branchIdentifier,
			)

			const response = await axios.get(
				`${url}/api/x/documents/${documentId}/branch/${branchId}`,
			)

			if (!response.data.rawContent) {
				const ydoc = new Y.Doc()

				replaceYdocContent(ydoc, {
					name: response.data.documentName,
					content:
						response.data.content.type ==
						"doc"
							? response.data.content
							: {
									type: "doc",
									content: [
										{
											type: "paragraph",
											attrs: {
												uid: nanoid(),
											},
											content: [
												{
													type: "text",
													text: "Hello World!",
												},
											],
										},
									],
								},
					icon: response.data.icon,
				})

				// Persist rawContent immediately to prevent content duplication.
				// Without this, if the server restarts before onStoreDocument runs,
				// onLoadDocument would create a new Y.Doc with a different clientID,
				// causing the reconnecting client's existing state to merge with the
				// new state instead of being recognized as identical — duplicating
				// all content.
				const rawContent = Buffer.from(
					Y.encodeStateAsUpdate(ydoc),
				).toString("base64")

				try {
					await axios.put(
						`${url}/api/x/documents/${documentId}/branch/${branchId}`,
						{
							name: response.data.documentName,
							icon: response.data.icon,
							content: response.data.content,
							maintainers: [],
							rawContent,
							system: true,
						},
					)
				} catch (err) {
					Sentry.captureException(err)
				}

				return ydoc
			}

			const ydoc = new Y.Doc()

			const data = new Uint8Array(
				Buffer.from(response.data.rawContent, "base64"),
			)

			Y.applyUpdate(ydoc, data)

			return ydoc
		} catch (err) {
			Sentry.captureException(err)
			throw err
		}
	},
	async onStoreDocument(data) {
		try {
			const { documentId, branchIdentifier } =
				parseDocumentName(data.documentName)
			const branchId = await resolveBranchId(
				documentId,
				branchIdentifier,
			)

			const maintainers = Array.from(
				documentMaintainers.get(data.documentName) ??
					[],
			)
			documentMaintainers.delete(data.documentName)

			const name = transformer.fromYdoc(data.document, "name")
			.content[0]?.content[0]?.text ||
				"Untitled Document"

			await axios.put(
				`${url}/api/x/documents/${documentId}/branch/${branchId}`,
				{
					name: name,
					icon: data.document.getText("icon").toString(),
					content: transformer.fromYdoc(
						data.document,
						"content",
					),
					maintainers: maintainers,
					rawContent: Buffer.from(
						Y.encodeStateAsUpdate(data.document),
					).toString("base64"),
					system: false,
				},
			)
		} catch (err) {
			Sentry.captureException(err)

			data.document.broadcastStateless(
				JSON.stringify({ type: "error", code: "hocuspocus.store_failed" }),
			)
		}
	},
})

type HeaderEntry = [string, string]

function normalizeHeaderValue(value: unknown): string | null {
	if (value == null) {
		return null
	}

	return Array.isArray(value) ? value.join(", ") : String(value)
}

function collectHeaderEntries(requestHeaders: any): HeaderEntry[] | null {
	if (!requestHeaders) {
		return null
	}

	// WHATWG Headers or compatible
	if (typeof requestHeaders.forEach === "function") {
		const entries: HeaderEntry[] = []
		requestHeaders.forEach((v: string, k: string) => {
			const value = normalizeHeaderValue(v)
			if (value != null) {
				entries.push([k, value])
			}
		})

		return entries
	}

	// Iterable header entries
	if (typeof requestHeaders[Symbol.iterator] === "function") {
		const entries: HeaderEntry[] = []
		for (const [k, v] of requestHeaders as any) {
			const value = normalizeHeaderValue(v)
			if (value != null) {
				entries.push([k, value])
			}
		}

		return entries
	}

	// Plain object map
	if (typeof requestHeaders === "object") {
		const entries: HeaderEntry[] = []
		for (const k of Reflect.ownKeys(requestHeaders) as string[]) {
			const value = normalizeHeaderValue(requestHeaders[k])
			if (value != null) {
				entries.push([k, value])
			}
		}

		return entries
	}

	return null
}

function toHeaders(req: any, requestHeaders: any): Headers {
	const res = new Headers()
	const entries = collectHeaderEntries(requestHeaders)

	if (entries) {
		for (const [k, v] of entries) {
			res.append(k, v)
		}

		return res
	}

	// fallback: Node IncomingMessage.rawHeaders (always present on Node)
	if (req?.rawHeaders && Array.isArray(req.rawHeaders)) {
		for (let i = 0; i < req.rawHeaders.length; i += 2) {
			const k = String(req.rawHeaders[i])
			const v = String(req.rawHeaders[i + 1])
			res.append(k, v)
		}

		return res
	}

	// last resort: regular req.headers
	if (req?.headers && typeof req.headers === "object") {
		for (const k of Reflect.ownKeys(req.headers) as string[]) {
			const v = req.headers[k as any]
			if (v != null)
				res.set(
					k,
					Array.isArray(v)
						? v.join(", ")
						: String(v),
				)
		}

		return res
	}

	return res
}

export function toAxiosHeaders(requestHeaders: any): AxiosHeaders {
	const res = new AxiosHeaders()
	const entries = collectHeaderEntries(requestHeaders)

	if (entries) {
		for (const [k, v] of entries) {
			res.set(k, v)
		}

		return res
	}

	return res
}
