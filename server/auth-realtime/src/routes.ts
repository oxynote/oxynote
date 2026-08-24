import * as Sentry from "@sentry/node"
import { Hono } from "hono"
import type { ContentfulStatusCode } from "hono/utils/http-status"
import { cors } from "hono/cors"
import { createLocalJWKSet, jwtVerify } from "jose"
import * as Y from "yjs"
import type { Store } from "./db.js"
import type { CoreClient, MergedDocument } from "./core.js"
import type { Env } from "./env.js"
import { authMethods } from "./auth.js"
import { toAxiosHeaders } from "./headers.js"
import { bestEffort } from "./reporting.js"
import { replaceYdocContent } from "./ydocument.js"
import { applyOperations, type Operation } from "./operations.js"

// the two things the routes ask of better-auth: the request handler
// mounted under /auth/**, and the local JWKS the MCP session endpoint
// verifies bearer tokens against.
export interface AuthHandler {
	handler(request: Request): Promise<Response>
	api: {
		getJwks(): Promise<Parameters<typeof createLocalJWKSet>[0]>
	}
}

export interface DirectConnection {
	transact(transaction: (doc: Y.Doc) => void): Promise<void>
	disconnect(): void | Promise<void>
}

// the part of the hocuspocus server the routes reach into: the live
// in-memory documents, and the direct connections the operations endpoint
// opens against them.
export interface DocumentRegistry {
	documents: { get(documentName: string): Y.Doc | undefined }
	openDirectConnection(documentName: string): Promise<DirectConnection>
}

export interface RouteDeps {
	env: Env
	auth: AuthHandler
	store: Store
	hocuspocus: DocumentRegistry
	core: CoreClient
}

// request bodies arrive as unvalidated JSON, so every field a route reads
// is taken through a narrowing read rather than trusted from a type
// annotation.
function readString(source: unknown, key: string): string | undefined {
	if (!source || typeof source !== "object") {
		return undefined
	}

	const value = (source as Record<string, unknown>)[key]

	return typeof value === "string" ? value : undefined
}

// an axios rejection carrying core's own response, which the merge route
// forwards verbatim so the caller sees core's status and error body rather
// than a generic 500.
function upstreamResponse(
	error: unknown,
): { status: ContentfulStatusCode; data: any } | undefined {
	if (!error || typeof error !== "object") {
		return undefined
	}

	const response = (error as { response?: unknown }).response
	if (!response || typeof response !== "object") {
		return undefined
	}

	const { status, data } = response as {
		status?: unknown
		data?: unknown
	}
	if (typeof status !== "number") {
		return undefined
	}

	return { status: status as ContentfulStatusCode, data }
}

export function createRoutes({
	env,
	auth,
	store,
	hocuspocus,
	core,
}: RouteDeps): Hono {
	const app = new Hono()

	app.use(
		"*",
		cors({
			origin: env.trustedOrigins,
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
	// providers are configured (and, later, other signup-related
	// capability flags). Lives outside the better-auth /auth/**
	// namespace.
	app.get("/auth-config", (c) => {
		return c.json({ methods: authMethods(env) })
	})

	app.get("/organizations/stats", async (c) => {
		const count = await store.totalOrganizationCount()
		return c.json({ availableSlots: env.maxOrganizations - count })
	})

	// auth is delegated: the caller's headers (including the session
	// cookie) are forwarded to core, whose middleware validates the
	// session and authorizes the merge. The in-memory Y.Doc mutation
	// below only runs when core responds 200, so an unauthenticated
	// caller changes nothing.
	app.put("/documents/:documentId/merge", async (c) => {
		const documentId = c.req.param("documentId")

		let body: unknown
		try {
			body = await c.req.json()
		} catch {
			return c.json({ error: "invalid request body" }, 400)
		}

		const fromBranchId = readString(body, "fromBranchId")
		const toBranchId = readString(body, "toBranchId")

		if (!fromBranchId || !toBranchId) {
			return c.json(
				{
					error: "fromBranchId and toBranchId are required",
				},
				400,
			)
		}

		try {
			const response = await core.mergeBranches(
				documentId,
				fromBranchId,
				toBranchId,
				{ headers: toAxiosHeaders(c.req.raw.headers) },
			)

			if (response.status === 200) {
				await applyMergeToOpenDocument(
					documentId,
					toBranchId,
					response.data,
				)
			}

			return c.json(
				response.data,
				response.status as ContentfulStatusCode,
			)
		} catch (error) {
			Sentry.captureException(error)

			const upstream = upstreamResponse(error)
			if (upstream) {
				return c.json(upstream.data, upstream.status)
			}

			return c.json({ error: "failed to merge" }, 500)
		}
	})

	// mirrors the merge into the target branch's live Y.Doc, if one is
	// open. Direct cloning rather than Y.applyUpdate: source and target
	// docs have different client origins, and merging them CRDT-style
	// duplicates the content instead of replacing it.
	async function applyMergeToOpenDocument(
		documentId: string,
		toBranchId: string,
		merged: MergedDocument,
	): Promise<void> {
		const toDocument = hocuspocus.documents.get(
			`${documentId}-${toBranchId}`,
		)
		if (!toDocument) {
			return
		}

		replaceYdocContent(toDocument, {
			name: merged.documentName,
			content: merged.content,
			icon: merged.icon,
		})

		// persist rawContent immediately because MergeBranches sets
		// rawContent to nil in the database. Without this, a server
		// restart before onStoreDocument would trigger
		// onLoadDocument's first-time path, creating a new Y.Doc with
		// a different clientID that CRDT-merges with reconnecting
		// clients and duplicates content.
		await bestEffort(() =>
			core.storeBranchContent(documentId, toBranchId, {
				name: merged.documentName,
				icon: merged.icon,
				content: merged.content,
				maintainers: [],
				rawContent: Buffer.from(
					Y.encodeStateAsUpdate(toDocument),
				).toString("base64"),
				system: true,
			}),
		)
	}

	// cached JWKS for MCP access-token verification. Cleared on a failed
	// verification so a key rotation is picked up on the next request.
	let mcpKeySet: ReturnType<typeof createLocalJWKSet> | null = null

	async function mcpTokenKeySet(): Promise<
		ReturnType<typeof createLocalJWKSet>
	> {
		mcpKeySet ??= createLocalJWKSet(await auth.api.getJwks())

		return mcpKeySet
	}

	async function verifyMCPToken(token: string) {
		const options = {
			issuer: env.mcpTokenIssuer,
			audience: env.mcpResource,
		}

		try {
			return (
				await jwtVerify(
					token,
					await mcpTokenKeySet(),
					options,
				)
			).payload
		} catch {
			// the cached key set may predate a key rotation; retry
			// once with a fresh set before rejecting.
			mcpKeySet = null

			return (
				await jwtVerify(
					token,
					await mcpTokenKeySet(),
					options,
				)
			).payload
		}
	}

	// Server-to-server endpoint called by core to validate an MCP bearer
	// token. Signature, issuer, audience and expiry are checked against
	// the local JWKS; on top of that the token is only as alive as the
	// consent that produced it, so revoking a client from the settings UI
	// cuts off its outstanding access tokens immediately. Like the other
	// /internal routes, this is protected by the container network plus
	// Caddy's 403 on /auth-realtime/api/internal/*.
	app.get("/internal/mcp/session", async (c) => {
		const header = c.req.header("Authorization") ?? ""
		if (!header.toLowerCase().startsWith("bearer ")) {
			return c.json({ error: "missing bearer token" }, 401)
		}

		const token = header.slice("bearer ".length).trim()

		let payload
		try {
			payload = await verifyMCPToken(token)
		} catch {
			return c.json({ error: "invalid token" }, 401)
		}

		const userId = payload.sub
		const clientId =
			typeof payload.azp === "string"
				? payload.azp
				: undefined
		const organizationId =
			typeof payload.org_id === "string"
				? payload.org_id
				: undefined
		const scopes =
			typeof payload.scope === "string"
				? payload.scope.split(" ").filter(Boolean)
				: []

		if (!userId || !clientId || !organizationId) {
			return c.json({ error: "invalid token" }, 401)
		}

		try {
			if (!(await store.hasOAuthConsent(clientId, userId))) {
				return c.json({ error: "consent revoked" }, 401)
			}

			if (
				!(await store.isOrganizationMember(
					userId,
					organizationId,
				))
			) {
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

			let body: unknown
			try {
				body = await c.req.json()
			} catch {
				return c.json(
					{ error: "invalid request body" },
					400,
				)
			}

			const operations =
				body && typeof body === "object"
					? (body as { operations?: unknown })
							.operations
					: undefined

			if (!Array.isArray(operations)) {
				return c.json(
					{
						error: "operations array is required",
					},
					400,
				)
			}

			// each op is validated as it is applied — a malformed
			// one lands in the batch's errors list rather than
			// failing the request.
			const ops = operations as Operation[]

			// Hocuspocus addresses documents by
			// `documentId-branchId`; the Go caller resolves
			// "default" to a concrete branchId before sending so
			// this path can be a direct lookup.
			const documentName = `${documentId}-${branchId}`

			let connection
			try {
				connection =
					await hocuspocus.openDirectConnection(
						documentName,
					)
			} catch (err) {
				Sentry.captureException(err)
				return c.json(
					{ error: "failed to open document" },
					500,
				)
			}

			try {
				let result: {
					applied: number
					errors: {
						index: number
						message: string
					}[]
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
				await bestEffort(() => connection.disconnect())
			}
		},
	)

	return app
}
