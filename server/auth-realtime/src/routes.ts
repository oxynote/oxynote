import * as Sentry from "@sentry/node"
import { Hono } from "hono"
import type { ContentfulStatusCode } from "hono/utils/http-status"
import { cors } from "hono/cors"
import { createLocalJWKSet, jwtVerify } from "jose"
import {
	auth,
	totalOrganizationCount,
	AUTH_METHODS,
	MAX_ORGANIZATIONS,
	MCP_RESOURCE,
	MCP_TOKEN_ISSUER,
} from "./auth.js"
import { db } from "./database.js"
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

// cached JWKS for MCP access-token verification. Cleared on a failed
// verification so a key rotation is picked up on the next request.
let mcpKeySet: ReturnType<typeof createLocalJWKSet> | null = null

async function mcpTokenKeySet(): Promise<ReturnType<typeof createLocalJWKSet>> {
	if (!mcpKeySet) {
		const jwks = await auth.api.getJwks()
		mcpKeySet = createLocalJWKSet(jwks)
	}

	return mcpKeySet
}

// Server-to-server endpoint called by core to validate an MCP bearer token.
// Signature, issuer, audience and expiry are checked against the local JWKS;
// on top of that the token is only as alive as the consent that produced it,
// so revoking a client from the settings UI cuts off its outstanding access
// tokens immediately. Like the other /internal routes, this is protected by
// the container network plus Caddy's 403 on /auth-realtime/api/internal/*.
app.get("/internal/mcp/session", async (c) => {
	const header = c.req.header("Authorization") ?? ""
	if (!header.toLowerCase().startsWith("bearer ")) {
		return c.json({ error: "missing bearer token" }, 401)
	}

	const token = header.slice("bearer ".length).trim()

	let payload
	try {
		payload = (
			await jwtVerify(token, await mcpTokenKeySet(), {
				issuer: MCP_TOKEN_ISSUER,
				audience: MCP_RESOURCE,
			})
		).payload
	} catch {
		// the cached key set may predate a key rotation; retry once
		// with a fresh set before rejecting.
		mcpKeySet = null

		try {
			payload = (
				await jwtVerify(token, await mcpTokenKeySet(), {
					issuer: MCP_TOKEN_ISSUER,
					audience: MCP_RESOURCE,
				})
			).payload
		} catch {
			return c.json({ error: "invalid token" }, 401)
		}
	}

	const userId = payload.sub
	const clientId =
		typeof payload.azp === "string" ? payload.azp : undefined
	const organizationId =
		typeof payload.org_id === "string" ? payload.org_id : undefined
	const scopes =
		typeof payload.scope === "string"
			? payload.scope.split(" ").filter(Boolean)
			: []

	if (!userId || !clientId || !organizationId) {
		return c.json({ error: "invalid token" }, 401)
	}

	try {
		const consent = await db
			.selectFrom("oauth_consents")
			.where("client_id", "=", clientId)
			.where("fk_user_id", "=", userId)
			.select("client_id")
			.executeTakeFirst()

		if (!consent) {
			return c.json({ error: "consent revoked" }, 401)
		}

		const member = await db
			.selectFrom("organization_members")
			.where("fk_user_id", "=", userId)
			.where("fk_organization_id", "=", organizationId)
			.select("fk_user_id")
			.executeTakeFirst()

		if (!member) {
			return c.json(
				{ error: "not an organization member" },
				401,
			)
		}
	} catch (err) {
		Sentry.captureException(err)
		return c.json({ error: "token validation failed" }, 500)
	}

	return c.json({
		userId,
		organizationId,
		clientId,
		scopes,
		expiresAt: payload.exp,
	})
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
