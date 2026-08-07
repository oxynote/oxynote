import * as Sentry from "@sentry/node"
import { Hono } from "hono"
import type { ContentfulStatusCode } from "hono/utils/http-status"
import { cors } from "hono/cors"
import {
	auth,
	totalOrganizationCount,
	AUTH_METHODS,
	MAX_ORGANIZATIONS,
} from "./auth.js"
import * as Y from "yjs"
import { hocuspocus, toAxiosHeaders } from "./hocuspocus.js"
import { replaceYdocContent } from "./ydocument.js"
import { applyOperations, type Operation } from "./operations.js"
import axios from "axios"

const app = new Hono()
const backendUrl = process.env.OXYNOTE_AUTH_REALTIME_BACKEND_URL

app.use(
	"*",
	cors({
		origin: (
			(process.env.OXYNOTE_AUTH_REALTIME_TRUSTED_ORIGINS ||
				"") as string
		).split(","),
		allowMethods: [
			"GET",
			"POST",
			"PUT",
			"PATCH",
			"DELETE",
			"OPTIONS",
		],
		allowHeaders: ["Content-Type", "Authorization"],
		credentials: true,
	}),
)

app.get("/", (c) => {
	return c.text("Hello Hono!")
})

app.on(["POST", "GET"], "/auth/**", (c) => {
	return auth.handler(c.req.raw)
})

// public auth configuration for the login/signup pages: which social
// providers are configured (and, later, other signup-related capability
// flags). Lives outside the better-auth /auth/** namespace.
app.get("/auth-config", (c) => {
	return c.json({ methods: AUTH_METHODS })
})

app.get("/organizations/stats", async (c) => {
	const count = await totalOrganizationCount()
	return c.json({ availableSlots: MAX_ORGANIZATIONS - count })
})

// auth is delegated: the caller's headers (including the session cookie)
// are forwarded to core, whose middleware validates the session and
// authorizes the merge. The in-memory Y.Doc mutation below only runs when
// core responds 200, so an unauthenticated caller changes nothing.
app.put("/documents/:documentId/merge", async (c) => {
	const documentId = c.req.param("documentId")

	let fromBranchId: string
	let toBranchId: string

	try {
		const body = await c.req.json()
		fromBranchId = body.fromBranchId
		toBranchId = body.toBranchId
	} catch {
		return c.json({ error: "invalid request body" }, 400)
	}

	if (!fromBranchId || !toBranchId) {
		return c.json(
			{ error: "fromBranchId and toBranchId are required" },
			400,
		)
	}

	try {
		const response = await axios.put(
			`${backendUrl}/api/documents/${documentId}/merge`,
			{ fromBranchId, toBranchId },
			{
				headers: toAxiosHeaders(c.req.raw.headers),
			},
		)

		if (response.status === 200) {
			const mergedData = {
				name: response.data.documentName,
				content: response.data.content,
				icon: response.data.icon,
			}

			// update the in-memory target branch Y.Doc using direct cloning to
			// avoid Y.applyUpdate merge semantics that cause content
			// duplication when source and target docs have different
			// client origins.
			const toDocument = hocuspocus.documents.get(
				`${documentId}-${toBranchId}`,
			)
			if (toDocument) {
				replaceYdocContent(toDocument, mergedData)

				// persist rawContent immediately because MergeBranches
				// sets rawContent to nil in the database. Without
				// this, a server restart before onStoreDocument
				// would trigger onLoadDocument's first-time path,
				// creating a new Y.Doc with a different clientID
				// that CRDT-merges with reconnecting clients and
				// duplicates content.
				const rawContent = Buffer.from(
					Y.encodeStateAsUpdate(toDocument),
				).toString("base64")

				try {
					await axios.put(
						`${backendUrl}/api/x/documents/${documentId}/branch/${toBranchId}`,
						{
							name: mergedData.name,
							icon: mergedData.icon,
							content: mergedData.content,
							maintainers: [],
							rawContent,
							system: true,
						},
					)
				} catch (err) {
					Sentry.captureException(err)
				}
			}
		}

		return c.json(
			response.data,
			response.status as ContentfulStatusCode,
		)
	} catch (error: any) {
		Sentry.captureException(error)

		if (error.response) {
			return c.json(
				error.response.data,
				error.response.status,
			)
		}

		return c.json({ error: "failed to merge" }, 500)
	}
})

// Server-to-server endpoint called by the Go assistant to apply
// edit operations to a live Y.Doc. Each request opens (or reuses) a
// direct connection to the document, runs the batch inside a single
// Y.Doc transaction, and disconnects. Hocuspocus handles
// broadcasting the resulting update to subscribed clients and
// persisting via onStoreDocument.
//
// This route is service-to-service only, like the Go `/api/x/...`
// endpoints: core calls it directly over the container network, and
// the reverse proxy blocks `/auth-realtime/api/internal/*` at the
// front door.
app.post(
	"/internal/documents/:documentId/branches/:branchId/operations",
	async (c) => {
		const documentId = c.req.param("documentId")
		const branchId = c.req.param("branchId")

		let body: { operations?: Operation[] }
		try {
			body = await c.req.json()
		} catch {
			return c.json({ error: "invalid request body" }, 400)
		}

		const ops = body.operations
		if (!Array.isArray(ops)) {
			return c.json(
				{ error: "operations array is required" },
				400,
			)
		}

		// Hocuspocus addresses documents by `documentId-branchId`; the
		// Go caller resolves "default" to a concrete branchId before
		// sending so this path can be a direct lookup.
		const documentName = `${documentId}-${branchId}`

		let connection
		try {
			connection =
				await hocuspocus.openDirectConnection(
					documentName,
				)
		} catch (err) {
			Sentry.captureException(err)
			return c.json({ error: "failed to open document" }, 500)
		}

		try {
			let result: {
				applied: number
				errors: { index: number; message: string }[]
			} = {
				applied: 0,
				errors: [],
			}

			await connection.transact((doc) => {
				result = applyOperations(doc, ops)
			})

			return c.json(result, 200)
		} catch (err) {
			Sentry.captureException(err)
			return c.json(
				{ error: "failed to apply operations" },
				500,
			)
		} finally {
			try {
				await connection.disconnect()
			} catch (err) {
				Sentry.captureException(err)
			}
		}
	},
)

export default app
