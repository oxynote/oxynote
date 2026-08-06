import * as Sentry from "@sentry/node"
import { betterAuth } from "better-auth"
import { createAuthMiddleware, APIError } from "better-auth/api"
import { organization } from "better-auth/plugins"
import { electron } from "@better-auth/electron"
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

const ALLOWED_EMAILS = new Set(
	(process.env.OXYNOTE_AUTH_REALTIME_ALLOWED_EMAILS || "")
		.split(",")
		.map((e) => e.trim().toLowerCase())
		.filter(Boolean),
)

const FRONTEND_URL = process.env.OXYNOTE_AUTH_REALTIME_FRONTEND_URL as string

const PUBLIC_AUTH_BASE_URL = process.env
	.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_BASE_URL as string

const AUTH_ORIGIN = new URL(PUBLIC_AUTH_BASE_URL).origin

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

// the capability list served by /api/auth-methods. Derived from the same
// flags that register the social providers below, so the endpoint can never
// disagree with what better-auth actually accepts.
export const AUTH_METHODS = [
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

function isEmailAllowed(email: string): boolean {
	if (ALLOWED_EMAILS.size === 0) {
		return true
	}

	return ALLOWED_EMAILS.has(email.toLowerCase())
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
	database: dialect,
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
				await sendEmailVerification(
					newEmail,
					toPublicAuthURL(url),
				)
			},
		},
		deleteUser: {
			enabled: true,
			sendDeleteAccountVerification: async ({
				user,
				url,
			}) => {
				await sendUserDeletionConfirmation(
					user.email,
					toPublicAuthURL(url),
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
			refreshTokenExpiresAt: "refresh_token_expires_at",
			createdAt: "created_at",
			updatedAt: "updated_at",
		},
	},
	session: {
		modelName: "user_sessions",
		storeSessionInDatabase: false,
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

				await sendOrganizationInvitation(
					data.email,
					data.organization.name,
					inviteLink,
				)
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
		after: createAuthMiddleware(async (ctx) => {
			if (!ctx.path.startsWith("/callback/")) {
				return
			}

			const setCookie =
				ctx.context.responseHeaders?.get("set-cookie")
			if (!setCookie) {
				return
			}

			const match = setCookie.match(
				/auth\.session_token=([^;,]+)/,
			)
			if (!match?.[1]) {
				return
			}

			const signedToken = decodeURIComponent(match[1])
			const token = signedToken.substring(
				0,
				signedToken.lastIndexOf("."),
			)
			if (!token) {
				return
			}

			const sessionData = await redisClient.get(token)
			if (!sessionData) {
				return
			}

			const { user } = JSON.parse(sessionData)
			if (!user) {
				return
			}

			if (!isEmailAllowed(user.email)) {
				await redisClient.del(token)

				const isNewUser =
					Date.now() -
						new Date(
							user.createdAt,
						).getTime() <
					10_000

				const errorCode = isNewUser
					? "auth.registration_not_allowed"
					: "auth.login_not_allowed"

				const redirectHeaders = new Headers()
				redirectHeaders.set(
					"location",
					`${FRONTEND_URL}/login?error=${errorCode}`,
				)
				throw new APIError(
					"FOUND",
					undefined,
					redirectHeaders,
				)
			}
		}),
	},
	databaseHooks: {
		user: {
			create: {
				after: async (user) => {
					await sendUserCreation(user.email)
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

async function sendEmailVerification(
	email: string,
	link: string,
): Promise<void> {
	try {
		await axios.post(`${backendUrl}/api/x/email/verification`, {
			email,
			link,
		})
	} catch (err) {
		Sentry.captureException(err)
		throw err
	}
}

async function sendOrganizationInvitation(
	email: string,
	organization: string,
	link: string,
): Promise<void> {
	try {
		await axios.post(
			`${backendUrl}/api/x/email/organization-invitation`,
			{
				email,
				organization,
				link,
			},
		)
	} catch (err) {
		Sentry.captureException(err)
		throw err
	}
}

async function sendUserDeletionConfirmation(
	email: string,
	link: string,
): Promise<void> {
	try {
		await axios.post(`${backendUrl}/api/x/email/user-deletion`, {
			email,
			link,
		})
	} catch (err) {
		Sentry.captureException(err)
		throw err
	}
}

async function sendUserCreation(email: string): Promise<void> {
	try {
		await axios.post(`${backendUrl}/api/x/email/user-creation`, {
			email,
		})
	} catch (err) {
		Sentry.captureException(err)
		throw err
	}
}
