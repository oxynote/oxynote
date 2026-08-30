import type { Config } from "./env.js"
import type { Secrets } from "./secrets.js"

// the internal port layout of the production image. Core's /api/x/* and
// auth-realtime's /api/internal/* surfaces carry no authentication, so the
// three services bind loopback — unreachable from outside the container —
// and caddy is the only wildcard listener. The supervisor's boot check
// verifies these binds; change them only together with
// docker/prod/Caddyfile.
export const caddyPort = 8080
export const corePort = 8180
export const authRealtimePort = 8181
export const webPort = 3000

export const coreUrl = `http://127.0.0.1:${corePort}`
export const authRealtimeUrl = `http://127.0.0.1:${authRealtimePort}`
export const webUrl = `http://127.0.0.1:${webPort}`

// where the data volume is mounted; generated secrets live beneath it.
export const dataDir = "/oxynote/data"

// where core keeps uploaded objects when no object store is configured.
// It sits on the data volume, so the files survive a restart like the
// secrets do.
const objectStorageLocalPath = `${dataDir}/object-storage`

// the fixed path a GitHub App's private key is mounted at when the
// integration is enabled.
const githubPrivateKeyPath = "/oxynote/github/private-key.pem"

export interface SentryDsns {
	webDsn: string
	coreDsn: string
	authRealtimeDsn: string
}

export interface ChildEnvs {
	core: Record<string, string>
	authRealtime: Record<string, string>
	web: Record<string, string>
	caddy: Record<string, string>
}

// in-app documentation links shipped with the image. The values mirror
// docker/env/web.example.env — the nuxt-side defaults are empty strings, so
// omitting them here would blank the links.
const webDocumentationLinks = {
	NUXT_PUBLIC_LINK_TO_MORE_INFO_ABOUT_PRODUCT: "https://oxynote.io",
	NUXT_PUBLIC_POSTGRESQL_READ_ONLY_USER_SETUP_GUIDE_URL:
		"https://tableplus.com/blog/2018/04/postgresql-how-to-create-read-only-user.html",
	NUXT_PUBLIC_MYSQL_READ_ONLY_USER_SETUP_GUIDE_URL:
		"https://guide.trevor.io/hc/en-us/articles/6843804879261-Creating-a-read-only-database-user-in-MySQL",
	NUXT_PUBLIC_MARIADB_READ_ONLY_USER_SETUP_GUIDE_URL:
		"https://guide.trevor.io/hc/en-us/articles/6843804879261-Creating-a-read-only-database-user-in-MySQL",
	NUXT_PUBLIC_PROMETHEUS_QUERY_GUIDE_URL:
		"https://prometheus.io/docs/prometheus/latest/querying/basics/",
	NUXT_PUBLIC_POSTGRESQL_QUERY_GUIDE_URL:
		"https://www.postgresql.org/docs/current/sql-select.html",
	NUXT_PUBLIC_MYSQL_QUERY_GUIDE_URL:
		"https://dev.mysql.com/doc/refman/8.0/en/select.html",
	NUXT_PUBLIC_MARIADB_QUERY_GUIDE_URL:
		"https://mariadb.com/kb/en/select/",
}

function aiAssistantEnv(
	aiAssistant: Record<string, string>,
): Record<string, string> {
	return Object.fromEntries(
		Object.entries(aiAssistant).map(([key, value]) => [
			`OXYNOTE_CORE_ASSISTANT_${key}`,
			value,
		]),
	)
}

// buildChildEnvs assembles each process's environment from scratch: a child
// receives exactly its own component's variables and nothing else, so no
// flat OXYNOTE_* value and no other component's secret ever leaks through.
export function buildChildEnvs(
	config: Config,
	secrets: Secrets,
	bakedSentryDsns: SentryDsns,
	inheritedEnv: Record<string, string>,
): ChildEnvs {
	const publicCoreUrl = `${config.publicOrigin}/core`
	const publicAuthRealtimeUrl = `${config.publicOrigin}/auth-realtime`
	const mcpResourceUrl = `${config.publicOrigin}/core/api/mcp`
	const sentryDsns = config.crashReportingDisabled
		? { webDsn: "", coreDsn: "", authRealtimeDsn: "" }
		: bakedSentryDsns

	const core: Record<string, string> = {
		...inheritedEnv,
		OXYNOTE_CORE_SERVER_ADDRESS: `127.0.0.1:${corePort}`,
		OXYNOTE_CORE_DB_DSN: config.databaseDsn,
		OXYNOTE_CORE_VALKEY_DSN: config.valkeyDsn ?? "",
		OXYNOTE_CORE_MEILISEARCH_URL: config.meilisearch?.url ?? "",
		OXYNOTE_CORE_MEILISEARCH_MASTER_KEY:
			config.meilisearch?.masterKey ?? "",
		// an empty URL is what makes core store objects on disk at
		// OBJECT_STORAGE_LOCAL_PATH instead of in a remote store.
		OXYNOTE_CORE_OBJECT_STORAGE_URL:
			config.objectStorage?.url ?? "",
		OXYNOTE_CORE_OBJECT_STORAGE_REGION:
			config.objectStorage?.region ?? "",
		OXYNOTE_CORE_OBJECT_STORAGE_ACCESS_KEY:
			config.objectStorage?.accessKey ?? "",
		OXYNOTE_CORE_OBJECT_STORAGE_SECRET_KEY:
			config.objectStorage?.secretKey ?? "",
		OXYNOTE_CORE_OBJECT_STORAGE_BUCKET:
			config.objectStorage?.bucket ?? "",
		OXYNOTE_CORE_OBJECT_STORAGE_LOCAL_PATH: objectStorageLocalPath,
		OXYNOTE_CORE_SERVER_AUTH_BETTER_AUTH_URL: `${authRealtimeUrl}/api/auth/get-session`,
		OXYNOTE_CORE_SERVER_MCP_SESSION_URL: `${authRealtimeUrl}/api/internal/mcp/session`,
		OXYNOTE_CORE_AUTH_REALTIME_URL: authRealtimeUrl,
		OXYNOTE_CORE_CHANGEDETECTION_API_URL:
			config.changeDetection?.url ?? "",
		OXYNOTE_CORE_CHANGEDETECTION_API_KEY:
			config.changeDetection?.apiKey ?? "",
		OXYNOTE_CORE_SERVER_PUBLIC_URL: publicCoreUrl,
		OXYNOTE_CORE_BASE_APP_URL: config.publicOrigin,
		OXYNOTE_CORE_SERVER_MCP_RESOURCE_URL: mcpResourceUrl,
		OXYNOTE_CORE_ORIGINS: `${config.publicOrigin},${config.publicHostPort}`,
		OXYNOTE_CORE_GITHUB_APP_ID: config.githubApp?.appId ?? "",
		OXYNOTE_CORE_GITHUB_APP_SLUG: config.githubApp?.appSlug ?? "",
		OXYNOTE_CORE_GITHUB_SIGNATURE_SECRET:
			config.githubApp?.signatureSecret ?? "",
		OXYNOTE_CORE_GITHUB_INSTALLATION_SIGNING_SECRET:
			secrets.githubInstallationSigningSecret,
		OXYNOTE_CORE_GITHUB_PRIVATE_KEY_PATH: githubPrivateKeyPath,
		OXYNOTE_CORE_SLACK_CLIENT_ID: config.slackApp?.clientId ?? "",
		OXYNOTE_CORE_SLACK_CLIENT_SECRET:
			config.slackApp?.clientSecret ?? "",
		OXYNOTE_CORE_SLACK_SIGNATURE_SECRET:
			config.slackApp?.signatureSecret ?? "",
		OXYNOTE_CORE_SLACK_REDIRECT_URL: `${config.publicOrigin}/slack`,
		OXYNOTE_CORE_SLACK_INSTALLATION_SIGNING_SECRET:
			secrets.slackInstallationSigningSecret,
		...aiAssistantEnv(config.aiAssistant),
		OXYNOTE_CORE_DB_MAX_DOCUMENT_HISTORY_ENTRIES:
			config.maxDocumentHistoryEntries,
		OXYNOTE_CORE_DB_DOCUMENT_HISTORY_RETENTION:
			config.documentHistoryRetention,
		OXYNOTE_CORE_DB_DATA_SOURCE_CREDENTIALS_SIGNING_SECRET:
			secrets.dataSourceEncryptionKey,
		OXYNOTE_CORE_EMAIL_SMTP_HOST: config.smtp?.host ?? "",
		OXYNOTE_CORE_EMAIL_SMTP_PORT: config.smtp?.port ?? "",
		OXYNOTE_CORE_EMAIL_SMTP_USERNAME: config.smtp?.username ?? "",
		OXYNOTE_CORE_EMAIL_SMTP_PASSWORD: config.smtp?.password ?? "",
		OXYNOTE_CORE_EMAIL_SMTP_TLS: config.smtp?.tls ?? "",
		OXYNOTE_CORE_EMAIL_FROM_ADDRESS: config.emailFromAddress,
		OXYNOTE_CORE_SENTRY_DSN: sentryDsns.coreDsn,
		OXYNOTE_CORE_LOG_LEVEL: config.logLevel,
	}

	const authRealtime: Record<string, string> = {
		...inheritedEnv,
		NODE_ENV: "production",
		OXYNOTE_AUTH_REALTIME_ADDRESS: `127.0.0.1:${authRealtimePort}`,
		OXYNOTE_AUTH_REALTIME_BACKEND_URL: coreUrl,
		OXYNOTE_AUTH_REALTIME_DB_DSN: config.databaseDsn,
		OXYNOTE_AUTH_REALTIME_VALKEY_DSN: config.valkeyDsn ?? "",
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_BASE_URL:
			publicAuthRealtimeUrl,
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SECRET: secrets.authSecret,
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_COOKIE_DOMAIN:
			config.cookieDomain,
		OXYNOTE_AUTH_REALTIME_FRONTEND_URL: config.publicOrigin,
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_ORGANIZATION_INVITATION_URL: `${config.publicOrigin}/accept-invite`,
		// the two oxynote schemes keep the desktop app's OAuth
		// deep-link handoff working against a self-hosted instance.
		OXYNOTE_AUTH_REALTIME_TRUSTED_ORIGINS: `${config.publicOrigin},oxynote://,oxynote:/`,
		OXYNOTE_AUTH_REALTIME_MCP_RESOURCE: mcpResourceUrl,
		OXYNOTE_AUTH_REALTIME_MAX_ORGANIZATIONS:
			config.maxOrganizations,
		OXYNOTE_AUTH_REALTIME_MAX_ORGANIZATION_MEMBERS:
			config.maxOrganizationMembers,
		OXYNOTE_AUTH_REALTIME_RATE_LIMIT_DISABLED:
			config.rateLimitDisabled ? "true" : "false",
		OXYNOTE_AUTH_REALTIME_LOG_LEVEL: config.logLevel,
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GITHUB_CLIENT_ID:
			config.socialLogin.github?.clientId ?? "",
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GITHUB_CLIENT_SECRET:
			config.socialLogin.github?.clientSecret ?? "",
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GOOGLE_CLIENT_ID:
			config.socialLogin.google?.clientId ?? "",
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GOOGLE_CLIENT_SECRET:
			config.socialLogin.google?.clientSecret ?? "",
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SLACK_CLIENT_ID:
			config.socialLogin.slack?.clientId ?? "",
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SLACK_CLIENT_SECRET:
			config.socialLogin.slack?.clientSecret ?? "",
		SENTRY_DSN: sentryDsns.authRealtimeDsn,
	}

	const web: Record<string, string> = {
		...inheritedEnv,
		NODE_ENV: "production",
		NITRO_HOST: "127.0.0.1",
		NITRO_PORT: String(webPort),
		NUXT_PUBLIC_APP_BASE_URL: config.publicOrigin,
		NUXT_PUBLIC_CORE_API_BASE_HTTP_URL: publicCoreUrl,
		NUXT_PUBLIC_CORE_API_BASE_WS_URL: `${config.publicWebSocketOrigin}/core`,
		NUXT_PUBLIC_AUTH_REALTIME_API_BASE_HTTP_URL:
			publicAuthRealtimeUrl,
		NUXT_PUBLIC_AUTH_REALTIME_API_BASE_WS_URL: `${config.publicWebSocketOrigin}/auth-realtime/hocuspocus`,
		NUXT_CORE_API_INTERNAL_HTTP_URL: coreUrl,
		NUXT_AUTH_REALTIME_API_INTERNAL_HTTP_URL: authRealtimeUrl,
		NUXT_PUBLIC_SENTRY_DSN: sentryDsns.webDsn,
		NUXT_PUBLIC_TERMS_OF_SERVICE_URL: config.termsOfServiceUrl,
		NUXT_PUBLIC_PRIVACY_POLICY_URL: config.privacyPolicyUrl,
		...webDocumentationLinks,
	}

	// caddy wants writable storage even with TLS off; /tmp is the only
	// scratch path the image allows.
	const caddy: Record<string, string> = {
		...inheritedEnv,
		XDG_DATA_HOME: "/tmp/caddy/data",
		XDG_CONFIG_HOME: "/tmp/caddy/config",
	}

	return { core, authRealtime, web, caddy }
}
