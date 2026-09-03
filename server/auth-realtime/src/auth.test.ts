import { describe, it, vi } from "vitest"
import {
	authMethods,
	createAuth,
	createOrganizationHooks,
	createSecondaryStorage,
	electronCallbackOverride,
	invitationLink,
	organizationClaims,
	toPublicAuthUrl,
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

			await auth.options.emailAndPassword.sendResetPassword({
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

			await auth.options.emailVerification.sendVerificationEmail(
				{
					user: { email: "a@b.c" },
					url: "http://localhost:8080/api/auth/verify/tok",
					token: "tok",
				} as never,
			)

			expect(core.sendEmail).toHaveBeenCalledWith(
				"signup_verification",
				{
					email: "a@b.c",
					link: "http://localhost:8080/auth-realtime/api/auth/verify/tok",
				},
			)
		})

		it("confirms a change of address at the new address", async ({
			expect,
		}) => {
			const { auth, core } = buildAuth()

			await auth.options.user.changeEmail.sendChangeEmailConfirmation(
				{
					user: { email: "old@b.c" },
					newEmail: "new@b.c",
					url: "http://localhost:8080/api/auth/change/tok",
					token: "tok",
				} as never,
			)

			expect(core.sendEmail).toHaveBeenCalledWith(
				"email_verification",
				{
					email: "new@b.c",
					link: "http://localhost:8080/auth-realtime/api/auth/change/tok",
				},
			)
		})

		it("confirms an account deletion by email", async ({
			expect,
		}) => {
			const { auth, core } = buildAuth()

			await auth.options.user.deleteUser.sendDeleteAccountVerification(
				{
					user: { email: "a@b.c" },
					url: "http://localhost:8080/api/auth/delete/tok",
					token: "tok",
				} as never,
			)

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
				auth.options.emailAndPassword.sendResetPassword(
					{
						user: { email: "a@b.c" },
						url: "http://localhost:8080/api/auth/reset/tok",
						token: "tok",
					} as never,
				),
			).rejects.toBe(failure)
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
