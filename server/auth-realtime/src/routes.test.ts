import { describe, it, vi, type Mock } from "vitest"
import * as Y from "yjs"
import { SignJWT, exportJWK, generateKeyPair, type JWTPayload } from "jose"
import { createRoutes, type DocumentRegistry } from "./routes.js"
import { isSystemContext, replaceYdocContent } from "./ydocument.js"
import {
	fragmentXml,
	stubCore,
	stubStore,
	testEnv,
	type StubCore,
	type StubStore,
} from "./test-helpers.js"

const ENV = testEnv()

function stubAuth() {
	return {
		handler: vi
			.fn()
			.mockResolvedValue(
				new Response("handled by better-auth"),
			),
		api: { getJwks: vi.fn().mockResolvedValue({ keys: [] }) },
	}
}

function stubRegistry(
	overrides: Partial<{
		documents: Map<string, Y.Doc>
		transact: (fn: (doc: Y.Doc) => void) => Promise<void>
		open: () => Promise<never>
	}> = {},
) {
	const documents = overrides.documents ?? new Map<string, Y.Doc>()
	const disconnect = vi.fn()
	const transact = vi.fn(
		overrides.transact ??
			((fn: (doc: Y.Doc) => void) => {
				fn(new Y.Doc())

				return Promise.resolve()
			}),
	)
	const openDirectConnection = vi.fn(
		overrides.open ??
			(() => Promise.resolve({ transact, disconnect })),
	)

	return {
		registry: {
			documents,
			openDirectConnection,
		} as unknown as DocumentRegistry,
		openDirectConnection,
		transact,
		disconnect,
	}
}

function build(
	overrides: {
		env?: typeof ENV
		auth?: ReturnType<typeof stubAuth>
		store?: StubStore
		hocuspocus?: DocumentRegistry
		flushDocument?: Mock<(documentName: string) => Promise<void>>
		core?: StubCore
	} = {},
) {
	const auth = overrides.auth ?? stubAuth()
	const core = overrides.core ?? stubCore()
	const store = overrides.store ?? stubStore()
	const flushDocument: Mock<(documentName: string) => Promise<void>> =
		overrides.flushDocument ?? vi.fn().mockResolvedValue(undefined)
	const resetConnections = vi.fn<(documentName: string) => void>()
	const app = createRoutes({
		env: overrides.env ?? ENV,
		auth,
		store,
		hocuspocus: overrides.hocuspocus ?? stubRegistry().registry,
		flushDocument,
		resetConnections,
		core,
	})

	return { app, auth, core, store, flushDocument, resetConnections }
}

// the order in which the flushes and the core call happened, which is
// what the branch routes are about: the flush has to come first.
function callOrder(flushDocument: Mock, coreCall: Mock): string[] {
	return [
		...flushDocument.mock.invocationCallOrder.map(
			(order, i) =>
				`${order}:flush ${String(flushDocument.mock.calls[i]?.[0])}`,
		),
		...coreCall.mock.invocationCallOrder.map(
			(order) => `${order}:core`,
		),
	]
		.sort(
			(a, b) =>
				Number(a.split(":")[0]) -
				Number(b.split(":")[0]),
		)
		.map((entry) => entry.slice(entry.indexOf(":") + 1))
}

function json(body: unknown, cookie?: string): RequestInit {
	return {
		method: "PUT",
		body: JSON.stringify(body),
		headers: {
			"Content-Type": "application/json",
			...(cookie ? { Cookie: cookie } : {}),
		},
	}
}

function seededDocument(text: string): Y.Doc {
	const doc = new Y.Doc()
	replaceYdocContent(doc, {
		name: "Runbook",
		content: {
			type: "doc",
			content: [
				{
					type: "paragraph",
					attrs: { uid: "p1" },
					content: [{ type: "text", text }],
				},
			],
		},
		icon: "lucide:file",
	})

	return doc
}

// a real key pair, so the mcp session route runs its actual signature,
// issuer and audience checks rather than a stubbed verifier
async function mcpKeys() {
	const { privateKey, publicKey } = await generateKeyPair("ES256", {
		extractable: true,
	})
	const jwk = await exportJWK(publicKey)

	return {
		privateKey,
		jwks: { keys: [{ ...jwk, kid: "test-key", alg: "ES256" }] },
	}
}

async function mcpToken(
	privateKey: CryptoKey,
	claims: JWTPayload = {},
	overrides: { issuer?: string; audience?: string } = {},
) {
	return new SignJWT({
		azp: "client-1",
		org_id: "org-1",
		scope: "documents:read documents:write",
		...claims,
	})
		.setProtectedHeader({ alg: "ES256", kid: "test-key" })
		.setSubject(claims.sub ?? "user-1")
		.setIssuer(overrides.issuer ?? ENV.mcpTokenIssuer)
		.setAudience(overrides.audience ?? ENV.mcpResource)
		.setExpirationTime("1h")
		.sign(privateKey)
}

// a store answering for a user whose consent still stands but who has
// left the organization the token names
function memberlessStore() {
	const store = stubStore()
	store.isOrganizationMember.mockResolvedValue(false)

	return store
}

function bearer(token: string) {
	return { headers: { Authorization: `Bearer ${token}` } }
}

describe("createRoutes", () => {
	describe("GET /", () => {
		it("answers so a health check can see the service is up", async ({
			expect,
		}) => {
			const { app } = build()

			const res = await app.request("/")

			expect(res.status).toBe(200)
			expect(await res.text()).toBe("Hello Hono!")
		})
	})

	describe("GET /auth-config", () => {
		it("publishes the sign-in methods the service actually accepts", async ({
			expect,
		}) => {
			const { app } = build({
				env: testEnv({
					socialProviders: {
						github: {
							clientId: "id",
							clientSecret: "secret",
						},
					},
				}),
			})

			const res = await app.request("/auth-config")

			expect(res.status).toBe(200)
			expect(await res.json()).toEqual({
				methods: ["email-password", "github"],
			})
		})
	})

	describe("/auth/**", () => {
		it("hands the raw request to better-auth", async ({
			expect,
		}) => {
			const { app, auth } = build()

			const res = await app.request("/auth/get-session")

			expect(res.status).toBe(200)
			expect(await res.text()).toBe("handled by better-auth")
			expect(auth.handler).toHaveBeenCalledTimes(1)
		})
	})

	describe("GET /organizations/stats", () => {
		it("reports the slots left under the organization limit", async ({
			expect,
		}) => {
			const store = stubStore()
			store.totalOrganizationCount.mockResolvedValue(40)
			const { app } = build({
				store,
				env: testEnv({ maxOrganizations: 50 }),
			})

			const res = await app.request("/organizations/stats")

			expect(await res.json()).toEqual({ availableSlots: 10 })
		})
	})

	describe("POST /documents/:documentId/branches", () => {
		it.for([
			{ name: "a body that is not JSON", input: "not json" },
			{
				name: "a body missing sourceBranchId",
				input: '{"branch":"draft"}',
			},
			{
				name: "a sourceBranchId that is not a string",
				input: '{"branch":"draft","sourceBranchId":1}',
			},
		])("rejects $name", async ({ input }, { expect }) => {
			const { app, core, flushDocument } = build()

			const res = await app.request(
				"/documents/doc1/branches",
				{
					...json(null),
					method: "POST",
					body: input,
				},
			)

			expect(res.status).toBe(400)
			expect(flushDocument).toHaveBeenCalledTimes(0)
			expect(core.createBranch).toHaveBeenCalledTimes(0)
		})

		it("flushes the source branch before core copies it", async ({
			expect,
		}) => {
			const { app, core, flushDocument } = build()

			const res = await app.request(
				"/documents/doc1/branches",
				{
					...json(
						{
							branch: "draft",
							sourceBranchId: "b1",
						},
						"auth.session=abc",
					),
					method: "POST",
				},
			)

			expect(res.status).toBe(200)
			expect(await res.json()).toEqual({
				branchId: "branch-2",
			})
			expect(
				callOrder(flushDocument, core.createBranch),
			).toEqual(["flush doc1-b1", "core"])
			const [documentId, request, options] =
				core.createBranch.mock.calls[0] ?? []
			expect(documentId).toBe("doc1")
			expect(request?.body).toEqual({
				branch: "draft",
				sourceBranchId: "b1",
			})
			expect(options?.headers?.get("cookie")).toBe(
				"auth.session=abc",
			)
		})

		// the default branch can be open under its id and under the
		// "default" alias; a store pending under either is the same edits
		it("also flushes the default alias when forking the default branch", async ({
			expect,
		}) => {
			const core = stubCore()
			core.fetchBranches.mockResolvedValue([
				{
					branchId: "b1",
					default: true,
					protected: false,
				},
			])
			const { app, flushDocument } = build({ core })

			await app.request("/documents/doc1/branches", {
				...json({
					branch: "draft",
					sourceBranchId: "b1",
				}),
				method: "POST",
			})

			expect(
				flushDocument.mock.calls.map((call) => call[0]),
			).toEqual(["doc1-b1", "doc1-default"])
		})

		it("refuses the fork when the flush fails", async ({
			expect,
		}) => {
			const flushDocument = vi
				.fn()
				.mockRejectedValue(
					new Error("core unreachable"),
				)
			const { app, core } = build({ flushDocument })

			const res = await app.request(
				"/documents/doc1/branches",
				{
					...json({
						branch: "draft",
						sourceBranchId: "b1",
					}),
					method: "POST",
				},
			)

			expect(res.status).toBe(500)
			expect(await res.json()).toEqual({
				error: "pending changes could not be stored",
			})
			expect(core.createBranch).toHaveBeenCalledTimes(0)
		})

		it("answers with core's refusal", async ({ expect }) => {
			const core = stubCore()
			core.createBranch.mockRejectedValue({
				response: {
					status: 409,
					data: { error: "branch exists" },
				},
			})
			const { app } = build({ core })

			const res = await app.request(
				"/documents/doc1/branches",
				{
					...json({
						branch: "draft",
						sourceBranchId: "b1",
					}),
					method: "POST",
				},
			)

			expect(res.status).toBe(409)
			expect(await res.json()).toEqual({
				error: "branch exists",
			})
		})
	})

	describe("PUT /documents/:documentId/branches/:branchId", () => {
		it("rejects a body that is not JSON", async ({ expect }) => {
			const { app, core, flushDocument } = build()

			const res = await app.request(
				"/documents/doc1/branches/b1",
				{
					...json(null),
					body: "not json",
				},
			)

			expect(res.status).toBe(400)
			expect(flushDocument).toHaveBeenCalledTimes(0)
			expect(core.updateBranch).toHaveBeenCalledTimes(0)
		})

		// the store the editors have pending is a user write, and core
		// refuses those once the branch is protected: it has to land
		// before the flag is set
		it("flushes the branch before core changes it", async ({
			expect,
		}) => {
			const { app, core, flushDocument } = build()

			const res = await app.request(
				"/documents/doc1/branches/b1",
				json({ protected: true }, "auth.session=abc"),
			)

			expect(res.status).toBe(200)
			expect(
				callOrder(flushDocument, core.updateBranch),
			).toEqual(["flush doc1-b1", "core"])
			const [documentId, branchId, request, options] =
				core.updateBranch.mock.calls[0] ?? []
			expect(documentId).toBe("doc1")
			expect(branchId).toBe("b1")
			expect(request?.body).toEqual({ protected: true })
			expect(options?.headers?.get("cookie")).toBe(
				"auth.session=abc",
			)
		})

		it.for([
			{
				name: "protecting",
				before: false,
				body: { protected: true },
			},
			{
				name: "unprotecting",
				before: true,
				body: { protected: false },
			},
		])(
			"resets the branch's connections when $name it",
			async ({ before, body }, { expect }) => {
				const core = stubCore()
				core.fetchBranches.mockResolvedValue([
					{
						branchId: "b1",
						default: true,
						protected: before,
					},
				])
				const { app, resetConnections } = build({
					core,
				})

				await app.request(
					"/documents/doc1/branches/b1",
					json(body),
				)

				expect(
					resetConnections.mock.calls.map(
						(call) => call[0],
					),
				).toEqual(["doc1-b1", "doc1-default"])
			},
		)

		it.for([
			{
				name: "a rename",
				body: { name: "release" },
			},
			{
				name: "a protection the branch already has",
				body: { protected: false },
			},
		])(
			"keeps the connections on $name",
			async ({ body }, { expect }) => {
				const core = stubCore()
				core.fetchBranches.mockResolvedValue([
					{
						branchId: "b1",
						default: false,
						protected: false,
					},
				])
				const { app, resetConnections } = build({
					core,
				})

				const res = await app.request(
					"/documents/doc1/branches/b1",
					json(body),
				)

				expect(res.status).toBe(200)
				expect(resetConnections).toHaveBeenCalledTimes(
					0,
				)
			},
		)

		it("keeps the connections when core refuses the change", async ({
			expect,
		}) => {
			const core = stubCore()
			core.updateBranch.mockResolvedValue({
				status: 403,
				data: { error: "not permitted" },
			})
			const { app, resetConnections } = build({ core })

			const res = await app.request(
				"/documents/doc1/branches/b1",
				json({ protected: true }),
			)

			expect(res.status).toBe(403)
			expect(resetConnections).toHaveBeenCalledTimes(0)
		})

		it("refuses the change when the flush fails", async ({
			expect,
		}) => {
			const flushDocument = vi
				.fn()
				.mockRejectedValue(
					new Error("core unreachable"),
				)
			const { app, core, resetConnections } = build({
				flushDocument,
			})

			const res = await app.request(
				"/documents/doc1/branches/b1",
				json({ protected: true }),
			)

			expect(res.status).toBe(500)
			expect(core.updateBranch).toHaveBeenCalledTimes(0)
			expect(resetConnections).toHaveBeenCalledTimes(0)
		})

		it("keeps the connections when core is unreachable", async ({
			expect,
		}) => {
			const core = stubCore()
			core.updateBranch.mockRejectedValue(
				new Error("connect ECONNREFUSED"),
			)
			const { app, resetConnections } = build({ core })

			const res = await app.request(
				"/documents/doc1/branches/b1",
				json({ protected: true }),
			)

			expect(res.status).toBe(500)
			expect(await res.json()).toEqual({
				error: "failed to update branch",
			})
			expect(resetConnections).toHaveBeenCalledTimes(0)
		})
	})

	describe("DELETE /documents/:documentId/branches/:branchId", () => {
		it("forwards the deletion to core with the caller's headers, without a flush", async ({
			expect,
		}) => {
			const { app, core, flushDocument } = build()

			const res = await app.request(
				"/documents/doc1/branches/b1",
				{
					method: "DELETE",
					headers: { Cookie: "auth.session=abc" },
				},
			)

			expect(res.status).toBe(204)
			expect(flushDocument).toHaveBeenCalledTimes(0)
			const [documentId, branchId, options] =
				core.deleteBranch.mock.calls[0] ?? []
			expect(documentId).toBe("doc1")
			expect(branchId).toBe("b1")
			expect(options?.headers?.get("cookie")).toBe(
				"auth.session=abc",
			)
		})

		it("drops the connections still on the deleted branch", async ({
			expect,
		}) => {
			const { app, resetConnections } = build()

			await app.request("/documents/doc1/branches/b1", {
				method: "DELETE",
			})

			expect(
				resetConnections.mock.calls.map(
					(call) => call[0],
				),
			).toEqual(["doc1-b1"])
		})

		it("keeps the connections when core refuses the deletion", async ({
			expect,
		}) => {
			const core = stubCore()
			core.deleteBranch.mockResolvedValue({
				status: 409,
				data: { code: "document.last_branch" },
			})
			const { app, resetConnections } = build({ core })

			const res = await app.request(
				"/documents/doc1/branches/b1",
				{ method: "DELETE" },
			)

			expect(res.status).toBe(409)
			expect(await res.json()).toEqual({
				code: "document.last_branch",
			})
			expect(resetConnections).toHaveBeenCalledTimes(0)
		})

		it("keeps the connections when core is unreachable", async ({
			expect,
		}) => {
			const core = stubCore()
			core.deleteBranch.mockRejectedValue(
				new Error("connect ECONNREFUSED"),
			)
			const { app, resetConnections } = build({ core })

			const res = await app.request(
				"/documents/doc1/branches/b1",
				{ method: "DELETE" },
			)

			expect(res.status).toBe(500)
			expect(await res.json()).toEqual({
				error: "failed to delete branch",
			})
			expect(resetConnections).toHaveBeenCalledTimes(0)
		})
	})

	describe("PUT /documents/:documentId/merge", () => {
		it("flushes the source and then the target before core merges", async ({
			expect,
		}) => {
			const { app, core, flushDocument } = build()

			await app.request(
				"/documents/doc1/merge",
				json({ fromBranchId: "b2", toBranchId: "b1" }),
			)

			expect(
				callOrder(flushDocument, core.mergeBranches),
			).toEqual(["flush doc1-b2", "flush doc1-b1", "core"])
		})

		it("refuses the merge when a flush fails", async ({
			expect,
		}) => {
			const flushDocument = vi
				.fn()
				.mockRejectedValue(
					new Error("core unreachable"),
				)
			const { app, core } = build({ flushDocument })

			const res = await app.request(
				"/documents/doc1/merge",
				json({ fromBranchId: "b2", toBranchId: "b1" }),
			)

			expect(res.status).toBe(500)
			expect(core.mergeBranches).toHaveBeenCalledTimes(0)
		})

		it.for([
			{ name: "a body that is not JSON", input: "not json" },
			{ name: "a body that is not an object", input: "null" },
			{
				name: "a body missing fromBranchId",
				input: '{"toBranchId":"b1"}',
			},
			{
				name: "a body missing toBranchId",
				input: '{"fromBranchId":"b2"}',
			},
			{
				name: "branch ids that are not strings",
				input: '{"fromBranchId":1,"toBranchId":2}',
			},
		])("rejects $name", async ({ input }, { expect }) => {
			const { app, core } = build()

			const res = await app.request("/documents/doc1/merge", {
				method: "PUT",
				body: input,
				headers: { "Content-Type": "application/json" },
			})

			expect(res.status).toBe(400)
			expect(core.mergeBranches).toHaveBeenCalledTimes(0)
		})

		// core validates the session and authorizes the merge; this
		// service only mirrors the result into the live document
		it("forwards the caller's headers to core", async ({
			expect,
		}) => {
			const { app, core } = build()

			await app.request("/documents/doc1/merge", {
				method: "PUT",
				body: JSON.stringify({
					fromBranchId: "b2",
					toBranchId: "b1",
				}),
				headers: {
					"Content-Type": "application/json",
					Cookie: "auth.session=abc",
				},
			})

			expect(core.mergeBranches).toHaveBeenCalledTimes(1)
			const [documentId, from, to, options] =
				core.mergeBranches.mock.calls[0] ?? []
			expect(documentId).toBe("doc1")
			expect(from).toBe("b2")
			expect(to).toBe("b1")
			expect(options?.headers?.get("cookie")).toBe(
				"auth.session=abc",
			)
		})

		it("replaces the open target document with the merged content", async ({
			expect,
		}) => {
			const target = seededDocument("Before the merge")
			const { registry } = stubRegistry({
				documents: new Map([["doc1-b1", target]]),
			})
			const core = stubCore()
			core.mergeBranches.mockResolvedValue({
				status: 200,
				data: {
					documentName: "Runbook",
					content: {
						type: "doc",
						content: [
							{
								type: "paragraph",
								attrs: {
									uid: "p9",
								},
								content: [
									{
										type: "text",
										text: "After the merge",
									},
								],
							},
						],
					},
					icon: "lucide:file",
				},
			})
			const { app } = build({ hocuspocus: registry, core })

			await app.request("/documents/doc1/merge", {
				method: "PUT",
				body: JSON.stringify({
					fromBranchId: "b2",
					toBranchId: "b1",
				}),
				headers: { "Content-Type": "application/json" },
			})

			const content = fragmentXml(target, "content")
			expect(content).toContain("After the merge")
			expect(content).not.toContain("Before the merge")
		})

		// main is protected throughout a review, so the persist that
		// follows the mirrored merge has to be recognisable as core's
		it("mirrors the merge under core's own origin", async ({
			expect,
		}) => {
			const target = seededDocument("Before the merge")
			const { registry } = stubRegistry({
				documents: new Map([["doc1-b1", target]]),
			})
			const core = stubCore()
			core.mergeBranches.mockResolvedValue({
				status: 200,
				data: {
					documentName: "Runbook",
					content: { type: "doc", content: [] },
					icon: "lucide:file",
				},
			})
			const { app } = build({ hocuspocus: registry, core })

			const origins: unknown[] = []
			target.on(
				"update",
				(_update: Uint8Array, origin: unknown) => {
					origins.push(origin)
				},
			)

			await app.request("/documents/doc1/merge", {
				method: "PUT",
				body: JSON.stringify({
					fromBranchId: "b2",
					toBranchId: "b1",
				}),
				headers: { "Content-Type": "application/json" },
			})

			expect(origins).toHaveLength(1)
			expect(
				isSystemContext(
					(origins[0] as { context: unknown })
						.context,
				),
			).toBe(true)
		})

		// core sets rawContent to nil when it merges, so a restart
		// before the next store would rebuild the doc under a new
		// clientID and duplicate every block on reconnect
		it("persists the merged state immediately as a system write", async ({
			expect,
		}) => {
			const target = seededDocument("Before the merge")
			const { registry } = stubRegistry({
				documents: new Map([["doc1-b1", target]]),
			})
			const { app, core } = build({ hocuspocus: registry })

			await app.request("/documents/doc1/merge", {
				method: "PUT",
				body: JSON.stringify({
					fromBranchId: "b2",
					toBranchId: "b1",
				}),
				headers: { "Content-Type": "application/json" },
			})

			expect(core.storeBranchContent).toHaveBeenCalledTimes(1)
			const [documentId, branchId, update] =
				core.storeBranchContent.mock.calls[0] ?? []
			expect(documentId).toBe("doc1")
			expect(branchId).toBe("b1")
			expect(update?.system).toBe(true)
			expect(update?.maintainers).toEqual([])
		})

		it("leaves nothing to mirror when the branch has no open document", async ({
			expect,
		}) => {
			const { app, core } = build()

			const res = await app.request("/documents/doc1/merge", {
				method: "PUT",
				body: JSON.stringify({
					fromBranchId: "b2",
					toBranchId: "b1",
				}),
				headers: { "Content-Type": "application/json" },
			})

			expect(res.status).toBe(200)
			expect(core.storeBranchContent).toHaveBeenCalledTimes(0)
		})

		it("does not touch the open document when core refuses the merge", async ({
			expect,
		}) => {
			const target = seededDocument("Before the merge")
			const { registry } = stubRegistry({
				documents: new Map([["doc1-b1", target]]),
			})
			const core = stubCore()
			core.mergeBranches.mockResolvedValue({
				status: 409,
				data: {
					documentName: "",
					content: null,
					icon: "",
				},
			})
			const { app } = build({ hocuspocus: registry, core })

			const res = await app.request("/documents/doc1/merge", {
				method: "PUT",
				body: JSON.stringify({
					fromBranchId: "b2",
					toBranchId: "b1",
				}),
				headers: { "Content-Type": "application/json" },
			})

			expect(res.status).toBe(409)
			expect(fragmentXml(target, "content")).toContain(
				"Before the merge",
			)
			expect(core.storeBranchContent).toHaveBeenCalledTimes(0)
		})

		it("answers 200 even when persisting the merged state fails", async ({
			expect,
		}) => {
			const target = seededDocument("Before the merge")
			const { registry } = stubRegistry({
				documents: new Map([["doc1-b1", target]]),
			})
			const core = stubCore()
			core.storeBranchContent.mockRejectedValue(
				new Error("core unreachable"),
			)
			const { app } = build({ hocuspocus: registry, core })

			const res = await app.request("/documents/doc1/merge", {
				method: "PUT",
				body: JSON.stringify({
					fromBranchId: "b2",
					toBranchId: "b1",
				}),
				headers: { "Content-Type": "application/json" },
			})

			expect(res.status).toBe(200)
		})

		// the caller gets core's own answer rather than a generic 500,
		// so a rejected merge reads the same through the proxy as it
		// would directly
		it("forwards core's status and body when core rejects the merge", async ({
			expect,
		}) => {
			const core = stubCore()
			core.mergeBranches.mockRejectedValue({
				response: {
					status: 403,
					data: { error: "not a maintainer" },
				},
			})
			const { app } = build({ core })

			const res = await app.request("/documents/doc1/merge", {
				method: "PUT",
				body: JSON.stringify({
					fromBranchId: "b2",
					toBranchId: "b1",
				}),
				headers: { "Content-Type": "application/json" },
			})

			expect(res.status).toBe(403)
			expect(await res.json()).toEqual({
				error: "not a maintainer",
			})
		})

		it.for([
			{
				name: "a plain transport failure",
				input: new Error("connect ECONNREFUSED"),
			},
			{
				name: "a rejection carrying no response",
				input: { message: "boom" },
			},
			{
				name: "a response with no usable status",
				input: {
					response: { status: "403", data: {} },
				},
			},
			{
				name: "a rejection that is not an object",
				input: "boom",
			},
		])("answers 500 for $name", async ({ input }, { expect }) => {
			const core = stubCore()
			core.mergeBranches.mockRejectedValue(input)
			const { app } = build({ core })

			const res = await app.request("/documents/doc1/merge", {
				method: "PUT",
				body: JSON.stringify({
					fromBranchId: "b2",
					toBranchId: "b1",
				}),
				headers: {
					"Content-Type": "application/json",
				},
			})

			expect(res.status).toBe(500)
			expect(await res.json()).toEqual({
				error: "failed to merge",
			})
		})
	})

	describe("GET /internal/mcp/session", () => {
		it.for([
			{ name: "no Authorization header", input: undefined },
			{ name: "a non-bearer scheme", input: "Basic abc" },
			{ name: "an empty bearer", input: "Bearer" },
		])("rejects $name", async ({ input }, { expect }) => {
			const { app } = build()

			const res = await app.request(
				"/internal/mcp/session",
				input
					? { headers: { Authorization: input } }
					: {},
			)

			expect(res.status).toBe(401)
			expect(await res.json()).toEqual({
				error: "missing bearer token",
			})
		})

		it("rejects a token that is not a signed JWT", async ({
			expect,
		}) => {
			const { app } = build()

			const res = await app.request(
				"/internal/mcp/session",
				bearer("not-a-jwt"),
			)

			expect(res.status).toBe(401)
			expect(await res.json()).toEqual({
				error: "invalid token",
			})
		})

		it.for([
			{ name: "another issuer", input: "issuer" as const },
			{
				name: "another audience",
				input: "audience" as const,
			},
		])(
			"rejects a token minted for $name",
			async ({ input }, { expect }) => {
				const { privateKey, jwks } = await mcpKeys()
				const auth = stubAuth()
				auth.api.getJwks.mockResolvedValue(jwks)
				const { app } = build({ auth })
				const token = await mcpToken(
					privateKey,
					{},
					{ [input]: "http://evil.example" },
				)

				const res = await app.request(
					"/internal/mcp/session",
					bearer(token),
				)

				expect(res.status).toBe(401)
			},
		)

		it.for([
			{ name: "no subject", input: { sub: "" } },
			{ name: "no client", input: { azp: undefined } },
			{
				name: "no organization",
				input: { org_id: undefined },
			},
		])(
			"rejects a token with $name",
			async ({ input }, { expect }) => {
				const { privateKey, jwks } = await mcpKeys()
				const auth = stubAuth()
				auth.api.getJwks.mockResolvedValue(jwks)
				const { app } = build({
					auth,
				})
				const token = await mcpToken(privateKey, input)

				const res = await app.request(
					"/internal/mcp/session",
					bearer(token),
				)

				expect(res.status).toBe(401)
				expect(await res.json()).toEqual({
					error: "invalid token",
				})
			},
		)

		// revoking a client from the settings UI has to cut off the
		// access tokens it already holds, which is why the consent row
		// is checked on every call rather than only at issuance
		it("rejects a token whose consent has been revoked", async ({
			expect,
		}) => {
			const { privateKey, jwks } = await mcpKeys()
			const auth = stubAuth()
			auth.api.getJwks.mockResolvedValue(jwks)
			const store = stubStore()
			store.hasOAuthConsent.mockResolvedValue(false)
			const { app } = build({ auth, store })
			const token = await mcpToken(privateKey)

			const res = await app.request(
				"/internal/mcp/session",
				bearer(token),
			)

			expect(res.status).toBe(401)
			expect(await res.json()).toEqual({
				error: "consent revoked",
			})
		})

		it("rejects a token for an organization the user has left", async ({
			expect,
		}) => {
			const { privateKey, jwks } = await mcpKeys()
			const auth = stubAuth()
			auth.api.getJwks.mockResolvedValue(jwks)
			const { app } = build({
				auth,
				store: memberlessStore(),
			})
			const token = await mcpToken(privateKey)

			const res = await app.request(
				"/internal/mcp/session",
				bearer(token),
			)

			expect(res.status).toBe(401)
			expect(await res.json()).toEqual({
				error: "not an organization member",
			})
		})

		it("describes the session behind a valid token", async ({
			expect,
		}) => {
			const { privateKey, jwks } = await mcpKeys()
			const auth = stubAuth()
			auth.api.getJwks.mockResolvedValue(jwks)
			const { app } = build({
				auth,
			})
			const token = await mcpToken(privateKey)

			const res = await app.request(
				"/internal/mcp/session",
				bearer(token),
			)

			expect(res.status).toBe(200)
			expect(await res.json()).toMatchObject({
				userId: "user-1",
				organizationId: "org-1",
				clientId: "client-1",
				scopes: ["documents:read", "documents:write"],
			})
		})

		it("reports no scopes for a token that carries none", async ({
			expect,
		}) => {
			const { privateKey, jwks } = await mcpKeys()
			const auth = stubAuth()
			auth.api.getJwks.mockResolvedValue(jwks)
			const { app } = build({
				auth,
			})
			const token = await mcpToken(privateKey, {
				scope: undefined,
			})

			const res = await app.request(
				"/internal/mcp/session",
				bearer(token),
			)

			const body = (await res.json()) as { scopes: string[] }

			expect(body.scopes).toEqual([])
		})

		it("answers 500 when the consent lookup fails", async ({
			expect,
		}) => {
			const { privateKey, jwks } = await mcpKeys()
			const auth = stubAuth()
			auth.api.getJwks.mockResolvedValue(jwks)
			const store = stubStore()
			store.hasOAuthConsent.mockRejectedValue(
				new Error("connection terminated"),
			)
			const { app } = build({ auth, store })
			const token = await mcpToken(privateKey)

			const res = await app.request(
				"/internal/mcp/session",
				bearer(token),
			)

			expect(res.status).toBe(500)
			expect(await res.json()).toEqual({
				error: "token validation failed",
			})
		})

		it("fetches the key set once across repeated calls", async ({
			expect,
		}) => {
			const { privateKey, jwks } = await mcpKeys()
			const auth = stubAuth()
			auth.api.getJwks.mockResolvedValue(jwks)
			const { app } = build({
				auth,
			})
			const token = await mcpToken(privateKey)

			await app.request(
				"/internal/mcp/session",
				bearer(token),
			)
			await app.request(
				"/internal/mcp/session",
				bearer(token),
			)

			expect(auth.api.getJwks).toHaveBeenCalledTimes(1)
		})

		// a signing key rotates without warning, so a cached key set
		// that no longer verifies is refreshed once before the token is
		// called invalid
		it("refetches the key set once before rejecting after a rotation", async ({
			expect,
		}) => {
			const stale = await mcpKeys()
			const current = await mcpKeys()
			const auth = stubAuth()
			auth.api.getJwks
				.mockResolvedValueOnce(stale.jwks)
				.mockResolvedValue(current.jwks)
			const { app } = build({
				auth,
			})
			const token = await mcpToken(current.privateKey)

			const res = await app.request(
				"/internal/mcp/session",
				bearer(token),
			)

			expect(res.status).toBe(200)
			expect(auth.api.getJwks).toHaveBeenCalledTimes(2)
		})
	})

	describe("POST /internal/documents/:documentId/branches/:branchId/operations", () => {
		const path = "/internal/documents/doc1/branches/b1/operations"

		it.for([
			{ name: "a body that is not JSON", input: "not json" },
			{ name: "a body with no operations", input: "{}" },
			{ name: "a body that is not an object", input: "null" },
			{
				name: "an operations field that is not an array",
				input: '{"operations":"append"}',
			},
		])("rejects $name", async ({ input }, { expect }) => {
			const { registry, openDirectConnection } =
				stubRegistry()
			const { app } = build({ hocuspocus: registry })

			const res = await app.request(path, {
				method: "POST",
				body: input,
				headers: { "Content-Type": "application/json" },
			})

			expect(res.status).toBe(400)
			expect(openDirectConnection).toHaveBeenCalledTimes(0)
		})

		it("applies the batch to the branch's live document", async ({
			expect,
		}) => {
			const doc = new Y.Doc()
			const { registry, openDirectConnection, disconnect } =
				stubRegistry({
					transact: (fn) => {
						fn(doc)

						return Promise.resolve()
					},
				})
			const { app } = build({ hocuspocus: registry })

			const res = await app.request(path, {
				method: "POST",
				body: JSON.stringify({
					operations: [
						{
							kind: "set_icon",
							icon: "lucide:siren",
						},
					],
				}),
				headers: { "Content-Type": "application/json" },
			})

			expect(res.status).toBe(200)
			expect(await res.json()).toEqual({
				applied: 1,
				errors: [],
			})
			expect(openDirectConnection).toHaveBeenCalledWith(
				"doc1-b1",
				{},
			)
			expect(doc.getText("icon").toJSON()).toBe(
				"lucide:siren",
			)
			expect(disconnect).toHaveBeenCalledTimes(1)
		})

		it("opens the connection as core's own when the batch is marked", async ({
			expect,
		}) => {
			const doc = new Y.Doc()
			const { registry, openDirectConnection } = stubRegistry(
				{
					documents: new Map([["doc1-b1", doc]]),
					transact: (fn) => {
						fn(doc)

						return Promise.resolve()
					},
				},
			)
			const { app } = build({ hocuspocus: registry })

			const res = await app.request(path, {
				method: "POST",
				body: JSON.stringify({
					system: true,
					operations: [
						{
							kind: "set_icon",
							icon: "lucide:siren",
						},
					],
				}),
				headers: { "Content-Type": "application/json" },
			})

			expect(res.status).toBe(200)
			expect(openDirectConnection).toHaveBeenCalledWith(
				"doc1-b1",
				{ system: true },
			)
		})

		// what an editor or the assistant sends: the persist that follows
		// carries no permission of core's, so a protected branch refuses it
		it.for([
			{ name: "no system field", input: {} },
			{
				name: "a system field of false",
				input: { system: false },
			},
			{
				name: "a system field that is a string",
				input: { system: "true" },
			},
			{
				name: "a system field that is a number",
				input: { system: 1 },
			},
		])(
			"opens the connection as an ordinary one given $name",
			async ({ input }, { expect }) => {
				const doc = new Y.Doc()
				const { registry, openDirectConnection } =
					stubRegistry({
						documents: new Map([
							["doc1-b1", doc],
						]),
						transact: (fn) => {
							fn(doc)

							return Promise.resolve()
						},
					})
				const { app } = build({ hocuspocus: registry })

				await app.request(path, {
					method: "POST",
					body: JSON.stringify({
						...input,
						operations: [
							{
								kind: "set_icon",
								icon: "lucide:siren",
							},
						],
					}),
					headers: {
						"Content-Type":
							"application/json",
					},
				})

				expect(
					openDirectConnection,
				).toHaveBeenCalledWith("doc1-b1", {})
			},
		)

		it("reports a failing operation without failing the request", async ({
			expect,
		}) => {
			const { registry } = stubRegistry()
			const { app } = build({ hocuspocus: registry })

			const res = await app.request(path, {
				method: "POST",
				body: JSON.stringify({
					operations: [
						{
							kind: "delete",
							block_uid: "missing",
						},
					],
				}),
				headers: { "Content-Type": "application/json" },
			})

			expect(res.status).toBe(200)
			const body = (await res.json()) as {
				applied: number
				errors: { index: number }[]
			}
			expect(body.applied).toBe(0)
			expect(body.errors).toHaveLength(1)
			expect(body.errors[0]?.index).toBe(0)
		})

		it("answers 500 when the document cannot be opened", async ({
			expect,
		}) => {
			const { registry } = stubRegistry({
				open: () =>
					Promise.reject(
						new Error("no such document"),
					),
			})
			const { app } = build({ hocuspocus: registry })

			const res = await app.request(path, {
				method: "POST",
				body: JSON.stringify({ operations: [] }),
				headers: { "Content-Type": "application/json" },
			})

			expect(res.status).toBe(500)
			expect(await res.json()).toEqual({
				error: "failed to open document",
			})
		})

		it("releases the connection when applying the batch throws", async ({
			expect,
		}) => {
			const { registry, disconnect } = stubRegistry({
				transact: () =>
					Promise.reject(
						new Error("transaction failed"),
					),
			})
			const { app } = build({ hocuspocus: registry })

			const res = await app.request(path, {
				method: "POST",
				body: JSON.stringify({ operations: [] }),
				headers: { "Content-Type": "application/json" },
			})

			expect(res.status).toBe(500)
			expect(await res.json()).toEqual({
				error: "failed to apply operations",
			})
			expect(disconnect).toHaveBeenCalledTimes(1)
		})

		it("still answers when releasing the connection fails", async ({
			expect,
		}) => {
			const { registry, disconnect } = stubRegistry()
			disconnect.mockRejectedValue(
				new Error("already closed"),
			)
			const { app } = build({ hocuspocus: registry })

			const res = await app.request(path, {
				method: "POST",
				body: JSON.stringify({ operations: [] }),
				headers: { "Content-Type": "application/json" },
			})

			expect(res.status).toBe(200)
		})
	})

	describe("cors", () => {
		it("allows a trusted origin to send credentials", async ({
			expect,
		}) => {
			const { app } = build()

			const res = await app.request("/auth-config", {
				headers: { Origin: "http://localhost:8080" },
			})

			expect(
				res.headers.get("Access-Control-Allow-Origin"),
			).toBe("http://localhost:8080")
			expect(
				res.headers.get(
					"Access-Control-Allow-Credentials",
				),
			).toBe("true")
		})

		it("does not answer an untrusted origin with its own origin", async ({
			expect,
		}) => {
			const { app } = build()

			const res = await app.request("/auth-config", {
				headers: { Origin: "http://evil.example" },
			})

			expect(
				res.headers.get("Access-Control-Allow-Origin"),
			).not.toBe("http://evil.example")
		})
	})
})
