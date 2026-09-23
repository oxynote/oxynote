import { z } from "zod"

export interface ObjectStorageConfig {
	url: string
	accessKey: string
	secretKey: string
	bucket: string
	region: string
}

const tlsModes = ["none", "starttls", "tls"] as const

type TlsMode = (typeof tlsModes)[number]

function isTlsMode(value: string): value is TlsMode {
	return (tlsModes as readonly string[]).includes(value)
}

export interface SmtpConfig {
	host: string
	port: string
	username: string
	password: string
	tls: TlsMode
}

export interface OAuthCredentials {
	clientId: string
	clientSecret: string
}

export interface GitHubAppConfig {
	appId: string
	appSlug: string
	signatureSecret: string
}

export interface SlackAppConfig {
	clientId: string
	clientSecret: string
	signatureSecret: string
}

export type SocialLoginProviderName = "github" | "google" | "slack"

export interface Config {
	// the public origin, no trailing slash; every public URL the
	// components need derives from it.
	publicOrigin: string
	// host[:port] — core's origin allowlist matches this bare form for
	// WebSocket upgrades.
	publicHostPort: string
	// the bare hostname, used as the session cookie domain.
	cookieDomain: string
	// the ws(s) counterpart of publicOrigin.
	publicWebSocketOrigin: string
	// absent unless the operator points the image at an external
	// PostgreSQL; the image then runs its own on the data volume.
	databaseDsn: string | undefined
	// absent on a deployment running without valkey: the assistant keeps
	// its conversations in the core process and better-auth keeps no
	// secondary storage.
	valkeyDsn: string | undefined
	// absent on a deployment without an object store: objects are kept on
	// the data volume instead.
	objectStorage: ObjectStorageConfig | undefined
	smtp: SmtpConfig | undefined
	emailFromAddress: string
	changeDetection: { url: string; apiKey: string } | undefined
	githubApp: GitHubAppConfig | undefined
	slackApp: SlackAppConfig | undefined
	socialLogin: Partial<Record<SocialLoginProviderName, OAuthCredentials>>
	// the assistant family is passed through to core untouched, keyed by
	// the OXYNOTE_AI_ASSISTANT_ suffix; core owns its deep validation.
	aiAssistant: Record<string, string>
	maxOrganizations: string
	maxOrganizationMembers: string
	rateLimitDisabled: boolean
	maxDocumentHistoryEntries: string
	documentHistoryRetention: string
	logLevel: string
	termsOfServiceUrl: string
	privacyPolicyUrl: string
	crashReportingDisabled: boolean
	// explicit secret overrides; unset means generate-and-persist.
	authSecret: string | undefined
	dataSourceEncryptionKeys: string | undefined
}

// every URL here is dialed or redirected to, so the scheme is pinned: bare
// z.url() accepts "core:8080" as a valid URL with a "core:" scheme, which
// would boot fine and then fail on first use.
function httpUrl() {
	return z.url({ protocol: /^https?$/ })
}

// only the two literals are accepted: a flag spelled "1" or "yes" would
// otherwise read as false and silently leave the setting at its default.
function flag() {
	return z
		.enum(["true", "false"])
		.optional()
		.transform((value) => value === "true")
}

// a whole number kept as the string the component parses itself.
function count() {
	return z.string().regex(/^\d+$/, "must be a whole number").optional()
}

// a limit kept as the string auth-realtime parses itself: -1 for none, or a
// positive integer.
function limit() {
	return z
		.string()
		.regex(/^(-1|[1-9]\d*)$/, "must be -1 or a positive integer")
		.optional()
}

// what the public origin falls back to. Caddy listens on 8080 inside the
// container, so a container published as 8080:8080 is reachable here with
// no variable set; anything else (a domain, another host port) needs
// OXYNOTE_PUBLIC_URL.
const defaultPublicUrl = "http://localhost:8080"

// the public origin every URL derives from. A path would silently break the
// caddy routing, which is root-anchored.
const publicOrigin = httpUrl().refine(
	(value) => {
		// zod runs this check even when the base URL check already
		// failed, so the parse cannot be assumed to succeed.
		try {
			const url = new URL(value)

			return url.pathname === "/" && !url.search && !url.hash
		} catch {
			return false
		}
	},
	{ error: "must be an origin without a path" },
)

// both components take the URL verbatim: node-redis parses it natively and
// core hands it to redigo's URL dialer, so credentials and a database index
// are supported end to end.
const valkeyDsn = z
	.string()
	.min(1)
	.refine(
		(value) => {
			try {
				const url = new URL(value)

				return (
					url.protocol === "redis:" ||
					url.protocol === "rediss:"
				)
			} catch {
				return false
			}
		},
		{ error: "must be a redis:// or rediss:// URL" },
	)

// one DSN carrying everything the S3 client needs:
// http(s)://ACCESS_KEY:SECRET_KEY@host:port/bucket[?region=...]
const objectStorageDsn = z
	.string()
	.min(1)
	.transform((value, ctx): ObjectStorageConfig => {
		let url: URL

		try {
			url = new URL(value)
		} catch {
			ctx.addIssue({
				code: "custom",
				message: "must be a URL",
			})

			return z.NEVER
		}

		if (url.protocol !== "http:" && url.protocol !== "https:") {
			ctx.addIssue({
				code: "custom",
				message: "must use http or https",
			})

			return z.NEVER
		}

		if (!url.username || !url.password) {
			ctx.addIssue({
				code: "custom",
				message: "must carry the access key and secret key as URL credentials",
			})

			return z.NEVER
		}

		const bucket = url.pathname.replaceAll("/", "")

		return {
			url: url.origin,
			accessKey: decodeURIComponent(url.username),
			secretKey: decodeURIComponent(url.password),
			bucket: bucket || "oxynote",
			region: url.searchParams.get("region") ?? "",
		}
	})

// one URL carrying the relay's coordinates:
// smtp[s]://[user:pass@]host:port[?tls=none|starttls|tls]. The scheme picks
// the TLS default (smtp -> none, smtps -> tls); the query overrides it,
// which is how STARTTLS on port 587 is spelled.
const smtpDsn = z
	.string()
	.min(1)
	.transform((value, ctx): SmtpConfig => {
		let url: URL

		try {
			url = new URL(value)
		} catch {
			ctx.addIssue({
				code: "custom",
				message: "must be a URL",
			})

			return z.NEVER
		}

		if (url.protocol !== "smtp:" && url.protocol !== "smtps:") {
			ctx.addIssue({
				code: "custom",
				message: "must use smtp or smtps",
			})

			return z.NEVER
		}

		if (!url.port) {
			ctx.addIssue({
				code: "custom",
				message: "must name a port",
			})

			return z.NEVER
		}

		const tlsParam = url.searchParams.get("tls")

		if (tlsParam !== null && !isTlsMode(tlsParam)) {
			ctx.addIssue({
				code: "custom",
				message: "tls must be none, starttls, or tls",
			})

			return z.NEVER
		}

		const fallbackTls: TlsMode =
			url.protocol === "smtps:" ? "tls" : "none"

		return {
			host: url.hostname,
			port: url.port,
			username: decodeURIComponent(url.username),
			password: decodeURIComponent(url.password),
			tls:
				tlsParam !== null && isTlsMode(tlsParam)
					? tlsParam
					: fallbackTls,
		}
	})

// the standard base64 encoding of exactly 32 bytes, as core's strict
// decoder reads it: 43 characters and one pad, and the last character
// carries two zero padding bits, which leaves it 16 of the alphabet.
const dataSourceKeyPattern = /^[A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]=$/

// the keys encrypt stored data-source credentials: a comma-separated list,
// newest first, each the standard base64 of exactly 32 bytes and none
// listed twice. Core refuses to boot on anything else, so the check here
// names the public variable.
const encryptionKeys = z
	.string()
	.optional()
	.refine(
		(value) =>
			value === undefined ||
			value
				.split(",")
				.every((key) =>
					dataSourceKeyPattern.test(key.trim()),
				),
		{ error: "every key must be base64 of exactly 32 bytes" },
	)
	.refine(
		(value) => {
			if (value === undefined) {
				return true
			}

			const keys = value.split(",").map((key) => key.trim())

			return new Set(keys).size === keys.length
		},
		{ error: "lists the same key twice" },
	)

// the value core parses with Go's time.ParseDuration.
const goDuration = z
	.string()
	.regex(
		/^(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$|^0$/,
		"must be a Go duration such as 2160h",
	)
	.optional()

const socialLoginProviders = [
	{
		name: "github",
		idKey: "OXYNOTE_SOCIAL_LOGIN_GITHUB_CLIENT_ID",
		secretKey: "OXYNOTE_SOCIAL_LOGIN_GITHUB_CLIENT_SECRET",
	},
	{
		name: "google",
		idKey: "OXYNOTE_SOCIAL_LOGIN_GOOGLE_CLIENT_ID",
		secretKey: "OXYNOTE_SOCIAL_LOGIN_GOOGLE_CLIENT_SECRET",
	},
	{
		name: "slack",
		idKey: "OXYNOTE_SOCIAL_LOGIN_SLACK_CLIENT_ID",
		secretKey: "OXYNOTE_SOCIAL_LOGIN_SLACK_CLIENT_SECRET",
	},
] as const satisfies readonly {
	name: SocialLoginProviderName
	idKey: string
	secretKey: string
}[]

const aiAssistantKeys = [
	"PROVIDER",
	"MODEL",
	"API_KEY",
	"BASE_URL",
	"MAX_TOKENS",
	"REQUEST_TIMEOUT",
	"SUMMARY_MODEL",
	"AZURE_API_VERSION",
	"BEDROCK_REGION",
	"BEDROCK_ACCESS_KEY",
	"BEDROCK_SECRET_ACCESS_KEY",
	"BEDROCK_SESSION_TOKEN",
	"VERTEX_PROJECT_ID",
	"VERTEX_REGION",
	"VERTEX_SERVICE_ACCOUNT_JSON",
] as const

// every variable an operator may set, spelled out. Anything else beginning
// with OXYNOTE_ is a boot error, so a typo — or an internal component
// variable — fails loudly instead of being silently ignored.
const baseSchema = z.object({
	OXYNOTE_PUBLIC_URL: publicOrigin.optional(),
	OXYNOTE_DB_DSN: z.string().min(1).optional(),
	OXYNOTE_VALKEY_DSN: valkeyDsn.optional(),
	OXYNOTE_OBJECT_STORAGE_DSN: objectStorageDsn.optional(),
	OXYNOTE_SMTP_DSN: smtpDsn.optional(),
	OXYNOTE_EMAIL_FROM_ADDRESS: z.string().optional(),
	OXYNOTE_CHANGE_DETECTION_URL: httpUrl().optional(),
	OXYNOTE_CHANGE_DETECTION_API_KEY: z.string().optional(),
	OXYNOTE_GITHUB_APP_ID: z
		.string()
		.regex(/^\d+$/, "must be the numeric app id")
		.optional(),
	OXYNOTE_GITHUB_APP_SLUG: z.string().optional(),
	OXYNOTE_GITHUB_APP_SIGNATURE_SECRET: z.string().optional(),
	OXYNOTE_SLACK_APP_CLIENT_ID: z.string().optional(),
	OXYNOTE_SLACK_APP_CLIENT_SECRET: z.string().optional(),
	OXYNOTE_SLACK_APP_SIGNATURE_SECRET: z.string().optional(),
	OXYNOTE_SOCIAL_LOGIN_GITHUB_CLIENT_ID: z.string().optional(),
	OXYNOTE_SOCIAL_LOGIN_GITHUB_CLIENT_SECRET: z.string().optional(),
	OXYNOTE_SOCIAL_LOGIN_GOOGLE_CLIENT_ID: z.string().optional(),
	OXYNOTE_SOCIAL_LOGIN_GOOGLE_CLIENT_SECRET: z.string().optional(),
	OXYNOTE_SOCIAL_LOGIN_SLACK_CLIENT_ID: z.string().optional(),
	OXYNOTE_SOCIAL_LOGIN_SLACK_CLIENT_SECRET: z.string().optional(),
	OXYNOTE_AI_ASSISTANT_PROVIDER: z.string().optional(),
	OXYNOTE_AI_ASSISTANT_MODEL: z.string().optional(),
	OXYNOTE_AI_ASSISTANT_API_KEY: z.string().optional(),
	OXYNOTE_AI_ASSISTANT_BASE_URL: z.string().optional(),
	OXYNOTE_AI_ASSISTANT_MAX_TOKENS: count(),
	OXYNOTE_AI_ASSISTANT_REQUEST_TIMEOUT: goDuration,
	OXYNOTE_AI_ASSISTANT_SUMMARY_MODEL: z.string().optional(),
	OXYNOTE_AI_ASSISTANT_AZURE_API_VERSION: z.string().optional(),
	OXYNOTE_AI_ASSISTANT_BEDROCK_REGION: z.string().optional(),
	OXYNOTE_AI_ASSISTANT_BEDROCK_ACCESS_KEY: z.string().optional(),
	OXYNOTE_AI_ASSISTANT_BEDROCK_SECRET_ACCESS_KEY: z.string().optional(),
	OXYNOTE_AI_ASSISTANT_BEDROCK_SESSION_TOKEN: z.string().optional(),
	OXYNOTE_AI_ASSISTANT_VERTEX_PROJECT_ID: z.string().optional(),
	OXYNOTE_AI_ASSISTANT_VERTEX_REGION: z.string().optional(),
	OXYNOTE_AI_ASSISTANT_VERTEX_SERVICE_ACCOUNT_JSON: z.string().optional(),
	OXYNOTE_MAX_ORGANIZATIONS: limit(),
	OXYNOTE_MAX_ORGANIZATION_MEMBERS: limit(),
	OXYNOTE_RATE_LIMIT_DISABLED: flag(),
	OXYNOTE_MAX_DOCUMENT_HISTORY_ENTRIES: count(),
	OXYNOTE_DOCUMENT_HISTORY_RETENTION: goDuration,
	OXYNOTE_LOG_LEVEL: z
		.enum(["DEBUG", "INFO", "WARN", "ERROR"])
		.optional(),
	OXYNOTE_TERMS_OF_SERVICE_URL: httpUrl().optional(),
	OXYNOTE_PRIVACY_POLICY_URL: httpUrl().optional(),
	OXYNOTE_CRASH_REPORTING_DISABLED: flag(),
	OXYNOTE_AUTH_SECRET: z.string().optional(),
	OXYNOTE_DATA_SOURCE_ENCRYPTION_KEYS: encryptionKeys,
})

type ParsedValues = z.infer<typeof baseSchema>

// each optional feature is keyed on one variable. A companion set without
// its key would be silently ignored by the component, and a key missing a
// required companion would fail deep inside it — both become boot errors
// here, named in the flat namespace.
const featureGroups: {
	key: keyof ParsedValues
	requires: (keyof ParsedValues)[]
	companions: (keyof ParsedValues)[]
}[] = [
	{
		key: "OXYNOTE_SMTP_DSN",
		requires: ["OXYNOTE_EMAIL_FROM_ADDRESS"],
		companions: ["OXYNOTE_EMAIL_FROM_ADDRESS"],
	},
	{
		key: "OXYNOTE_CHANGE_DETECTION_URL",
		requires: [],
		companions: ["OXYNOTE_CHANGE_DETECTION_API_KEY"],
	},
	{
		key: "OXYNOTE_GITHUB_APP_ID",
		requires: [
			"OXYNOTE_GITHUB_APP_SLUG",
			"OXYNOTE_GITHUB_APP_SIGNATURE_SECRET",
		],
		companions: [
			"OXYNOTE_GITHUB_APP_SLUG",
			"OXYNOTE_GITHUB_APP_SIGNATURE_SECRET",
		],
	},
	{
		key: "OXYNOTE_SLACK_APP_CLIENT_ID",
		requires: [
			"OXYNOTE_SLACK_APP_CLIENT_SECRET",
			"OXYNOTE_SLACK_APP_SIGNATURE_SECRET",
		],
		companions: [
			"OXYNOTE_SLACK_APP_CLIENT_SECRET",
			"OXYNOTE_SLACK_APP_SIGNATURE_SECRET",
		],
	},
	{
		key: "OXYNOTE_AI_ASSISTANT_PROVIDER",
		requires: [],
		companions: aiAssistantKeys
			.filter((key) => key !== "PROVIDER")
			.map((key) => `OXYNOTE_AI_ASSISTANT_${key}` as const),
	},
]

const schema = baseSchema.superRefine((values, ctx) => {
	for (const group of featureGroups) {
		if (values[group.key] !== undefined) {
			for (const required of group.requires) {
				if (values[required] !== undefined) {
					continue
				}

				ctx.addIssue({
					code: "custom",
					path: [required],
					message: `required when ${group.key} is set`,
				})
			}

			continue
		}

		for (const companion of group.companions) {
			if (values[companion] === undefined) {
				continue
			}

			ctx.addIssue({
				code: "custom",
				path: [companion],
				message: `only meaningful with ${group.key}`,
			})
		}
	}

	// a social login provider needs both halves of its credentials or
	// neither; one half is a boot error rather than a broken OAuth
	// redirect a user discovers.
	for (const provider of socialLoginProviders) {
		const id = values[provider.idKey]
		const secret = values[provider.secretKey]

		if (Boolean(id) === Boolean(secret)) {
			continue
		}

		ctx.addIssue({
			code: "custom",
			path: [id ? provider.secretKey : provider.idKey],
			message: `${provider.name} login needs both a client id and a client secret, or neither`,
		})
	}
})

function socialLoginConfig(
	values: ParsedValues,
): Partial<Record<SocialLoginProviderName, OAuthCredentials>> {
	const login: Partial<
		Record<SocialLoginProviderName, OAuthCredentials>
	> = {}

	for (const provider of socialLoginProviders) {
		const clientId = values[provider.idKey]
		const clientSecret = values[provider.secretKey]

		if (!clientId || !clientSecret) {
			continue
		}

		login[provider.name] = { clientId, clientSecret }
	}

	return login
}

function aiAssistantConfig(values: ParsedValues): Record<string, string> {
	return Object.fromEntries(
		aiAssistantKeys.map((key) => [
			key,
			values[`OXYNOTE_AI_ASSISTANT_${key}`] ?? "",
		]),
	)
}

// loadConfig parses the container environment into the launcher's typed
// configuration. It is the only place that reads the environment, so a test
// builds a Config by calling this with a literal record.
export function loadConfig(source: Record<string, string | undefined>): Config {
	// compose files list variables that are left blank; an empty value
	// reads as absent, so a required variable reports as missing instead
	// of empty.
	const present = Object.fromEntries(
		Object.entries(source).filter(
			([, value]) => value !== undefined && value !== "",
		),
	)

	// the guard that keeps the public namespace closed: an undeclared
	// OXYNOTE_ variable is a typo or an internal component variable, and
	// silently ignoring either would leave the operator believing it
	// took effect.
	const known = new Set(Object.keys(baseSchema.shape))
	const unknown = Object.keys(present).filter(
		(key) => key.startsWith("OXYNOTE_") && !known.has(key),
	)

	if (unknown.length > 0) {
		throw new Error(
			`unknown environment variables: ${unknown.join(", ")}. ` +
				"The image is configured only through the " +
				"documented OXYNOTE_* variables.",
		)
	}

	const result = schema.safeParse(present)

	if (!result.success) {
		throw new Error(
			`invalid environment:\n${z.prettifyError(result.error)}`,
		)
	}

	const values = result.data
	const url = new URL(values.OXYNOTE_PUBLIC_URL ?? defaultPublicUrl)

	return {
		publicOrigin: url.origin,
		publicHostPort: url.host,
		cookieDomain: url.hostname,
		publicWebSocketOrigin: `${url.protocol === "https:" ? "wss" : "ws"}://${url.host}`,
		databaseDsn: values.OXYNOTE_DB_DSN,
		valkeyDsn: values.OXYNOTE_VALKEY_DSN,
		objectStorage: values.OXYNOTE_OBJECT_STORAGE_DSN,
		smtp: values.OXYNOTE_SMTP_DSN,
		emailFromAddress: values.OXYNOTE_EMAIL_FROM_ADDRESS ?? "",
		changeDetection: values.OXYNOTE_CHANGE_DETECTION_URL
			? {
					url: values.OXYNOTE_CHANGE_DETECTION_URL,
					apiKey:
						values.OXYNOTE_CHANGE_DETECTION_API_KEY ??
						"",
				}
			: undefined,
		githubApp: values.OXYNOTE_GITHUB_APP_ID
			? {
					appId: values.OXYNOTE_GITHUB_APP_ID,
					appSlug:
						values.OXYNOTE_GITHUB_APP_SLUG ??
						"",
					signatureSecret:
						values.OXYNOTE_GITHUB_APP_SIGNATURE_SECRET ??
						"",
				}
			: undefined,
		slackApp: values.OXYNOTE_SLACK_APP_CLIENT_ID
			? {
					clientId: values.OXYNOTE_SLACK_APP_CLIENT_ID,
					clientSecret:
						values.OXYNOTE_SLACK_APP_CLIENT_SECRET ??
						"",
					signatureSecret:
						values.OXYNOTE_SLACK_APP_SIGNATURE_SECRET ??
						"",
				}
			: undefined,
		socialLogin: socialLoginConfig(values),
		aiAssistant: aiAssistantConfig(values),
		maxOrganizations: values.OXYNOTE_MAX_ORGANIZATIONS ?? "",
		maxOrganizationMembers:
			values.OXYNOTE_MAX_ORGANIZATION_MEMBERS ?? "",
		rateLimitDisabled: values.OXYNOTE_RATE_LIMIT_DISABLED,
		maxDocumentHistoryEntries:
			values.OXYNOTE_MAX_DOCUMENT_HISTORY_ENTRIES ?? "",
		documentHistoryRetention:
			values.OXYNOTE_DOCUMENT_HISTORY_RETENTION ?? "",
		// the components default to INFO on their own, which puts every
		// served request in `docker logs`.
		logLevel: values.OXYNOTE_LOG_LEVEL ?? "WARN",
		termsOfServiceUrl: values.OXYNOTE_TERMS_OF_SERVICE_URL ?? "",
		privacyPolicyUrl: values.OXYNOTE_PRIVACY_POLICY_URL ?? "",
		crashReportingDisabled: values.OXYNOTE_CRASH_REPORTING_DISABLED,
		authSecret: values.OXYNOTE_AUTH_SECRET,
		dataSourceEncryptionKeys:
			values.OXYNOTE_DATA_SOURCE_ENCRYPTION_KEYS,
	}
}
