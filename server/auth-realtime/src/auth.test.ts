import { describe, it, vi } from "vitest"
import {
	authMethods,
	checkSignUpAllowed,
	confirmWithPassword,
	createAuth,
	createOrganizationHooks,
	createSecondaryStorage,
	electronCallbackOverride,
	invitationLink,
	organizationClaims,
	refuseLastMemberDeletion,
	sendChangeEmailConfirmation,
	toPublicAuthUrl,
	verificationTemplate,
	type SecondaryStorageClient,
} from "./auth.js"
import { createDatabase, type Store } from "./db.js"
import {
	stubCore,
	stubLog,
	stubStore,
	testEnv,
	type StubCore,
} from "./test-helpers.js"

function stubRedis() {
	return {
		get: vi.fn().mockResolvedValue(null),
		getDel: vi.fn().mockResolvedValue(null),
		incr: vi.fn().mockResolvedValue(1),
		expire: vi.fn().mockResolvedValue(1),
		set: vi.fn().mockResolvedValue("OK"),
		del: vi.fn().mockResolvedValue(1),
	}
}

const GOOGLE = {
	clientId: "google-id",
	clientSecret: "google-secret",
}

// better-auth starts its context eagerly, and the mcp plugin seeds its
// resource row from there — the one thing in this file that dials the
// database. No test awaits that context, so its connection failure would
// surface as an unhandled rejection in whichever file was running.
function dropContextInit(auth: { $context: Promise<unknown> }): void {
	void auth.$context.catch(() => undefined)
}

// a real auth instance, built against a pg pool that only ever fails to
// dial. Its `options` are what better-auth would invoke at runtime, so the
// callbacks are asserted exactly as wired.
function buildAuth(
	overrides: {
		env?: Parameters<typeof createAuth>[0]["env"]
		core?: StubCore
		redis?: SecondaryStorageClient
		store?: Store
	} = {},
) {
	const real = createDatabase("postgresql://u:p@localhost/d")
	const core = overrides.core ?? stubCore()
	const log = stubLog()
	const auth = createAuth({
		env: overrides.env ?? testEnv(),
		store: overrides.store ?? stubStore(),
		dialect: real.dialect,
		redis: overrides.redis ?? stubRedis(),
		core,
		log,
	})

	dropContextInit(auth)

	return { auth, core, log }
}

// an email callback is left out when no email can be sent, so its type
// admits undefined even on an instance built with email enabled.
function configured<T>(callback: T | undefined): T {
	if (!callback) {
		throw new Error("callback is not configured")
	}

	return callback
}

describe("authMethods", () => {
	it("always offers email and password", ({ expect }) => {
		expect(authMethods(testEnv())).toEqual(["email-password"])
	})

	it("offers each provider that has credentials", ({ expect }) => {
		const methods = authMethods(
			testEnv({
				socialProviders: {
					google: GOOGLE,
					slack: {
						clientId: "slack-id",
						clientSecret: "slack-secret",
					},
				},
			}),
		)

		expect(methods).toEqual(["email-password", "google", "slack"])
	})
})

describe("toPublicAuthUrl", () => {
	// better-auth builds links from the bare origin, but the browser can
	// only reach this service behind the proxy's /auth-realtime prefix
	it("rewrites a link built on the bare origin onto the public prefix", ({
		expect,
	}) => {
		const url = toPublicAuthUrl(
			testEnv(),
			"http://localhost:8080/api/auth/reset-password/tok",
		)

		expect(url).toBe(
			"http://localhost:8080/auth-realtime/api/auth/reset-password/tok",
		)
	})

	it("leaves a link on another origin alone", ({ expect }) => {
		const url = toPublicAuthUrl(testEnv(), "http://elsewhere/reset")

		expect(url).toBe("http://elsewhere/reset")
	})
})

describe("verificationTemplate", () => {
	// better-auth's verification tokens are JWTs; only the payload is read
	function token(payload: Record<string, unknown>): string {
		const body = Buffer.from(JSON.stringify(payload)).toString(
			"base64url",
		)

		return `header.${body}.signature`
	}

	it("sends the new-address wording for a change of address", ({
		expect,
	}) => {
		expect(
			verificationTemplate(
				token({
					email: "old@b.c",
					updateTo: "new@b.c",
					requestType:
						"change-email-verification",
				}),
			),
		).toBe("email_verification")
	})

	it.for([
		{
			name: "a signup token, which names no other address",
			input: "header.eyJlbWFpbCI6ImFAYi5jIn0.signature",
		},
		{ name: "a token with no payload at all", input: "opaque" },
		{
			name: "a payload that is not JSON",
			input: "header.@@@.signature",
		},
	])(
		"sends the activation wording for $name",
		({ input }, { expect }) => {
			expect(verificationTemplate(input)).toBe(
				"signup_verification",
			)
		},
	)
})

describe("electronCallbackOverride", () => {
	const electronContext = {
		path: "/sign-in/social",
		query: { client_id: "electron" },
		body: { provider: "google" },
	}

	// the electron plugin sends no callbackURL, so better-auth would
	// redirect to the server root after the OAuth callback
	it("redirects the desktop OAuth proxy to the handoff page", ({
		expect,
	}) => {
		const override = electronCallbackOverride(
			electronContext,
			"http://localhost:8080",
		)

		expect(override).toEqual({
			context: {
				body: {
					provider: "google",
					callbackURL:
						"http://localhost:8080/desktop-auth",
				},
			},
		})
	})

	it.for([
		{
			name: "another path",
			input: { ...electronContext, path: "/sign-in/email" },
		},
		{
			name: "a non-electron client",
			input: {
				...electronContext,
				query: { client_id: "web" },
			},
		},
		{
			name: "no query at all",
			input: { ...electronContext, query: null },
		},
		{
			name: "a callback the caller already chose",
			input: {
				...electronContext,
				body: {
					provider: "google",
					callbackURL:
						"http://localhost:8080/back",
				},
			},
		},
	])("leaves $name untouched", ({ input }, { expect }) => {
		expect(
			electronCallbackOverride(
				input,
				"http://localhost:8080",
			),
		).toBeUndefined()
	})
})

describe("invitationLink", () => {
	it("carries everything the accept-invite page renders before sign-in", ({
		expect,
	}) => {
		const link = invitationLink(testEnv(), {
			id: "inv-1",
			email: "a@b.c",
			inviter: { user: { name: "Ada" } },
			organization: { id: "org-1", name: "Acme" },
		})

		expect(link).toBe(
			"http://localhost:8080/accept-invite?id=inv-1&email=a@b.c&inviter=Ada&orgName=Acme&orgId=org-1",
		)
	})
})

describe("organizationClaims", () => {
	it("binds the token to the user's organization", async ({ expect }) => {
		const store = stubStore()
		store.userOrganizationId.mockResolvedValue("org-1")

		expect(
			await organizationClaims(store, { id: "user-1" }),
		).toEqual({ org_id: "org-1" })
	})

	it("claims nothing for a user in no organization", async ({
		expect,
	}) => {
		const store = stubStore()
		store.userOrganizationId.mockResolvedValue(null)

		expect(
			await organizationClaims(store, { id: "user-1" }),
		).toEqual({})
	})

	it("claims nothing when the token has no user", async ({ expect }) => {
		const store = stubStore()

		expect(await organizationClaims(store, null)).toEqual({})
		expect(store.userOrganizationId).toHaveBeenCalledTimes(0)
	})
})

describe("createOrganizationHooks", () => {
	describe("canCreateOrganization", () => {
		it.for([
			{ name: "below the limit", input: 99, expected: true },
			{ name: "at the limit", input: 100, expected: false },
			{ name: "past the limit", input: 101, expected: false },
		])(
			"answers $expected with the count $name",
			async ({ input, expected }, { expect }) => {
				const store = stubStore()
				store.totalOrganizationCount.mockResolvedValue(
					input,
				)
				const hooks = createOrganizationHooks({
					env: testEnv({ maxOrganizations: 100 }),
					store,
					core: stubCore(),
				})

				expect(
					await hooks.canCreateOrganization(),
				).toBe(expected)
			},
		)
	})

	describe("afterCreateOrganization", () => {
		it("asks core to initialize the new organization", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = createOrganizationHooks({
				env: testEnv(),
				store: stubStore(),
				core,
			})

			await hooks.afterCreateOrganization({
				organization: { id: "org-1" },
			})

			expect(
				core.initializeOrganization,
			).toHaveBeenCalledWith("org-1")
		})
	})

	describe("beforeDeleteOrganization", () => {
		it("tears the organization down in core first", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = createOrganizationHooks({
				env: testEnv(),
				store: stubStore(),
				core,
			})

			await hooks.beforeDeleteOrganization({
				organization: { id: "org-1" },
			})

			expect(core.teardownOrganization).toHaveBeenCalledWith(
				"org-1",
			)
		})

		// better-auth aborts the deletion on a throw, which is what
		// keeps a half-torn-down organization from being deleted and
		// orphaning everything core owns for it
		it("propagates a failed teardown so the organization survives", async ({
			expect,
		}) => {
			const failure = new Error("core unreachable")
			const core = stubCore()
			core.teardownOrganization.mockRejectedValue(failure)
			const hooks = createOrganizationHooks({
				env: testEnv(),
				store: stubStore(),
				core,
			})

			await expect(
				hooks.beforeDeleteOrganization({
					organization: { id: "org-1" },
				}),
			).rejects.toBe(failure)
		})
	})

	describe("sendInvitationEmail", () => {
		it("emails the invitee a link to the organization's invite page", async ({
			expect,
		}) => {
			const core = stubCore()
			const hooks = createOrganizationHooks({
				env: testEnv(),
				store: stubStore(),
				core,
			})

			await hooks.sendInvitationEmail({
				id: "inv-1",
				email: "a@b.c",
				inviter: { user: { name: "Ada" } },
				organization: { id: "org-1", name: "Acme" },
			})

			expect(core.sendEmail).toHaveBeenCalledWith(
				"organization_invitation",
				{
					email: "a@b.c",
					organization: "Acme",
					link: "http://localhost:8080/accept-invite?id=inv-1&email=a@b.c&inviter=Ada&orgName=Acme&orgId=org-1",
				},
			)
		})
	})
})

describe("confirmWithPassword", () => {
	const signedIn = () =>
		Promise.resolve({ id: "u1", email: "ada@oxynote.test" })

	// an email change by an account whose password checks out, to an
	// address nobody holds, unless a test says otherwise.
	function passwordContext(
		overrides: {
			path?: string
			body?: Record<string, unknown>
			account?: { password?: string | null } | null
			verifies?: boolean
			holder?: unknown
		} = {},
	) {
		const internalAdapter = {
			findCredentialAccount: vi
				.fn()
				.mockResolvedValue(
					overrides.account === undefined
						? { password: "hash" }
						: overrides.account,
				),
			findUserByEmail: vi
				.fn()
				.mockResolvedValue(overrides.holder ?? null),
			updateUser: vi.fn().mockResolvedValue({}),
		}
		const password = {
			verify: vi
				.fn()
				.mockResolvedValue(overrides.verifies ?? true),
		}
		const json = vi.fn((body: { status: boolean }) => body)

		return {
			ctx: {
				path: overrides.path ?? "/change-email",
				body: overrides.body ?? {
					newEmail: "new@oxynote.test",
					password: "secret",
				},
				context: { internalAdapter, password },
				json,
			},
			internalAdapter,
			password,
			json,
		}
	}

	it("leaves every other endpoint alone", async ({ expect }) => {
		const { ctx, internalAdapter } = passwordContext({
			path: "/sign-in/email",
		})
		const currentUser = vi.fn(signedIn)

		const answer = await confirmWithPassword(ctx, currentUser)

		expect(answer).toBeUndefined()
		expect.soft(currentUser).not.toHaveBeenCalled()
		expect.soft(
			internalAdapter.findCredentialAccount,
		).not.toHaveBeenCalled()
	})

	it("leaves a caller without a session to the endpoint", async ({
		expect,
	}) => {
		const { ctx, internalAdapter } = passwordContext()

		const answer = await confirmWithPassword(ctx, () =>
			Promise.resolve(undefined),
		)

		expect(answer).toBeUndefined()
		expect.soft(
			internalAdapter.findCredentialAccount,
		).not.toHaveBeenCalled()
		expect.soft(internalAdapter.updateUser).not.toHaveBeenCalled()
	})

	it("refuses an account that has no password", async ({ expect }) => {
		const { ctx, internalAdapter, password } = passwordContext({
			account: null,
		})

		await expect(
			confirmWithPassword(ctx, signedIn),
		).rejects.toMatchObject({
			body: { code: "CREDENTIAL_ACCOUNT_NOT_FOUND" },
		})

		expect.soft(
			internalAdapter.findCredentialAccount,
		).toHaveBeenCalledWith("u1")
		expect.soft(password.verify).not.toHaveBeenCalled()
		expect.soft(internalAdapter.updateUser).not.toHaveBeenCalled()
	})

	it("refuses a request that carries no password", async ({ expect }) => {
		const { ctx, internalAdapter, password } = passwordContext({
			body: { newEmail: "new@oxynote.test" },
		})

		await expect(
			confirmWithPassword(ctx, signedIn),
		).rejects.toMatchObject({ body: { code: "INVALID_PASSWORD" } })

		expect.soft(password.verify).not.toHaveBeenCalled()
		expect.soft(internalAdapter.updateUser).not.toHaveBeenCalled()
	})

	it("refuses a wrong password", async ({ expect }) => {
		const { ctx, internalAdapter, password } = passwordContext({
			verifies: false,
		})

		await expect(
			confirmWithPassword(ctx, signedIn),
		).rejects.toMatchObject({ body: { code: "INVALID_PASSWORD" } })

		expect.soft(password.verify).toHaveBeenCalledWith({
			hash: "hash",
			password: "secret",
		})
		expect.soft(internalAdapter.updateUser).not.toHaveBeenCalled()
	})

	it("lets a deletion through to better-auth once the password checks out", async ({
		expect,
	}) => {
		const { ctx, internalAdapter, password, json } =
			passwordContext({
				path: "/delete-user",
				body: { password: "secret" },
			})

		const answer = await confirmWithPassword(ctx, signedIn)

		expect(answer).toBeUndefined()
		expect.soft(password.verify).toHaveBeenCalledTimes(1)
		expect.soft(
			internalAdapter.findUserByEmail,
		).not.toHaveBeenCalled()
		expect.soft(internalAdapter.updateUser).not.toHaveBeenCalled()
		expect.soft(json).not.toHaveBeenCalled()
	})

	it("stores the new address unverified once the password checks out", async ({
		expect,
	}) => {
		const { ctx, internalAdapter, password } = passwordContext({
			body: {
				newEmail: "New@Oxynote.test",
				password: "secret",
			},
		})

		const answer = await confirmWithPassword(ctx, signedIn)

		expect(answer).toEqual({ status: true })
		expect.soft(password.verify).toHaveBeenCalledTimes(1)
		expect.soft(
			internalAdapter.findUserByEmail,
		).toHaveBeenCalledWith("new@oxynote.test")
		expect.soft(internalAdapter.updateUser).toHaveBeenCalledWith(
			"u1",
			{
				email: "new@oxynote.test",
				emailVerified: false,
			},
		)
	})

	it("refuses an address another account holds", async ({ expect }) => {
		const { ctx, internalAdapter } = passwordContext({
			holder: { user: { id: "u2" } },
		})

		await expect(
			confirmWithPassword(ctx, signedIn),
		).rejects.toMatchObject({
			body: { code: "USER_ALREADY_EXISTS_USE_ANOTHER_EMAIL" },
		})

		expect.soft(internalAdapter.updateUser).not.toHaveBeenCalled()
	})

	it.for([
		{ name: "a malformed address", input: "not-an-email" },
		{ name: "the current address", input: "ada@oxynote.test" },
	])(
		"leaves $name to the endpoint's own validation",
		async ({ input }, { expect }) => {
			const { ctx, internalAdapter, json } = passwordContext({
				body: { newEmail: input, password: "secret" },
			})

			const answer = await confirmWithPassword(ctx, signedIn)

			expect(answer).toBeUndefined()
			expect.soft(
				internalAdapter.findUserByEmail,
			).not.toHaveBeenCalled()
			expect.soft(
				internalAdapter.updateUser,
			).not.toHaveBeenCalled()
			expect.soft(json).not.toHaveBeenCalled()
		},
	)
})

describe("checkSignUpAllowed", () => {
	it("allows any signup with more than one organization allowed", async ({
		expect,
	}) => {
		const store = stubStore()

		await expect(
			checkSignUpAllowed(
				testEnv({ maxOrganizations: 100 }),
				store,
				"a@b.c",
				true,
			),
		).resolves.toBeUndefined()
		expect.soft(store.hasPendingInvitation).not.toHaveBeenCalled()
	})

	// the bootstrap creates the admin from the service's own code
	it("allows a user created outside a request", async ({ expect }) => {
		const store = stubStore()
		store.hasPendingInvitation.mockResolvedValue(false)

		await expect(
			checkSignUpAllowed(
				testEnv({ maxOrganizations: 1 }),
				store,
				"a@b.c",
				false,
			),
		).resolves.toBeUndefined()
		expect.soft(store.hasPendingInvitation).not.toHaveBeenCalled()
	})

	it("allows an invited address", async ({ expect }) => {
		const store = stubStore()
		store.hasPendingInvitation.mockResolvedValue(true)

		await expect(
			checkSignUpAllowed(
				testEnv({ maxOrganizations: 1 }),
				store,
				"a@b.c",
				true,
			),
		).resolves.toBeUndefined()
		expect.soft(store.hasPendingInvitation).toHaveBeenCalledWith(
			"a@b.c",
		)
	})

	it("refuses an uninvited address", async ({ expect }) => {
		const store = stubStore()
		store.hasPendingInvitation.mockResolvedValue(false)

		await expect(
			checkSignUpAllowed(
				testEnv({ maxOrganizations: 1 }),
				store,
				"a@b.c",
				true,
			),
		).rejects.toMatchObject({
			statusCode: 400,
			body: { code: "SIGN_UP_DISABLED" },
		})
	})

	it("propagates a failed lookup", async ({ expect }) => {
		const failure = new Error("connection terminated")
		const store = stubStore()
		store.hasPendingInvitation.mockRejectedValue(failure)

		await expect(
			checkSignUpAllowed(
				testEnv({ maxOrganizations: 1 }),
				store,
				"a@b.c",
				true,
			),
		).rejects.toBe(failure)
	})
})

describe("refuseLastMemberDeletion", () => {
	const signedIn = () => Promise.resolve({ id: "u1", email: "a@b.c" })

	it.for([
		{ name: "the deletion request", input: "/delete-user" },
		{
			name: "the emailed callback",
			input: "/delete-user/callback",
		},
	])(
		"refuses $name from the only member",
		async ({ input }, { expect }) => {
			const store = stubStore()
			store.organizationMemberCount.mockResolvedValue(1)

			await expect(
				refuseLastMemberDeletion(
					testEnv({ maxOrganizations: 1 }),
					store,
					input,
					signedIn,
				),
			).rejects.toMatchObject({
				statusCode: 403,
				body: { code: "LAST_ORGANIZATION_MEMBER" },
			})
			expect.soft(
				store.userOrganizationId,
			).toHaveBeenCalledWith("u1")
			expect.soft(
				store.organizationMemberCount,
			).toHaveBeenCalledWith("org-1")
		},
	)

	it("lets a member go while others remain", async ({ expect }) => {
		const store = stubStore()
		store.organizationMemberCount.mockResolvedValue(2)

		await expect(
			refuseLastMemberDeletion(
				testEnv({ maxOrganizations: 1 }),
				store,
				"/delete-user",
				signedIn,
			),
		).resolves.toBeUndefined()
	})

	it("lets a user without an organization go", async ({ expect }) => {
		const store = stubStore()
		store.userOrganizationId.mockResolvedValue(null)

		await expect(
			refuseLastMemberDeletion(
				testEnv({ maxOrganizations: 1 }),
				store,
				"/delete-user",
				signedIn,
			),
		).resolves.toBeUndefined()
		expect.soft(
			store.organizationMemberCount,
		).not.toHaveBeenCalled()
	})

	it("leaves a request without a session to the endpoint", async ({
		expect,
	}) => {
		const store = stubStore()

		await expect(
			refuseLastMemberDeletion(
				testEnv({ maxOrganizations: 1 }),
				store,
				"/delete-user",
				() => Promise.resolve(undefined),
			),
		).resolves.toBeUndefined()
		expect.soft(store.userOrganizationId).not.toHaveBeenCalled()
	})

	it.for([
		{
			name: "with more than one organization allowed",
			input: { maxOrganizations: 100, path: "/delete-user" },
		},
		{
			name: "on every other endpoint",
			input: { maxOrganizations: 1, path: "/change-email" },
		},
	])("checks nothing $name", async ({ input }, { expect }) => {
		const store = stubStore()
		const currentUser = vi.fn(signedIn)

		await expect(
			refuseLastMemberDeletion(
				testEnv({
					maxOrganizations:
						input.maxOrganizations,
				}),
				store,
				input.path,
				currentUser,
			),
		).resolves.toBeUndefined()
		expect.soft(currentUser).not.toHaveBeenCalled()
		expect.soft(store.userOrganizationId).not.toHaveBeenCalled()
	})

	it("propagates a failed lookup", async ({ expect }) => {
		const failure = new Error("connection terminated")
		const store = stubStore()
		store.organizationMemberCount.mockRejectedValue(failure)

		await expect(
			refuseLastMemberDeletion(
				testEnv({ maxOrganizations: 1 }),
				store,
				"/delete-user",
				signedIn,
			),
		).rejects.toBe(failure)
	})
})

describe("sendChangeEmailConfirmation", () => {
	it("asks the current address to approve the change", async ({
		expect,
	}) => {
		const core = stubCore()

		await sendChangeEmailConfirmation(testEnv(), core, {
			user: { email: "old@b.c" },
			newEmail: "new@b.c",
			url: "http://localhost:8080/api/auth/verify-email?token=tok&callbackURL=%2F",
		})

		expect(core.sendEmail).toHaveBeenCalledTimes(1)
		expect(core.sendEmail).toHaveBeenCalledWith(
			"email_change_confirmation",
			{
				email: "old@b.c",
				link: "http://localhost:8080/auth-realtime/api/auth/verify-email?token=tok&callbackURL=%2F",
			},
		)
	})

	it("sends the bootstrap admin's new address the link that completes the change", async ({
		expect,
	}) => {
		const core = stubCore()

		await sendChangeEmailConfirmation(testEnv(), core, {
			user: { email: "admin@example.com" },
			newEmail: "new@b.c",
			url: "http://localhost:8080/api/auth/verify-email?token=tok&callbackURL=%2F",
		})

		expect(core.sendEmail).toHaveBeenCalledTimes(1)
		const [template, data] = core.sendEmail.mock.calls[0] ?? []
		expect(template).toBe("email_verification")
		expect(data?.email).toBe("new@b.c")

		const link = new URL(data?.link ?? "")
		expect(link.origin + link.pathname).toBe(
			"http://localhost:8080/auth-realtime/api/auth/verify-email",
		)
		expect(link.searchParams.get("callbackURL")).toBe("/")

		const payload = (link.searchParams.get("token") ?? "").split(
			".",
		)[1]
		expect(
			JSON.parse(
				Buffer.from(
					payload ?? "",
					"base64url",
				).toString(),
			),
		).toMatchObject({
			email: "admin@example.com",
			updateTo: "new@b.c",
			requestType: "change-email-verification",
		})
	})

	it("propagates a failed send", async ({ expect }) => {
		const failure = new Error("core unreachable")
		const core = stubCore()
		core.sendEmail.mockRejectedValue(failure)

		await expect(
			sendChangeEmailConfirmation(testEnv(), core, {
				user: { email: "old@b.c" },
				newEmail: "new@b.c",
				url: "http://localhost:8080/api/auth/verify-email?token=tok",
			}),
		).rejects.toBe(failure)
	})
})

describe("createSecondaryStorage", () => {
	it("reads through to the redis client", async ({ expect }) => {
		const redis = stubRedis()
		redis.get.mockResolvedValue("value")

		expect(await createSecondaryStorage(redis).get("key")).toBe(
			"value",
		)
		expect(redis.get).toHaveBeenCalledWith("key")
	})

	it("deletes on read for getAndDelete", async ({ expect }) => {
		const redis = stubRedis()
		redis.getDel.mockResolvedValue("value")

		expect(
			await createSecondaryStorage(redis).getAndDelete("key"),
		).toBe("value")
		expect(redis.getDel).toHaveBeenCalledWith("key")
		expect(redis.get).toHaveBeenCalledTimes(0)
	})

	// NX applies the TTL only to a key that has none, so the window is
	// fixed from creation and later increments cannot extend it
	it("expires a counter a fixed window after its first increment", async ({
		expect,
	}) => {
		const redis = stubRedis()
		redis.incr.mockResolvedValue(3)

		expect(
			await createSecondaryStorage(redis).increment(
				"key",
				60,
			),
		).toBe(3)
		expect(redis.expire).toHaveBeenCalledWith("key", 60, "NX")
	})

	it("writes with an expiry when given a ttl", async ({ expect }) => {
		const redis = stubRedis()

		await createSecondaryStorage(redis).set("key", "value", 60)

		expect(redis.set).toHaveBeenCalledWith("key", "value", {
			EX: 60,
		})
	})

	it("writes without an expiry when given no ttl", async ({ expect }) => {
		const redis = stubRedis()

		await createSecondaryStorage(redis).set("key", "value")

		expect(redis.set).toHaveBeenCalledWith("key", "value")
	})

	it("deletes the key", async ({ expect }) => {
		const redis = stubRedis()

		await createSecondaryStorage(redis).delete("key")

		expect(redis.del).toHaveBeenCalledWith("key")
	})

	it.for([
		{ name: "get", input: "get" as const },
		{ name: "getAndDelete", input: "getDel" as const },
		{ name: "increment", input: "incr" as const },
		{ name: "set", input: "set" as const },
		{ name: "delete", input: "del" as const },
	])(
		"propagates a redis failure from $name",
		async ({ input }, { expect }) => {
			const failure = new Error("redis unreachable")
			const redis = stubRedis()
			redis[input].mockRejectedValue(failure)
			const storage = createSecondaryStorage(redis)

			const calls = {
				get: () => storage.get("key"),
				getDel: () => storage.getAndDelete("key"),
				incr: () => storage.increment("key", 60),
				set: () => storage.set("key", "value"),
				del: () => storage.delete("key"),
			}

			await expect(calls[input]()).rejects.toBe(failure)
		},
	)
})

describe("createAuth", () => {
	it("matches on the bare origin so the proxy's stripped prefix lines up", ({
		expect,
	}) => {
		const { auth } = buildAuth()

		expect(auth.options.baseURL).toBe("http://localhost:8080")
		expect(auth.options.basePath).toBe("/api/auth")
	})

	// the default is the origin root, which serves the frontend's
	// document page and reads no error query
	it("points failed requests at the frontend's error page", ({
		expect,
	}) => {
		const { auth } = buildAuth()

		expect(auth.options.onAPIError.errorURL).toBe(
			"http://localhost:8080/auth-error",
		)
	})

	it("takes better-auth's level from the configured one", ({
		expect,
	}) => {
		const { auth } = buildAuth({
			env: testEnv({ logLevel: "ERROR" }),
		})

		expect(auth.options.logger.level).toBe("error")
	})

	it("writes better-auth's own output through the service logger", ({
		expect,
	}) => {
		const { auth, log } = buildAuth()

		auth.options.logger.log("warn", "trusted origin missing")

		expect(log.warn.mock.calls).toEqual([
			["better-auth: trusted origin missing"],
		])
	})

	it("keeps secondary storage when a redis client is given", ({
		expect,
	}) => {
		const { auth } = buildAuth()

		expect(auth.options.secondaryStorage).toBeDefined()
	})

	// a deployment without valkey hands createAuth no client at all,
	// which has to leave better-auth reading sessions from the database
	// rather than half-configured with an unusable storage. Nothing
	// pins rateLimit.storage either way: better-auth derives it from
	// whether a secondary storage exists, so it counts in valkey when
	// there is one and in memory when there is not
	it("runs without secondary storage when given no redis client", ({
		expect,
	}) => {
		const real = createDatabase("postgresql://u:p@localhost/d")
		const auth = createAuth({
			env: testEnv(),
			store: stubStore(),
			dialect: real.dialect,
			core: stubCore(),
			log: stubLog(),
		})

		dropContextInit(auth)

		expect(auth.options.secondaryStorage).toBeUndefined()
		expect(auth.options.session.storeSessionInDatabase).toBe(true)
		expect(auth.options.rateLimit).toEqual({ enabled: true })
	})

	it("registers no social provider without credentials", ({ expect }) => {
		const { auth } = buildAuth()

		expect(Object.keys(auth.options.socialProviders)).toEqual([])
	})

	// a provider registered with half its credentials would send users
	// into a broken OAuth redirect instead of failing at sign-in
	it("registers a configured provider with a browser-reachable redirect", ({
		expect,
	}) => {
		const { auth } = buildAuth({
			env: testEnv({ socialProviders: { google: GOOGLE } }),
		})

		expect(auth.options.socialProviders.google).toEqual({
			clientId: "google-id",
			clientSecret: "google-secret",
			redirectURI:
				"http://localhost:8080/auth-realtime/api/auth/callback/google",
		})
	})

	it.for([
		{ name: "enabled", input: true, expected: true },
		{ name: "disabled", input: false, expected: false },
	])(
		"leaves rate limiting $name as configured",
		({ input, expected }, { expect }) => {
			const { auth } = buildAuth({
				env: testEnv({ rateLimitEnabled: input }),
			})

			expect(auth.options.rateLimit.enabled).toBe(expected)
		},
	)

	it("trusts the configured origins", ({ expect }) => {
		const { auth } = buildAuth({
			env: testEnv({
				trustedOrigins: [
					"http://localhost:8080",
					"oxynote://",
				],
			}),
		})

		expect(auth.options.trustedOrigins).toEqual([
			"http://localhost:8080",
			"oxynote://",
		])
	})

	describe("email callbacks", () => {
		it("sends a password reset on the public link", async ({
			expect,
		}) => {
			const { auth, core } = buildAuth()

			await configured(
				auth.options.emailAndPassword.sendResetPassword,
			)({
				user: { email: "a@b.c" },
				url: "http://localhost:8080/api/auth/reset/tok",
				token: "tok",
			} as never)

			expect(core.sendEmail).toHaveBeenCalledWith(
				"password_reset",
				{
					email: "a@b.c",
					link: "http://localhost:8080/auth-realtime/api/auth/reset/tok",
				},
			)
		})

		// a duplicate signup gets better-auth's synthetic success so a
		// browser cannot probe which addresses have accounts; the real
		// owner is told through their inbox
		it("tells the existing owner rather than the browser about a duplicate signup", async ({
			expect,
		}) => {
			const { auth, core } = buildAuth()

			await auth.options.emailAndPassword.onExistingUserSignUp(
				{ user: { email: "a@b.c" } } as never,
			)

			expect(core.sendEmail).toHaveBeenCalledWith(
				"account_exists",
				{
					email: "a@b.c",
					link: "http://localhost:8080/login",
				},
			)
		})

		it("sends the signup verification on the public link", async ({
			expect,
		}) => {
			const { auth, core } = buildAuth()

			await configured(
				auth.options.emailVerification
					.sendVerificationEmail,
			)({
				user: { email: "a@b.c" },
				url: "http://localhost:8080/api/auth/verify/tok",
				token: "tok",
			} as never)

			expect(core.sendEmail).toHaveBeenCalledWith(
				"signup_verification",
				{
					email: "a@b.c",
					link: "http://localhost:8080/auth-realtime/api/auth/verify/tok",
				},
			)
		})

		// the first step of a change of address asks the current
		// address to approve it; sending this to the new address would
		// ask it to approve its own claim
		it("asks the current address to approve a change of address", async ({
			expect,
		}) => {
			const { auth, core } = buildAuth()

			await configured(
				auth.options.user.changeEmail
					.sendChangeEmailConfirmation,
			)({
				user: { email: "old@b.c" },
				newEmail: "new@b.c",
				url: "http://localhost:8080/api/auth/change/tok",
				token: "tok",
			} as never)

			expect(core.sendEmail).toHaveBeenCalledWith(
				"email_change_confirmation",
				{
					email: "old@b.c",
					link: "http://localhost:8080/auth-realtime/api/auth/change/tok",
				},
			)
		})

		// the second step, which better-auth sends through the same
		// callback as a signup activation
		it("asks the new address to verify itself", async ({
			expect,
		}) => {
			const { auth, core } = buildAuth()

			await configured(
				auth.options.emailVerification
					.sendVerificationEmail,
			)({
				user: { email: "new@b.c" },
				url: "http://localhost:8080/api/auth/verify/tok",
				token: `header.${Buffer.from(
					JSON.stringify({
						email: "old@b.c",
						updateTo: "new@b.c",
					}),
				).toString("base64url")}.signature`,
			} as never)

			expect(core.sendEmail).toHaveBeenCalledWith(
				"email_verification",
				{
					email: "new@b.c",
					link: "http://localhost:8080/auth-realtime/api/auth/verify/tok",
				},
			)
		})

		it("confirms an account deletion by email", async ({
			expect,
		}) => {
			const { auth, core } = buildAuth()

			await configured(
				auth.options.user.deleteUser
					.sendDeleteAccountVerification,
			)({
				user: { email: "a@b.c" },
				url: "http://localhost:8080/api/auth/delete/tok",
				token: "tok",
			} as never)

			expect(core.sendEmail).toHaveBeenCalledWith(
				"user_deletion",
				{
					email: "a@b.c",
					link: "http://localhost:8080/auth-realtime/api/auth/delete/tok",
				},
			)
		})

		it("propagates a failed send so better-auth fails the request", async ({
			expect,
		}) => {
			const failure = new Error("core unreachable")
			const core = stubCore()
			core.sendEmail.mockRejectedValue(failure)
			const { auth } = buildAuth({ core })

			await expect(
				configured(
					auth.options.emailAndPassword
						.sendResetPassword,
				)({
					user: { email: "a@b.c" },
					url: "http://localhost:8080/api/auth/reset/tok",
					token: "tok",
				} as never),
			).rejects.toBe(failure)
		})

		// nothing can reach an inbox, so no flow may wait on one
		it("sends nothing and verifies nothing when email is disabled", ({
			expect,
		}) => {
			const { auth } = buildAuth({
				env: testEnv({ emailEnabled: false }),
			})

			expect(
				auth.options.emailAndPassword
					.requireEmailVerification,
			).toBe(false)
			expect(
				auth.options.emailVerification.sendOnSignUp,
			).toBe(false)
			expect(
				auth.options.emailVerification
					.sendVerificationEmail,
			).toBeUndefined()
			expect(
				auth.options.emailAndPassword.sendResetPassword,
			).toBeUndefined()
			expect(
				auth.options.user.deleteUser
					.sendDeleteAccountVerification,
			).toBeUndefined()
		})
	})

	// core scopes every request by the session's organization, so a
	// session that reaches the database without one is unusable
	describe("session organization binding", () => {
		it("stamps the user's organization onto a new session", async ({
			expect,
		}) => {
			const store = stubStore()
			store.userOrganizationId.mockResolvedValue("org-1")
			const { auth } = buildAuth({ store })

			const result =
				await auth.options.databaseHooks.session.create.before(
					{
						userId: "user-1",
						token: "t",
					} as never,
				)

			expect(result).toMatchObject({
				data: {
					userId: "user-1",
					activeOrganizationId: "org-1",
				},
			})
		})

		it("stamps null when the user belongs to no organization", async ({
			expect,
		}) => {
			const store = stubStore()
			store.userOrganizationId.mockResolvedValue(null)
			const { auth } = buildAuth({ store })

			const result =
				await auth.options.databaseHooks.session.create.before(
					{
						userId: "user-1",
						token: "t",
					} as never,
				)

			expect(result).toMatchObject({
				data: { activeOrganizationId: null },
			})
		})

		it("re-resolves the organization when a session is updated", async ({
			expect,
		}) => {
			const store = stubStore()
			store.userOrganizationId.mockResolvedValue("org-2")
			const { auth } = buildAuth({ store })

			const result =
				await auth.options.databaseHooks.session.update.before(
					{ token: "t" },
					{
						context: {
							session: {
								user: {
									id: "user-1",
								},
							},
						},
					} as never,
				)

			expect(result).toMatchObject({
				data: { activeOrganizationId: "org-2" },
			})
		})

		it("leaves an update with no session context untouched", async ({
			expect,
		}) => {
			const store = stubStore()
			const { auth } = buildAuth({ store })

			const result =
				await auth.options.databaseHooks.session.update.before(
					{ token: "t" },
					undefined as never,
				)

			expect(result).toEqual({ data: { token: "t" } })
			expect(store.userOrganizationId).toHaveBeenCalledTimes(
				0,
			)
		})

		it("propagates a failed organization lookup", async ({
			expect,
		}) => {
			const store = stubStore()
			store.userOrganizationId.mockRejectedValue(
				new Error("connection terminated"),
			)
			const { auth } = buildAuth({ store })

			await expect(
				auth.options.databaseHooks.session.create.before(
					{
						userId: "user-1",
						token: "t",
					} as never,
				),
			).rejects.toThrow("connection terminated")
		})
	})

	describe("before hook", () => {
		// the request context as better-auth hands it to the hook, with the
		// session already resolved, which is where getSessionFromCtx reads
		// it from before it would look at any cookie.
		function changeEmailRequest(updateUser = vi.fn()) {
			return {
				path: "/change-email",
				body: {
					newEmail: "new@oxynote.test",
					password: "secret",
				},
				context: {
					session: {
						user: {
							id: "u1",
							email: "ada@oxynote.test",
						},
					},
					internalAdapter: {
						findCredentialAccount: vi
							.fn()
							.mockResolvedValue({
								password: "hash",
							}),
						findUserByEmail: vi
							.fn()
							.mockResolvedValue(
								null,
							),
						updateUser,
					},
					password: {
						verify: vi
							.fn()
							.mockResolvedValue(
								true,
							),
					},
				},
			}
		}

		it("changes the address itself once the password checks out when email is disabled", async ({
			expect,
		}) => {
			const { auth } = buildAuth({
				env: testEnv({ emailEnabled: false }),
			})
			const updateUser = vi.fn().mockResolvedValue({})

			const answer = await auth.options.hooks.before(
				changeEmailRequest(updateUser) as never,
			)

			expect(answer).toEqual({ status: true })
			expect.soft(updateUser).toHaveBeenCalledWith("u1", {
				email: "new@oxynote.test",
				emailVerified: false,
			})
		})

		it("leaves an address change to better-auth while email is enabled", async ({
			expect,
		}) => {
			const { auth } = buildAuth()
			const updateUser = vi.fn().mockResolvedValue({})

			const answer = await auth.options.hooks.before(
				changeEmailRequest(updateUser) as never,
			)

			expect(answer).toBeUndefined()
			expect.soft(updateUser).not.toHaveBeenCalled()
		})
	})

	describe("single-organization mode", () => {
		it("refuses an uninvited signup at user creation", async ({
			expect,
		}) => {
			const store = stubStore()
			store.hasPendingInvitation.mockResolvedValue(false)
			const { auth } = buildAuth({
				env: testEnv({ maxOrganizations: 1 }),
				store,
			})

			await expect(
				auth.options.databaseHooks.user.create.before(
					{ email: "a@b.c" } as never,
					{} as never,
				),
			).rejects.toMatchObject({
				body: { code: "SIGN_UP_DISABLED" },
			})
		})

		it("lets this service create a user outside a request", async ({
			expect,
		}) => {
			const store = stubStore()
			store.hasPendingInvitation.mockResolvedValue(false)
			const { auth } = buildAuth({
				env: testEnv({ maxOrganizations: 1 }),
				store,
			})

			await expect(
				auth.options.databaseHooks.user.create.before(
					{ email: "admin@example.com" } as never,
					undefined as never,
				),
			).resolves.toBeUndefined()
			expect.soft(
				store.hasPendingInvitation,
			).not.toHaveBeenCalled()
		})

		it("refuses to delete the last member's account", async ({
			expect,
		}) => {
			const store = stubStore()
			store.organizationMemberCount.mockResolvedValue(1)
			const { auth } = buildAuth({
				env: testEnv({ maxOrganizations: 1 }),
				store,
			})

			await expect(
				auth.options.hooks.before({
					path: "/delete-user",
					body: {},
					context: {
						session: {
							user: {
								id: "u1",
								email: "a@b.c",
							},
						},
					},
				} as never),
			).rejects.toMatchObject({
				body: { code: "LAST_ORGANIZATION_MEMBER" },
			})
		})

		it.for([
			{ name: "a member limit", input: 25, expected: 25 },
			// the limit is also a member list's page size in SQL
			{
				name: "no member limit",
				input: Infinity,
				expected: Number.MAX_SAFE_INTEGER,
			},
		])(
			"hands better-auth $name as a finite number",
			({ input, expected }, { expect }) => {
				const { auth } = buildAuth({
					env: testEnv({
						maxOrganizationMembers: input,
					}),
				})
				const plugin = auth.options.plugins.find(
					(p) => p.id === "organization",
				) as unknown as {
					options: { membershipLimit: number }
				}

				expect(plugin.options.membershipLimit).toBe(
					expected,
				)
			},
		)
	})

	describe("password policy", () => {
		it("requires a verified address and a long passphrase", ({
			expect,
		}) => {
			const { auth } = buildAuth()

			expect(
				auth.options.emailAndPassword
					.requireEmailVerification,
			).toBe(true)
			expect(
				auth.options.emailAndPassword.minPasswordLength,
			).toBe(16)
			// a reset happens outside any session, so every session
			// of the account goes with it
			expect(
				auth.options.emailAndPassword
					.revokeSessionsOnPasswordReset,
			).toBe(true)
		})
	})
})
