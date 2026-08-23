import * as Sentry from "@sentry/node"
import { betterAuth } from "better-auth"
import { createAuthMiddleware } from "better-auth/api"
import { jwt, organization } from "better-auth/plugins"
import { electron } from "@better-auth/electron"
import { mcp } from "@better-auth/mcp"
import { createClient } from "redis"
import { db, dialect } from "./database.js"
import axios from "axios"

const MAX_ORGANIZATION_MEMBERS = parseInt(
	process.env.OXYNOTE_AUTH_REALTIME_MAX_ORGANIZATION_MEMBERS || "5",
)
export const MAX_ORGANIZATIONS = parseInt(
	process.env.OXYNOTE_AUTH_REALTIME_MAX_ORGANIZATIONS || "100",
)
const backendUrl = process.env.OXYNOTE_AUTH_REALTIME_BACKEND_URL

const FRONTEND_URL = process.env.OXYNOTE_AUTH_REALTIME_FRONTEND_URL as string

const PUBLIC_AUTH_BASE_URL = process.env
	.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_BASE_URL as string

const AUTH_ORIGIN = new URL(PUBLIC_AUTH_BASE_URL).origin

// the canonical MCP protected-resource URL (RFC 8707/9728) — core's MCP
// endpoint as clients reach it through the front door. Issued access tokens
// are audience-bound to it.
export const MCP_RESOURCE = process.env
	.OXYNOTE_AUTH_REALTIME_MCP_RESOURCE as string

// MCP access tokens carry better-auth's resolved base URL (origin + basePath)
// as their issuer. The internal MCP session endpoint verifies against it.
export const MCP_TOKEN_ISSUER = `${AUTH_ORIGIN}/api/auth`

// better-auth's limiter buckets by client IP, and sign-in and sign-up
// share one allowance of three requests per ten seconds. That is wrong
// for any caller whose traffic arrives from a single address on behalf
// of many users — an end-to-end test run, or a deployment fronted by a
// proxy that does not pass the original IP through — so it can be
// switched off and left to whatever sits in front.
const RATE_LIMIT_DISABLED =
	process.env.OXYNOTE_AUTH_REALTIME_RATE_LIMIT_DISABLED === "true"

const GOOGLE_CONFIGURED = Boolean(
	process.env.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GOOGLE_CLIENT_ID &&
		process.env
			.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GOOGLE_CLIENT_SECRET,
)
const GITHUB_CONFIGURED = Boolean(
	process.env.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GITHUB_CLIENT_ID &&
		process.env
			.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GITHUB_CLIENT_SECRET,
)
const SLACK_CONFIGURED = Boolean(
	process.env.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SLACK_CLIENT_ID &&
		process.env
			.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SLACK_CLIENT_SECRET,
)

// the capability list served by /api/auth-config. Derived from the same
// flags that register the social providers below, so the endpoint can never
// disagree with what better-auth actually accepts. Email-password needs no
// external credentials, so it is always available.
export const AUTH_METHODS = [
	"email-password" as const,
	...(GOOGLE_CONFIGURED ? ["google" as const] : []),
	...(GITHUB_CONFIGURED ? ["github" as const] : []),
	...(SLACK_CONFIGURED ? ["slack" as const] : []),
]

// better-auth builds absolute links (email confirmation/deletion URLs) from
// its baseURL, which is the bare origin — see the baseURL comment below.
// Rewrite them onto the public prefix so emailed links survive the reverse
// proxy.
function toPublicAuthURL(url: string): string {
	return url.replace(AUTH_ORIGIN, PUBLIC_AUTH_BASE_URL)
}

const redisClient = createClient({
	url: process.env.OXYNOTE_AUTH_REALTIME_VALKEY_URL,
})

await redisClient.connect()

export const auth = betterAuth({
	// the public base URL carries the reverse proxy's /auth-realtime prefix,
	// but a path inside baseURL becomes better-auth's basePath — and the
	// proxy strips that prefix before requests reach this service. Match on
	// the bare origin (basePath stays /api/auth) and use the full public URL
	// only where absolute, browser-reachable URLs are built (the OAuth
	// redirect URIs below).
	baseURL: AUTH_ORIGIN,
	basePath: "/api/auth",
	secret: process.env.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SECRET,
	// the dialect instance alone leaves the CLI's schema generator unable
	// to tell which database it is talking to (instanceof fails across
	// the CLI's own kysely copy), and it silently falls back to sqlite
	// typing; naming the type keeps runtime behaviour identical and the
	// generated reference schema postgres-shaped.
	database: {
		dialect,
		type: "postgres",
	},
	// providers with missing credentials are left out entirely so sign-in
	// attempts fail with PROVIDER_NOT_FOUND instead of a broken OAuth
	// redirect.
	socialProviders: {
		...(SLACK_CONFIGURED && {
			slack: {
				clientId: process.env
					.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SLACK_CLIENT_ID as string,
				clientSecret: process.env
					.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SLACK_CLIENT_SECRET as string,
				redirectURI:
					PUBLIC_AUTH_BASE_URL +
					"/api/auth/callback/slack",
			},
		}),
		...(GOOGLE_CONFIGURED && {
			google: {
				clientId: process.env
					.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GOOGLE_CLIENT_ID as string,
				clientSecret: process.env
					.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GOOGLE_CLIENT_SECRET as string,
				redirectURI:
					PUBLIC_AUTH_BASE_URL +
					"/api/auth/callback/google",
			},
		}),
		...(GITHUB_CONFIGURED && {
			github: {
				clientId: process.env
					.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GITHUB_CLIENT_ID as string,
				clientSecret: process.env
					.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GITHUB_CLIENT_SECRET as string,
				redirectURI:
					PUBLIC_AUTH_BASE_URL +
					"/api/auth/callback/github",
			},
		}),
	},
	emailAndPassword: {
		enabled: true,
		requireEmailVerification: true,
		minPasswordLength: 16,
		maxPasswordLength: 128,
		// a reset always happens outside an authenticated session (the
		// emailed link opens in a plain browser), so this drops every
		// session of the account — nothing survives a password reset.
		// The logged-in change-password flow revokes its other sessions
		// via revokeOtherSessions on the client call instead.
		revokeSessionsOnPasswordReset: true,
		sendResetPassword: async ({ user, url }) => {
			await sendEmail("password_reset", {
				email: user.email,
				link: toPublicAuthURL(url),
			})
		},
		// duplicate signups get better-auth's synthetic success so the
		// browser can't probe which emails have accounts. The real
		// owner is told through their inbox instead.
		onExistingUserSignUp: async ({ user }) => {
			await sendEmail("account_exists", {
				email: user.email,
				link: `${FRONTEND_URL}/login`,
			})
		},
	},
	emailVerification: {
		sendOnSignUp: true,
		// re-send the verification link when an unverified user tries to
		// log in — the login page forwards them to the check-your-inbox
		// page, which would otherwise lie about an email being sent.
		sendOnSignIn: true,
		// signup/sign-in verification gets its own account-activation
		// template; the change-email flow below keeps the "new email
		// address" one.
		sendVerificationEmail: async ({ user, url }) => {
			await sendEmail("signup_verification", {
				email: user.email,
				link: toPublicAuthURL(url),
			})
		},
	},
	rateLimit: {
		enabled: !RATE_LIMIT_DISABLED,
	},
	advanced: {
		cookiePrefix: "auth",
		crossSubDomainCookies: {
			enabled: true,
			domain: process.env
				.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_COOKIE_DOMAIN as string,
		},
	},
	secondaryStorage: {
		get: async (key) => {
			try {
				return await redisClient.get(key)
			} catch (err) {
				Sentry.captureException(err)
				throw err
			}
		},
		getAndDelete: async (key) => {
			try {
				return await redisClient.getDel(key)
			} catch (err) {
				Sentry.captureException(err)
				throw err
			}
		},
		increment: async (key, ttl) => {
			try {
				const count = await redisClient.incr(key)

				// NX applies the TTL only when the key has none yet, so
				// the counter expires a fixed window after creation and
				// later increments never extend it.
				await redisClient.expire(key, ttl, "NX")

				return count
			} catch (err) {
				Sentry.captureException(err)
				throw err
			}
		},
		set: async (key, value, ttl) => {
			try {
				if (ttl) {
					await redisClient.set(key, value, {
						EX: ttl,
					})
					return
				}

				await redisClient.set(key, value)
			} catch (err) {
				Sentry.captureException(err)
				throw err
			}
		},
		delete: async (key) => {
			try {
				await redisClient.del(key)
			} catch (err) {
				Sentry.captureException(err)
				throw err
			}
		},
	},
	trustedOrigins: (
		(process.env.OXYNOTE_AUTH_REALTIME_TRUSTED_ORIGINS ||
			"") as string
	).split(","),
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
				await sendEmail("email_verification", {
					email: newEmail,
					link: toPublicAuthURL(url),
				})
			},
		},
		deleteUser: {
			enabled: true,
			sendDeleteAccountVerification: async ({
				user,
				url,
			}) => {
				await sendEmail("user_deletion", {
					email: user.email,
					link: toPublicAuthURL(url),
				})
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
			refreshTokenExpiresAt: "refresh_token_expires_at",
			createdAt: "created_at",
			updatedAt: "updated_at",
		},
	},
	session: {
		modelName: "user_sessions",
		// the OAuth provider (mcp plugin) refuses to run on
		// secondary-storage-only sessions, so sessions are persisted to
		// the database as well; Valkey stays the hot path for lookups.
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
			allowUserToCreateOrganization: async () => {
				try {
					const count =
						await totalOrganizationCount()
					return count < MAX_ORGANIZATIONS
				} catch (err) {
					Sentry.captureException(err)
					throw err
				}
			},
			disableOrganizationDeletion: true, // prevent clients from deleting the org
			organizationLimit: 1,
			membershipLimit: MAX_ORGANIZATION_MEMBERS,
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
				afterCreateOrganization: async ({
					organization,
				}) => {
					await initializeOrganization(
						organization.id,
					)
				},
				beforeDeleteOrganization: async ({
					organization,
				}) => {
					await teardownOrganization(
						organization.id,
					)
				},
			},
			sendInvitationEmail: async (data) => {
				const inviteLink =
					process.env
						.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_ORGANIZATION_INVITATION_URL +
					`?id=${data.id}` +
					`&email=${data.email}` +
					`&inviter=${data.inviter.user.name}` +
					`&orgName=${data.organization.name}` +
					`&orgId=${data.organization.id}`

				await sendEmail("organization_invitation", {
					email: data.email,
					organization: data.organization.name,
					link: inviteLink,
				})
			},
		}),
		// the mcp plugin requires jwt: MCP access tokens are JWTs signed
		// with a key from the jwks table.
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
			loginPage: `${FRONTEND_URL}/login`,
			consentPage: `${FRONTEND_URL}/oauth-consent`,
			resource: MCP_RESOURCE,
			scopes: [
				"documents:read",
				"documents:write",
				"data-sources:read",
			],
			// MCP clients self-register per RFC 7591 before the user has
			// any session, so registration must be open. What a client
			// can actually do is still gated by the user's consent and
			// the token scopes.
			allowDynamicClientRegistration: true,
			allowUnauthenticatedClientRegistration: true,
			// bind the token to the user's organization at issuance the
			// same way session creation resolves activeOrganizationId, so
			// core can scope every MCP call without a per-request lookup.
			customAccessTokenClaims: async ({ user }) => {
				if (!user) {
					return {}
				}

				try {
					const orgId = await userOrganizationId(
						user.id,
					)

					return orgId ? { org_id: orgId } : {}
				} catch (err) {
					Sentry.captureException(err)
					throw err
				}
			},
			schema: {
				oauthClient: {
					modelName: "oauth_clients",
					fields: {
						clientId: "client_id",
						clientSecret: "client_secret",
						clientDiscoveryId:
							"client_discovery_id",
						skipConsent: "skip_consent",
						enableEndSession:
							"enable_end_session",
						subjectType: "subject_type",
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
						redirectUris: "redirect_uris",
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
						responseTypes: "response_types",
						requirePKCE: "require_pkce",
						dpopBoundAccessTokens:
							"dpop_bound_access_tokens",
						referenceId: "reference_id",
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
						signingKeyId: "signing_key_id",
						allowedScopes: "allowed_scopes",
						customClaims: "custom_claims",
						dpopBoundAccessTokensRequired:
							"dpop_bound_access_tokens_required",
						createdAt: "created_at",
						updatedAt: "updated_at",
						policyVersion: "policy_version",
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
						referenceId: "reference_id",
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
						referenceId: "reference_id",
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
						referenceId: "reference_id",
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
		before: createAuthMiddleware(async (ctx) => {
			// The @better-auth/electron plugin proxies /electron/init-oauth-proxy
			// to /sign-in/social with only `{ provider }` in the body, so
			// generateState() falls back to `options.baseURL` for callbackURL and
			// the post-callback 302 lands on the server root (404). Send it to
			// the desktop-auth handoff page on the frontend instead — that page
			// reads the auth-code cookie set on /callback/* and triggers the
			// deep link back into the desktop app.
			if (
				ctx.path === "/sign-in/social" &&
				ctx.query?.client_id === "electron" &&
				!ctx.body?.callbackURL
			) {
				return {
					context: {
						body: {
							...ctx.body,
							callbackURL: `${FRONTEND_URL}/desktop-auth`,
						},
					},
				}
			}
		}),
	},
	databaseHooks: {
		user: {
			create: {
				after: async (user) => {
					await sendEmail("user_creation", {
						email: user.email,
					})
				},
			},
		},
		session: {
			create: {
				before: async (session) => {
					try {
						const orgId =
							await userOrganizationId(
								session.userId,
							)
						return {
							data: {
								...session,
								activeOrganizationId:
									orgId,
							},
						}
					} catch (err) {
						Sentry.captureException(err)
						throw err
					}
				},
			},
			update: {
				before: async (data, ctx) => {
					try {
						if (ctx?.context.session) {
							const orgId =
								await userOrganizationId(
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
						}

						return { data }
					} catch (err) {
						Sentry.captureException(err)
						throw err
					}
				},
			},
		},
	},
})

async function userOrganizationId(userId: string): Promise<string | null> {
	const res = await db
		.selectFrom("organization_members")
		.where("fk_user_id", "=", userId)
		.select("fk_organization_id")
		.executeTakeFirst()

	return res ? res.fk_organization_id : null
}

export async function totalOrganizationCount(): Promise<number> {
	const res = await db
		.selectFrom("organizations")
		.select(db.fn.countAll<number>().as("count"))
		.executeTakeFirstOrThrow()

	return res.count
}

async function initializeOrganization(organizationId: string): Promise<void> {
	try {
		await axios.post(
			`${backendUrl}/api/x/organizations/${organizationId}/initialize`,
		)
	} catch (err) {
		Sentry.captureException(err)
		throw err
	}
}

async function teardownOrganization(organizationId: string): Promise<void> {
	try {
		await axios.post(
			`${backendUrl}/api/x/organizations/${organizationId}/teardown`,
		)
	} catch (err) {
		Sentry.captureException(err)
		throw err
	}
}

async function sendEmail(
	template:
		| "email_verification"
		| "password_reset"
		| "account_exists"
		| "signup_verification"
		| "organization_invitation"
		| "user_deletion"
		| "user_creation",
	data: Record<string, string>,
): Promise<void> {
	try {
		await axios.post(`${backendUrl}/api/x/email`, {
			template,
			data,
		})
	} catch (err) {
		Sentry.captureException(err)
		throw err
	}
}
