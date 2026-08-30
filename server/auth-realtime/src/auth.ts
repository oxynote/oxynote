import { betterAuth } from "better-auth"
import { createAuthMiddleware } from "better-auth/api"
import { jwt, organization } from "better-auth/plugins"
import { electron } from "@better-auth/electron"
import { mcp } from "@better-auth/mcp"
import type { PostgresDialect } from "kysely"
import type { Store } from "./db.js"
import type { CoreClient } from "./core.js"
import type { Env, SocialProviderName } from "./env.js"
import type { Logger, LogLevel } from "./logging.js"
import { reported } from "./reporting.js"

export type AuthMethod = "email-password" | "google" | "github" | "slack"

// the subset of the redis client better-auth's secondary storage uses.
// Declared structurally so a test can drive the adapter with a plain
// object.
export interface SecondaryStorageClient {
	get(key: string): Promise<string | null>
	getDel(key: string): Promise<string | null>
	incr(key: string): Promise<number>
	expire(key: string, seconds: number, mode: "NX"): Promise<unknown>
	set(
		key: string,
		value: string,
		options?: { EX: number },
	): Promise<unknown>
	del(key: string): Promise<unknown>
}

export interface AuthDeps {
	env: Env
	store: Store
	dialect: PostgresDialect
	// absent on a deployment running without valkey, which leaves
	// better-auth with no secondary storage.
	redis?: SecondaryStorageClient
	core: CoreClient
	log: Logger
}

// better-auth's own level names, which are these lowercased. Passing it
// the matching one lets it drop a message before formatting it, and it
// resolves its extra "success" level to info before calling out.
const betterAuthLevels: Record<LogLevel, "debug" | "info" | "warn" | "error"> =
	{
		DEBUG: "debug",
		INFO: "info",
		WARN: "warn",
		ERROR: "error",
	}

// the capability list served by /api/auth-config. Derived from the same
// credentials that register the social providers, so the endpoint can
// never disagree with what better-auth actually accepts. Email-password
// needs no external credentials, so it is always available.
export function authMethods(env: Env): AuthMethod[] {
	return [
		"email-password",
		...(env.socialProviders.google ? (["google"] as const) : []),
		...(env.socialProviders.github ? (["github"] as const) : []),
		...(env.socialProviders.slack ? (["slack"] as const) : []),
	]
}

// better-auth builds absolute links (email confirmation/deletion URLs)
// from its baseURL, which is the bare origin — see the baseURL comment in
// createAuth. Rewrite them onto the public prefix so emailed links survive
// the reverse proxy.
export function toPublicAuthUrl(env: Env, url: string): string {
	return url.replace(env.authOrigin, env.publicAuthBaseUrl)
}

export interface SocialSignInContext {
	path: string
	query?: Record<string, unknown> | null
	body?: Record<string, unknown> | null
}

// the @better-auth/electron plugin proxies /electron/init-oauth-proxy to
// /sign-in/social with only `{ provider }` in the body, so generateState()
// falls back to `options.baseURL` for callbackURL and the post-callback 302
// lands on the server root (404). Send it to the desktop-auth handoff page
// on the frontend instead — that page reads the auth-code cookie set on
// /callback/* and triggers the deep link back into the desktop app.
// Returns undefined for every other request, which leaves the context
// untouched.
export function electronCallbackOverride(
	ctx: SocialSignInContext,
	frontendUrl: string,
): { context: { body: Record<string, unknown> } } | undefined {
	if (
		ctx.path !== "/sign-in/social" ||
		ctx.query?.client_id !== "electron" ||
		ctx.body?.callbackURL
	) {
		return undefined
	}

	return {
		context: {
			body: {
				...ctx.body,
				callbackURL: `${frontendUrl}/desktop-auth`,
			},
		},
	}
}

export interface InvitationData {
	id: string
	email: string
	inviter: { user: { name: string } }
	organization: { id: string; name: string }
}

// the invite link the emailed button points at. Everything the
// accept-invite page needs to render before the user has a session travels
// in the query string.
export function invitationLink(env: Env, data: InvitationData): string {
	return (
		env.organizationInvitationUrl +
		`?id=${data.id}` +
		`&email=${data.email}` +
		`&inviter=${data.inviter.user.name}` +
		`&orgName=${data.organization.name}` +
		`&orgId=${data.organization.id}`
	)
}

// bind an MCP token to the user's organization at issuance, the same way
// session creation resolves activeOrganizationId, so core can scope every
// call without a per-request lookup.
export async function organizationClaims(
	store: Store,
	user: { id: string } | undefined | null,
): Promise<Record<string, string>> {
	if (!user) {
		return {}
	}

	return reported(async (): Promise<Record<string, string>> => {
		const orgId = await store.userOrganizationId(user.id)

		return orgId ? { org_id: orgId } : {}
	})
}

// the organization plugin's callbacks. Named here rather than inlined in
// the plugin's options because the plugin does not hand them back, and the
// teardown contract below is worth being able to test on its own.
export function createOrganizationHooks({
	env,
	store,
	core,
}: {
	env: Env
	store: Store
	core: CoreClient
}) {
	return {
		canCreateOrganization: () =>
			reported(async () => {
				const count =
					await store.totalOrganizationCount()

				return count < env.maxOrganizations
			}),

		afterCreateOrganization: async ({
			organization,
		}: {
			organization: { id: string }
		}) => {
			await reported(() =>
				core.initializeOrganization(organization.id),
			)
		},

		// core tears the organization's external resources down while
		// every row still exists. The throw is load-bearing:
		// better-auth aborts the deletion, so a failed teardown leaves
		// the organization alive rather than orphaning what core owns.
		beforeDeleteOrganization: async ({
			organization,
		}: {
			organization: { id: string }
		}) => {
			await reported(() =>
				core.teardownOrganization(organization.id),
			)
		},

		sendInvitationEmail: async (data: InvitationData) => {
			await reported(() =>
				core.sendEmail("organization_invitation", {
					email: data.email,
					organization: data.organization.name,
					link: invitationLink(env, data),
				}),
			)
		},
	}
}

export function createSecondaryStorage(redis: SecondaryStorageClient) {
	return {
		get: (key: string) => reported(() => redis.get(key)),
		getAndDelete: (key: string) =>
			reported(() => redis.getDel(key)),
		increment: (key: string, ttl: number) =>
			reported(async () => {
				const count = await redis.incr(key)

				// NX applies the TTL only when the key has none
				// yet, so the counter expires a fixed window
				// after creation and later increments never
				// extend it.
				await redis.expire(key, ttl, "NX")

				return count
			}),
		set: (key: string, value: string, ttl?: number) =>
			reported(async () => {
				if (ttl) {
					await redis.set(key, value, { EX: ttl })
					return
				}

				await redis.set(key, value)
			}),
		delete: (key: string) =>
			reported(async () => {
				await redis.del(key)
			}),
	}
}

export function createAuth({
	env,
	store,
	dialect,
	redis,
	core,
	log,
}: AuthDeps) {
	const organizationHooks = createOrganizationHooks({ env, store, core })

	function socialProvider(name: SocialProviderName) {
		const credentials = env.socialProviders[name]
		if (!credentials) {
			return undefined
		}

		return {
			...credentials,
			redirectURI: `${env.publicAuthBaseUrl}/api/auth/callback/${name}`,
		}
	}

	return betterAuth({
		// the public base URL carries the reverse proxy's
		// /auth-realtime prefix, but a path inside baseURL becomes
		// better-auth's basePath — and the proxy strips that prefix
		// before requests reach this service. Match on the bare origin
		// (basePath stays /api/auth) and use the full public URL only
		// where absolute, browser-reachable URLs are built (the OAuth
		// redirect URIs below).
		baseURL: env.authOrigin,
		basePath: "/api/auth",
		// better-auth prints its own lines otherwise, on a level of
		// its own; routed here they obey OXYNOTE_AUTH_REALTIME_LOG_LEVEL
		// like everything else the service writes.
		logger: {
			level: betterAuthLevels[env.logLevel],
			log: (level, message) => {
				log[level](`better-auth: ${message}`)
			},
		},
		secret: env.betterAuthSecret,
		// the dialect instance alone leaves the CLI's schema generator
		// unable to tell which database it is talking to (instanceof
		// fails across the CLI's own kysely copy), and it silently
		// falls back to sqlite typing; naming the type keeps runtime
		// behaviour identical and the generated reference schema
		// postgres-shaped.
		database: {
			dialect,
			type: "postgres",
		},
		// providers with missing credentials are left out entirely so
		// sign-in attempts fail with PROVIDER_NOT_FOUND instead of a
		// broken OAuth redirect.
		socialProviders: {
			...(env.socialProviders.slack && {
				slack: socialProvider("slack"),
			}),
			...(env.socialProviders.google && {
				google: socialProvider("google"),
			}),
			...(env.socialProviders.github && {
				github: socialProvider("github"),
			}),
		},
		emailAndPassword: {
			enabled: true,
			requireEmailVerification: true,
			minPasswordLength: 16,
			maxPasswordLength: 128,
			// a reset always happens outside an authenticated session
			// (the emailed link opens in a plain browser), so this
			// drops every session of the account — nothing survives a
			// password reset. The logged-in change-password flow
			// revokes its other sessions via revokeOtherSessions on
			// the client call instead.
			revokeSessionsOnPasswordReset: true,
			sendResetPassword: async ({ user, url }) => {
				await reported(() =>
					core.sendEmail("password_reset", {
						email: user.email,
						link: toPublicAuthUrl(env, url),
					}),
				)
			},
			// duplicate signups get better-auth's synthetic success so
			// the browser can't probe which emails have accounts. The
			// real owner is told through their inbox instead.
			onExistingUserSignUp: async ({ user }) => {
				await reported(() =>
					core.sendEmail("account_exists", {
						email: user.email,
						link: `${env.frontendUrl}/login`,
					}),
				)
			},
		},
		emailVerification: {
			sendOnSignUp: true,
			// re-send the verification link when an unverified user
			// tries to log in — the login page forwards them to the
			// check-your-inbox page, which would otherwise lie about
			// an email being sent.
			sendOnSignIn: true,
			// signup/sign-in verification gets its own
			// account-activation template; the change-email flow
			// below keeps the "new email address" one.
			sendVerificationEmail: async ({ user, url }) => {
				await reported(() =>
					core.sendEmail("signup_verification", {
						email: user.email,
						link: toPublicAuthUrl(env, url),
					}),
				)
			},
		},
		rateLimit: {
			enabled: env.rateLimitEnabled,
		},
		advanced: {
			cookiePrefix: "auth",
			crossSubDomainCookies: {
				enabled: true,
				domain: env.cookieDomain,
			},
		},
		// without valkey there is no secondary storage at all:
		// sessions are read from the database, which
		// storeSessionInDatabase keeps them in either way, and the
		// rate-limit counters fall back to better-auth's in-memory
		// storage.
		...(redis
			? { secondaryStorage: createSecondaryStorage(redis) }
			: {}),
		trustedOrigins: env.trustedOrigins,
		user: {
			modelName: "users",
			fields: {
				emailVerified: "email_verified",
				createdAt: "created_at",
				updatedAt: "updated_at",
			},
			changeEmail: {
				enabled: true,
				sendChangeEmailConfirmation: async ({
					newEmail,
					url,
				}) => {
					await reported(() =>
						core.sendEmail(
							"email_verification",
							{
								email: newEmail,
								link: toPublicAuthUrl(
									env,
									url,
								),
							},
						),
					)
				},
			},
			deleteUser: {
				enabled: true,
				sendDeleteAccountVerification: async ({
					user,
					url,
				}) => {
					await reported(() =>
						core.sendEmail(
							"user_deletion",
							{
								email: user.email,
								link: toPublicAuthUrl(
									env,
									url,
								),
							},
						),
					)
				},
			},
		},
		account: {
			modelName: "user_accounts",
			fields: {
				accountId: "account_id",
				providerId: "provider_id",
				userId: "fk_user_id",
				accessToken: "access_token",
				refreshToken: "refresh_token",
				idToken: "id_token",
				accessTokenExpiresAt: "access_token_expires_at",
				refreshTokenExpiresAt:
					"refresh_token_expires_at",
				createdAt: "created_at",
				updatedAt: "updated_at",
			},
		},
		session: {
			modelName: "user_sessions",
			// the OAuth provider (mcp plugin) refuses to run on
			// secondary-storage-only sessions, so sessions are
			// persisted to the database as well; where Valkey is
			// configured it stays the hot path for lookups.
			storeSessionInDatabase: true,
			fields: {
				expiresAt: "expires_at",
				createdAt: "created_at",
				updatedAt: "updated_at",
				ipAddress: "ip_address",
				userAgent: "user_agent",
				userId: "fk_user_id",
			},
		},
		verification: {
			modelName: "user_verifications",
			fields: {
				expiresAt: "expires_at",
				createdAt: "created_at",
				updatedAt: "updated_at",
			},
		},
		plugins: [
			electron(),
			organization({
				allowUserToCreateOrganization:
					organizationHooks.canCreateOrganization,
				disableOrganizationDeletion: true, // prevent clients from deleting the org
				organizationLimit: 1,
				membershipLimit: env.maxOrganizationMembers,
				schema: {
					session: {
						fields: {
							activeOrganizationId:
								"active_organization_id",
						},
					},
					organization: {
						modelName: "organizations",
						fields: {
							createdAt: "created_at",
						},
					},
					member: {
						modelName: "organization_members",
						fields: {
							organizationId:
								"fk_organization_id",
							userId: "fk_user_id",
							createdAt: "created_at",
						},
					},
					invitation: {
						modelName: "organization_invitations",
						fields: {
							organizationId:
								"fk_organization_id",
							expiresAt: "expires_at",
							createdAt: "created_at",
							inviterId: "fk_inviter_id",
						},
					},
				},
				organizationHooks: {
					afterCreateOrganization:
						organizationHooks.afterCreateOrganization,
					beforeDeleteOrganization:
						organizationHooks.beforeDeleteOrganization,
				},
				sendInvitationEmail:
					organizationHooks.sendInvitationEmail,
			}),
			// the mcp plugin requires jwt: MCP access tokens are
			// JWTs signed with a key from the jwks table.
			jwt({
				schema: {
					jwks: {
						fields: {
							publicKey: "public_key",
							privateKey: "private_key",
							createdAt: "created_at",
							expiresAt: "expires_at",
						},
					},
				},
			}),
			mcp({
				loginPage: `${env.frontendUrl}/login`,
				consentPage: `${env.frontendUrl}/oauth-consent`,
				resource: env.mcpResource,
				scopes: [
					"documents:read",
					"documents:write",
					"data-sources:read",
				],
				// MCP clients self-register per RFC 7591 before
				// the user has any session, so registration must
				// be open. What a client can actually do is
				// still gated by the user's consent and the
				// token scopes.
				allowDynamicClientRegistration: true,
				allowUnauthenticatedClientRegistration: true,
				customAccessTokenClaims: ({ user }) =>
					organizationClaims(store, user),
				schema: {
					oauthClient: {
						modelName: "oauth_clients",
						fields: {
							clientId: "client_id",
							clientSecret:
								"client_secret",
							clientDiscoveryId:
								"client_discovery_id",
							skipConsent:
								"skip_consent",
							enableEndSession:
								"enable_end_session",
							subjectType:
								"subject_type",
							clientCredentialsScopes:
								"client_credentials_scopes",
							userId: "fk_user_id",
							createdAt: "created_at",
							updatedAt: "updated_at",
							softwareId: "software_id",
							softwareVersion:
								"software_version",
							softwareStatement:
								"software_statement",
							redirectUris:
								"redirect_uris",
							postLogoutRedirectUris:
								"post_logout_redirect_uris",
							backchannelLogoutUri:
								"backchannel_logout_uri",
							backchannelLogoutSessionRequired:
								"backchannel_logout_session_required",
							tokenEndpointAuthMethod:
								"token_endpoint_auth_method",
							applicationType:
								"application_type",
							jwksUri: "jwks_uri",
							grantTypes: "grant_types",
							responseTypes:
								"response_types",
							requirePKCE:
								"require_pkce",
							dpopBoundAccessTokens:
								"dpop_bound_access_tokens",
							referenceId:
								"reference_id",
						},
					},
					oauthResource: {
						modelName: "oauth_resources",
						fields: {
							accessTokenTtl:
								"access_token_ttl",
							refreshTokenTtl:
								"refresh_token_ttl",
							signingAlgorithm:
								"signing_algorithm",
							signingKeyId:
								"signing_key_id",
							allowedScopes:
								"allowed_scopes",
							customClaims:
								"custom_claims",
							dpopBoundAccessTokensRequired:
								"dpop_bound_access_tokens_required",
							createdAt: "created_at",
							updatedAt: "updated_at",
							policyVersion:
								"policy_version",
						},
					},
					oauthClientResource: {
						modelName: "oauth_client_resources",
						fields: {
							clientId: "client_id",
							resourceId: "resource_id",
							createdAt: "created_at",
						},
					},
					oauthRefreshToken: {
						modelName: "oauth_refresh_tokens",
						fields: {
							clientId: "client_id",
							sessionId: "session_id",
							userId: "fk_user_id",
							referenceId:
								"reference_id",
							authorizationCodeId:
								"authorization_code_id",
							requestedUserInfoClaims:
								"requested_user_info_claims",
							expiresAt: "expires_at",
							createdAt: "created_at",
							rotatedAt: "rotated_at",
							rotationReplayResponse:
								"rotation_replay_response",
							rotationReplayExpiresAt:
								"rotation_replay_expires_at",
							authTime: "auth_time",
						},
					},
					oauthAccessToken: {
						modelName: "oauth_access_tokens",
						fields: {
							clientId: "client_id",
							sessionId: "session_id",
							userId: "fk_user_id",
							referenceId:
								"reference_id",
							authorizationCodeId:
								"authorization_code_id",
							requestedUserInfoClaims:
								"requested_user_info_claims",
							refreshId: "fk_refresh_token_id",
							expiresAt: "expires_at",
							createdAt: "created_at",
						},
					},
					oauthConsent: {
						modelName: "oauth_consents",
						fields: {
							clientId: "client_id",
							userId: "fk_user_id",
							referenceId:
								"reference_id",
							requestedUserInfoClaims:
								"requested_user_info_claims",
							createdAt: "created_at",
							updatedAt: "updated_at",
						},
					},
					oauthClientAssertion: {
						modelName: "oauth_client_assertions",
						fields: {
							expiresAt: "expires_at",
						},
					},
				},
			}),
		],
		hooks: {
			before: createAuthMiddleware((ctx) =>
				Promise.resolve(
					electronCallbackOverride(
						ctx,
						env.frontendUrl,
					),
				),
			),
		},
		databaseHooks: {
			session: {
				create: {
					before: async (session) =>
						reported(async () => {
							const orgId =
								await store.userOrganizationId(
									session.userId,
								)

							return {
								data: {
									...session,
									activeOrganizationId:
										orgId,
								},
							}
						}),
				},
				update: {
					before: async (data, ctx) =>
						reported(async () => {
							if (
								!ctx?.context
									.session
							) {
								return { data }
							}

							const orgId =
								await store.userOrganizationId(
									ctx
										.context
										.session
										.user
										.id,
								)

							return {
								data: {
									...data,
									activeOrganizationId:
										orgId,
								},
							}
						}),
				},
			},
		},
	})
}
