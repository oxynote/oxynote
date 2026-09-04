import * as Sentry from "@sentry/node"
import { Hono, type Context } from "hono"
import type { ContentfulStatusCode } from "hono/utils/http-status"
import { cors } from "hono/cors"
import { createLocalJWKSet, jwtVerify } from "jose"
import * as Y from "yjs"
import type { Store } from "./db.js"
import type {
	BranchSummary,
	CoreClient,
	HttpResponse,
	MergedDocument,
} from "./core.js"
import type { Env } from "./env.js"
import { authMethods } from "./auth.js"
import { toAxiosHeaders } from "./headers.js"
import { bestEffort } from "./reporting.js"
import { replaceYdocContent, systemOrigin } from "./ydocument.js"
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
	openDirectConnection(
		documentName: string,
		context?: unknown,
	): Promise<DirectConnection>
}

export interface RouteDeps {
	env: Env
	auth: AuthHandler
	store: Store
	hocuspocus: DocumentRegistry
	// runs a document's pending store now; resolves once core has it.
	flushDocument: (documentName: string) => Promise<void>
	// drops every client's socket to a document so each reconnects and
	// authenticates again.
	resetConnections: (documentName: string) => void
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

function readBoolean(source: unknown, key: string): boolean | undefined {
	if (!source || typeof source !== "object") {
		return undefined
	}

	const value = (source as Record<string, unknown>)[key]

	return typeof value === "boolean" ? value : undefined
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
	flushDocument,
	resetConnections,
	core,
}: RouteDeps): Hono {
	const app = new Hono()

	// the names a branch's document can be open under: its id, and for
	// the default branch the alias a client may have connected with.
	function branchDocumentNames(
		documentId: string,
		branchId: string,
		branch: BranchSummary | undefined,
	): string[] {
		const names = [`${documentId}-${branchId}`]

		if (branch?.default) {
			names.push(`${documentId}-default`)
		}

		return names
	}

	// flushBranch stores what the editors hold for a branch before core
	// reads its row. Returns the branch as core lists it, so a caller can
	// tell what a change it is about to make actually changed.
	async function flushBranch(
		documentId: string,
		branchId: string,
	): Promise<BranchSummary | undefined> {
		const branches = await core.fetchBranches(documentId)
		const branch = branches.find((b) => b.branchId === branchId)

		for (const name of branchDocumentNames(
			documentId,
			branchId,
			branch,
		)) {
			await flushDocument(name)
		}

		return branch
	}

	// flushed runs the flushes an operation needs before it may go to
	// core. A failure means core would read a row the editors are ahead
	// of, so the operation is refused instead.
	async function flushed<T>(
		c: Context,
		run: () => Promise<T>,
	): Promise<T | Response> {
		try {
			return await run()
		} catch (error) {
			Sentry.captureException(error)

			return c.json(
				{
					error: "pending changes could not be stored",
				},
				500,
			)
		}
	}

	// calledCore answers with whatever core answered, including a refusal
	// carried on a rejection, so the caller sees core's verdict rather
	// than a generic failure.
	async function calledCore(
		c: Context,
		fallback: string,
		call: () => Promise<HttpResponse>,
	): Promise<HttpResponse | Response> {
		try {
			return await call()
		} catch (error) {
			Sentry.captureException(error)

			const upstream = upstreamResponse(error)
			if (upstream) {
				return c.json(upstream.data, upstream.status)
			}

			return c.json({ error: fallback }, 500)
		}
	}

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

	// the branch operations below are forwarded to core rather than
	// handled here, and auth is delegated with them: the caller's headers
	// (including the session cookie) go along, and core's middleware
	// validates the session and authorizes the change. What this service
	// adds is the flush: core reads the branch's stored row, and the
	// editors are up to ten seconds ahead of it until the debounced store
	// runs.

	// a fork copies the source branch's row.
	app.post("/documents/:documentId/branches", async (c) => {
		const documentId = c.req.param("documentId")

		let body: unknown
		try {
			body = await c.req.json()
		} catch {
			return c.json({ error: "invalid request body" }, 400)
		}

		const sourceBranchId = readString(body, "sourceBranchId")
		if (!sourceBranchId) {
			return c.json(
				{ error: "sourceBranchId is required" },
				400,
			)
		}

		const source = await flushed(c, () =>
			flushBranch(documentId, sourceBranchId),
		)
		if (source instanceof Response) {
			return source
		}

		const response = await calledCore(
			c,
			"failed to create branch",
			() =>
				core.createBranch(
					documentId,
					{ body },
					{
						headers: toAxiosHeaders(
							c.req.raw.headers,
						),
					},
				),
		)
		if (response instanceof Response) {
			return response
		}

		return c.json(
			response.data,
			response.status as ContentfulStatusCode,
		)
	})

	// protecting a branch makes core refuse every store but its own, so
	// the editors' pending one has to land first. Read-only is decided
	// per connection when it authenticates, so a change either way
	// resets the branch's connections: each client reconnects and comes
	// back with the permission the branch now has.
	app.put("/documents/:documentId/branches/:branchId", async (c) => {
		const documentId = c.req.param("documentId")
		const branchId = c.req.param("branchId")

		let body: unknown
		try {
			body = await c.req.json()
		} catch {
			return c.json({ error: "invalid request body" }, 400)
		}

		const before = await flushed(c, () =>
			flushBranch(documentId, branchId),
		)
		if (before instanceof Response) {
			return before
		}

		const response = await calledCore(
			c,
			"failed to update branch",
			() =>
				core.updateBranch(
					documentId,
					branchId,
					{ body },
					{
						headers: toAxiosHeaders(
							c.req.raw.headers,
						),
					},
				),
		)
		if (response instanceof Response) {
			return response
		}

		const protectedFlag = readBoolean(body, "protected")

		if (
			response.status === 200 &&
			protectedFlag !== undefined &&
			protectedFlag !== before?.protected
		) {
			for (const name of branchDocumentNames(
				documentId,
				branchId,
				before,
			)) {
				resetConnections(name)
			}
		}

		return c.json(
			response.data,
			response.status as ContentfulStatusCode,
		)
	})

	// a merge reads the source branch's row and rewrites the target's;
	// both are flushed so neither side is behind its editors. The
	// in-memory Y.Doc mutation below only runs when core responds 200,
	// so an unauthenticated caller changes nothing.
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

		const sides = await flushed(c, async () => {
			await flushBranch(documentId, fromBranchId)
			await flushBranch(documentId, toBranchId)
		})
		if (sides instanceof Response) {
			return sides
		}

		const response = await calledCore(c, "failed to merge", () =>
			core.mergeBranches(
				documentId,
				fromBranchId,
				toBranchId,
				{
					headers: toAxiosHeaders(
						c.req.raw.headers,
					),
				},
			),
		)
		if (response instanceof Response) {
			return response
		}

		if (response.status === 200) {
			await applyMergeToOpenDocument(
				documentId,
				toBranchId,
				response.data as MergedDocument,
			)
		}

		return c.json(
			response.data,
			response.status as ContentfulStatusCode,
		)
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

		// under core's own origin: a merge is core's write, so the
		// persist hocuspocus debounces afterwards is allowed onto a
		// protected target — which is what main is throughout a review.
		toDocument.transact(() => {
			replaceYdocContent(toDocument, {
				name: merged.documentName,
				content: merged.content,
				icon: merged.icon,
			})
		}, systemOrigin)

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

			// core marks the batches it originates itself. Those
			// persist straight away and are allowed onto a protected
			// branch; the debounced store that follows an edit is not,
			// which is what keeps an editor — or the assistant — from
			// writing to one through this endpoint.
			const system =
				body && typeof body === "object"
					? (body as { system?: unknown })
							.system === true
					: false

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
						system
							? systemOrigin.context
							: {},
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
