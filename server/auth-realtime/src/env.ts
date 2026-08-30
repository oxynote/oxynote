import { z } from "zod"

import type { LogLevel } from "./logging.js"

export type SocialProviderName = "google" | "github" | "slack"

export interface SocialProviderCredentials {
	clientId: string
	clientSecret: string
}

export interface Env {
	// where core answers. The variable behind it is still spelled
	// BACKEND_URL because that name is baked into every deployment's env
	// file; only the field this side reads it through says "core".
	coreUrl: string
	databaseDSN: string
	// absent on a deployment running without valkey, where better-auth
	// keeps no secondary storage at all.
	valkeyDsn: string | undefined
	// the interface the server binds to; undefined binds all interfaces.
	listenHost: string | undefined
	listenPort: number
	// the public base URL carries the reverse proxy's /auth-realtime
	// prefix; authOrigin is the bare origin the proxy forwards to.
	publicAuthBaseUrl: string
	authOrigin: string
	betterAuthSecret: string
	cookieDomain: string
	frontendUrl: string
	organizationInvitationUrl: string
	trustedOrigins: string[]
	mcpResource: string
	// MCP access tokens carry better-auth's resolved base URL (origin +
	// basePath) as their issuer. The internal MCP session endpoint
	// verifies against it.
	mcpTokenIssuer: string
	socialProviders: Partial<
		Record<SocialProviderName, SocialProviderCredentials>
	>
	maxOrganizations: number
	maxOrganizationMembers: number
	rateLimitEnabled: boolean
	logLevel: LogLevel
}

// every URL here is one something dials or redirects a browser to, so the
// scheme is pinned: bare z.url() accepts "core:8080" as a valid URL with a
// "core:" scheme, which would boot fine and then fail on first use.
function httpUrl() {
	return z.url({ protocol: /^https?$/ })
}

// an unset counter falls back to its default; a present one must be a
// positive integer, so a typo is a boot error instead of a NaN limit that
// compares false against every count.
function counter(fallback: number) {
	return z
		.string()
		.optional()
		.transform((value) =>
			value === undefined ? fallback : Number(value),
		)
		.pipe(z.number().int().positive())
}

// an unset address falls back to all interfaces on the default port; a
// present one must be host:port (the host part may be empty), so a typo is
// a boot error instead of a server listening on the wrong interface.
function listenAddress(fallbackPort: number) {
	return z
		.string()
		.regex(/^[^:]*:\d+$/, "must be host:port")
		.optional()
		.transform((value): { host?: string; port: number } => {
			if (value === undefined) {
				return { host: undefined, port: fallbackPort }
			}

			const colon = value.lastIndexOf(":")

			return {
				host: value.slice(0, colon) || undefined,
				port: Number(value.slice(colon + 1)),
			}
		})
		.pipe(
			z.object({
				host: z.string().optional(),
				port: z.number().int().positive(),
			}),
		)
}

// only the two literals are accepted: a flag spelled "1" or "yes" would
// otherwise read as false and silently leave the setting at its default.
function flag(fallback: boolean) {
	return z
		.enum(["true", "false"])
		.optional()
		.transform((value) =>
			value === undefined ? fallback : value === "true",
		)
}

const SOCIAL_PROVIDERS = [
	{
		name: "google",
		idKey: "OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GOOGLE_CLIENT_ID",
		secretKey: "OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GOOGLE_CLIENT_SECRET",
	},
	{
		name: "github",
		idKey: "OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GITHUB_CLIENT_ID",
		secretKey: "OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GITHUB_CLIENT_SECRET",
	},
	{
		name: "slack",
		idKey: "OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SLACK_CLIENT_ID",
		secretKey: "OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SLACK_CLIENT_SECRET",
	},
] as const satisfies readonly {
	name: SocialProviderName
	idKey: string
	secretKey: string
}[]

// every variable the service reads, spelled out. Only the social
// credentials and the valkey URL are allowed to be absent — a provider
// with neither half is left unregistered, and one with a single half is a
// boot error rather than a broken OAuth redirect discovered by a user.
const schema = z
	.object({
		OXYNOTE_AUTH_REALTIME_BACKEND_URL: httpUrl(),
		OXYNOTE_AUTH_REALTIME_DB_DSN: z.string().min(1),
		OXYNOTE_AUTH_REALTIME_VALKEY_DSN: z.string().min(1).optional(),
		OXYNOTE_AUTH_REALTIME_ADDRESS: listenAddress(8081),
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_BASE_URL: httpUrl(),
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SECRET: z.string().min(1),
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_COOKIE_DOMAIN: z
			.string()
			.min(1),
		OXYNOTE_AUTH_REALTIME_FRONTEND_URL: httpUrl(),
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_ORGANIZATION_INVITATION_URL:
			httpUrl(),
		OXYNOTE_AUTH_REALTIME_TRUSTED_ORIGINS: z.string().optional(),
		OXYNOTE_AUTH_REALTIME_MCP_RESOURCE: httpUrl(),
		OXYNOTE_AUTH_REALTIME_MAX_ORGANIZATIONS: counter(100),
		OXYNOTE_AUTH_REALTIME_MAX_ORGANIZATION_MEMBERS: counter(5),
		// better-auth's limiter buckets by client IP, and sign-in and
		// sign-up share one allowance of three requests per ten
		// seconds. That is wrong for any caller whose traffic arrives
		// from a single address on behalf of many users — an
		// end-to-end test run, or a deployment fronted by a proxy that
		// does not pass the original IP through — so it can be
		// switched off and left to whatever sits in front.
		OXYNOTE_AUTH_REALTIME_RATE_LIMIT_DISABLED: flag(false),
		// the floor for everything this service writes, better-auth's
		// and hocuspocus's output included. INFO matches core's own
		// default; the production image lowers both to WARN.
		OXYNOTE_AUTH_REALTIME_LOG_LEVEL: z
			.enum(["DEBUG", "INFO", "WARN", "ERROR"])
			.optional()
			.transform((value) => value ?? "INFO"),
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GOOGLE_CLIENT_ID: z
			.string()
			.optional(),
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GOOGLE_CLIENT_SECRET: z
			.string()
			.optional(),
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GITHUB_CLIENT_ID: z
			.string()
			.optional(),
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GITHUB_CLIENT_SECRET: z
			.string()
			.optional(),
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SLACK_CLIENT_ID: z
			.string()
			.optional(),
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SLACK_CLIENT_SECRET: z
			.string()
			.optional(),
	})
	.superRefine((values, ctx) => {
		for (const provider of SOCIAL_PROVIDERS) {
			const id = values[provider.idKey]
			const secret = values[provider.secretKey]

			if (Boolean(id) === Boolean(secret)) {
				continue
			}

			ctx.addIssue({
				code: "custom",
				path: [
					id
						? provider.secretKey
						: provider.idKey,
				],
				message: `${provider.name} needs both a client id and a client secret, or neither`,
			})
		}
	})

type ParsedEnv = z.infer<typeof schema>

function socialProviders(
	values: ParsedEnv,
): Partial<Record<SocialProviderName, SocialProviderCredentials>> {
	const providers: Partial<
		Record<SocialProviderName, SocialProviderCredentials>
	> = {}

	for (const provider of SOCIAL_PROVIDERS) {
		const clientId = values[provider.idKey]
		const clientSecret = values[provider.secretKey]

		if (!clientId || !clientSecret) {
			continue
		}

		providers[provider.name] = { clientId, clientSecret }
	}

	return providers
}

// loadEnv parses the process environment into the typed configuration the
// composition root hands to every factory. It is the only place that reads
// process.env, so a test builds an Env by calling this with a literal
// record instead of mutating the real environment.
export function loadEnv(source: Record<string, string | undefined>): Env {
	// docker's env files list every variable the service reads, including
	// the ones left blank, so an unset value arrives as "" rather than
	// absent. Dropping those lets optional values fall back to their
	// defaults and makes a required one report as missing instead of
	// empty.
	const present = Object.fromEntries(
		Object.entries(source).filter(
			([, value]) => value !== undefined && value !== "",
		),
	)

	const result = schema.safeParse(present)
	if (!result.success) {
		throw new Error(
			`invalid environment:\n${z.prettifyError(result.error)}`,
		)
	}

	const values = result.data
	const publicAuthBaseUrl =
		values.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_BASE_URL
	const authOrigin = new URL(publicAuthBaseUrl).origin

	return {
		coreUrl: values.OXYNOTE_AUTH_REALTIME_BACKEND_URL,
		databaseDSN: values.OXYNOTE_AUTH_REALTIME_DB_DSN,
		valkeyDsn: values.OXYNOTE_AUTH_REALTIME_VALKEY_DSN,
		listenHost: values.OXYNOTE_AUTH_REALTIME_ADDRESS.host,
		listenPort: values.OXYNOTE_AUTH_REALTIME_ADDRESS.port,
		publicAuthBaseUrl,
		authOrigin,
		betterAuthSecret:
			values.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SECRET,
		cookieDomain:
			values.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_COOKIE_DOMAIN,
		frontendUrl: values.OXYNOTE_AUTH_REALTIME_FRONTEND_URL,
		organizationInvitationUrl:
			values.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_ORGANIZATION_INVITATION_URL,
		trustedOrigins: (
			values.OXYNOTE_AUTH_REALTIME_TRUSTED_ORIGINS ?? ""
		)
			.split(",")
			.filter(Boolean),
		mcpResource: values.OXYNOTE_AUTH_REALTIME_MCP_RESOURCE,
		mcpTokenIssuer: `${authOrigin}/api/auth`,
		socialProviders: socialProviders(values),
		maxOrganizations:
			values.OXYNOTE_AUTH_REALTIME_MAX_ORGANIZATIONS,
		maxOrganizationMembers:
			values.OXYNOTE_AUTH_REALTIME_MAX_ORGANIZATION_MEMBERS,
		rateLimitEnabled:
			!values.OXYNOTE_AUTH_REALTIME_RATE_LIMIT_DISABLED,
		logLevel: values.OXYNOTE_AUTH_REALTIME_LOG_LEVEL,
	}
}
