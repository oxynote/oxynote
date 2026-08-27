import { describe, it } from "vitest"
import {
	authRealtimePort,
	buildChildEnvs,
	caddyPort,
	corePort,
	webPort,
} from "./mapping.js"
import { testConfig, testSecrets } from "./test-helpers.js"

const testSentryDsns = {
	webDsn: "https://web@sentry.example.com/1",
	coreDsn: "https://core@sentry.example.com/2",
	authRealtimeDsn: "https://auth@sentry.example.com/3",
}

const inheritedEnv = { PATH: "/usr/bin", HOME: "/oxynote" }

function build(
	config = testConfig(),
	sentryDsns = testSentryDsns,
	secrets = testSecrets(),
) {
	return buildChildEnvs(config, secrets, sentryDsns, inheritedEnv)
}

describe("buildChildEnvs", () => {
	// the loopback binds are the trust boundary: /api/x/* and
	// /api/internal/* carry no authentication, so a change here is a
	// security decision, not a port shuffle
	it("binds every service to loopback and leaves only caddy public", ({
		expect,
	}) => {
		const envs = build()

		expect(envs.core.OXYNOTE_CORE_SERVER_ADDRESS).toBe(
			"127.0.0.1:8180",
		)
		expect(envs.authRealtime.OXYNOTE_AUTH_REALTIME_ADDRESS).toBe(
			"127.0.0.1:8181",
		)
		expect(envs.web.NITRO_HOST).toBe("127.0.0.1")
		expect(envs.web.NITRO_PORT).toBe("3000")
		expect(caddyPort).toBe(8080)
		expect(corePort).toBe(8180)
		expect(authRealtimePort).toBe(8181)
		expect(webPort).toBe(3000)
	})

	it("derives every public URL from the single public origin", ({
		expect,
	}) => {
		const envs = build()

		expect(envs.core.OXYNOTE_CORE_SERVER_PUBLIC_URL).toBe(
			"https://notes.example.com/core",
		)
		expect(envs.core.OXYNOTE_CORE_BASE_APP_URL).toBe(
			"https://notes.example.com",
		)
		expect(envs.core.OXYNOTE_CORE_ORIGINS).toBe(
			"https://notes.example.com,notes.example.com",
		)
		expect(envs.core.OXYNOTE_CORE_SLACK_REDIRECT_URL).toBe(
			"https://notes.example.com/slack",
		)
		expect(
			envs.authRealtime
				.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_BASE_URL,
		).toBe("https://notes.example.com/auth-realtime")
		expect(
			envs.authRealtime
				.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_COOKIE_DOMAIN,
		).toBe("notes.example.com")
		expect(
			envs.authRealtime
				.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_ORGANIZATION_INVITATION_URL,
		).toBe("https://notes.example.com/accept-invite")
		expect(envs.web.NUXT_PUBLIC_APP_BASE_URL).toBe(
			"https://notes.example.com",
		)
		expect(envs.web.NUXT_PUBLIC_CORE_API_BASE_WS_URL).toBe(
			"wss://notes.example.com/core",
		)
		expect(envs.web.NUXT_PUBLIC_AUTH_REALTIME_API_BASE_WS_URL).toBe(
			"wss://notes.example.com/auth-realtime/hocuspocus",
		)
	})

	// the token audience check compares these two byte for byte
	it("gives core and auth-realtime the identical MCP resource URL", ({
		expect,
	}) => {
		const envs = build()

		expect(envs.core.OXYNOTE_CORE_SERVER_MCP_RESOURCE_URL).toBe(
			envs.authRealtime.OXYNOTE_AUTH_REALTIME_MCP_RESOURCE,
		)
		expect(envs.core.OXYNOTE_CORE_SERVER_MCP_RESOURCE_URL).toBe(
			"https://notes.example.com/core/api/mcp",
		)
	})

	it("wires the services to each other over loopback", ({ expect }) => {
		const envs = build()

		expect(envs.core.OXYNOTE_CORE_SERVER_AUTH_BETTER_AUTH_URL).toBe(
			"http://127.0.0.1:8181/api/auth/get-session",
		)
		expect(envs.core.OXYNOTE_CORE_SERVER_MCP_SESSION_URL).toBe(
			"http://127.0.0.1:8181/api/internal/mcp/session",
		)
		expect(
			envs.authRealtime.OXYNOTE_AUTH_REALTIME_BACKEND_URL,
		).toBe("http://127.0.0.1:8180")
		expect(envs.web.NUXT_CORE_API_INTERNAL_HTTP_URL).toBe(
			"http://127.0.0.1:8180",
		)
		expect(envs.web.NUXT_AUTH_REALTIME_API_INTERNAL_HTTP_URL).toBe(
			"http://127.0.0.1:8181",
		)
	})

	it("keeps every child's environment to its own component", ({
		expect,
	}) => {
		const envs = build()

		expect(
			Object.keys(envs.web).filter((key) =>
				key.startsWith("OXYNOTE_"),
			),
		).toEqual([])
		expect(
			Object.keys(envs.core).filter(
				(key) =>
					!key.startsWith("OXYNOTE_CORE_") &&
					!(key in inheritedEnv),
			),
		).toEqual([])
		expect(envs.caddy).toEqual({
			...inheritedEnv,
			XDG_DATA_HOME: "/tmp/caddy/data",
			XDG_CONFIG_HOME: "/tmp/caddy/config",
		})
	})

	it("hands each secret only to the component that needs it", ({
		expect,
	}) => {
		const envs = build()

		expect(
			envs.authRealtime
				.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SECRET,
		).toBe("test-auth-secret")
		expect(
			envs.core
				.OXYNOTE_CORE_DB_DATA_SOURCE_CREDENTIALS_SIGNING_SECRET,
		).toBe("0123456789abcdef0123456789abcdef")
		expect(
			Object.values(envs.web).includes("test-auth-secret"),
		).toBe(false)
		expect(
			Object.values(envs.authRealtime).includes(
				"0123456789abcdef0123456789abcdef",
			),
		).toBe(false)
	})

	// core keeps objects on the data volume when no store is configured,
	// and refuses to boot with neither a URL nor a path
	it("points core at the data volume when no object store is configured", ({
		expect,
	}) => {
		const envs = build(testConfig({ objectStorage: undefined }))

		expect(envs.core.OXYNOTE_CORE_OBJECT_STORAGE_LOCAL_PATH).toBe(
			"/oxynote/data/object-storage",
		)
		expect(envs.core.OXYNOTE_CORE_OBJECT_STORAGE_URL).toBe("")
		expect(envs.core.OXYNOTE_CORE_OBJECT_STORAGE_ACCESS_KEY).toBe(
			"",
		)
		expect(envs.core.OXYNOTE_CORE_OBJECT_STORAGE_BUCKET).toBe("")
	})

	it("keeps the storage path alongside a configured object store", ({
		expect,
	}) => {
		const envs = build()

		expect(envs.core.OXYNOTE_CORE_OBJECT_STORAGE_URL).toBe(
			"http://s3.example.com",
		)
		expect(envs.core.OXYNOTE_CORE_OBJECT_STORAGE_BUCKET).toBe(
			"documents",
		)
		expect(envs.core.OXYNOTE_CORE_OBJECT_STORAGE_LOCAL_PATH).toBe(
			"/oxynote/data/object-storage",
		)
	})

	it("empties both valkey variables when it is not configured", ({
		expect,
	}) => {
		const envs = build(testConfig({ valkeyDsn: undefined }))

		expect(envs.core.OXYNOTE_CORE_VALKEY_DSN).toBe("")
		expect(envs.authRealtime.OXYNOTE_AUTH_REALTIME_VALKEY_DSN).toBe(
			"",
		)
	})

	it("applies the baked telemetry DSNs per component", ({ expect }) => {
		const envs = build()

		expect(envs.web.NUXT_PUBLIC_SENTRY_DSN).toBe(
			testSentryDsns.webDsn,
		)
		expect(envs.core.OXYNOTE_CORE_SENTRY_DSN).toBe(
			testSentryDsns.coreDsn,
		)
		expect(envs.authRealtime.SENTRY_DSN).toBe(
			testSentryDsns.authRealtimeDsn,
		)
	})

	it("zeroes every DSN when error reporting is disabled", ({
		expect,
	}) => {
		const envs = build(testConfig({ crashReportingDisabled: true }))

		expect(envs.web.NUXT_PUBLIC_SENTRY_DSN).toBe("")
		expect(envs.core.OXYNOTE_CORE_SENTRY_DSN).toBe("")
		expect(envs.authRealtime.SENTRY_DSN).toBe("")
	})

	it("leaves a disabled feature's variables empty", ({ expect }) => {
		const envs = build()

		expect(envs.core.OXYNOTE_CORE_MEILISEARCH_URL).toBe("")
		expect(envs.core.OXYNOTE_CORE_EMAIL_SMTP_HOST).toBe("")
		expect(envs.core.OXYNOTE_CORE_GITHUB_APP_ID).toBe("")
		expect(envs.core.OXYNOTE_CORE_SLACK_CLIENT_ID).toBe("")
		expect(envs.core.OXYNOTE_CORE_ASSISTANT_PROVIDER).toBe("")
		expect(
			envs.authRealtime
				.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GITHUB_CLIENT_ID,
		).toBe("")
	})

	it("maps an enabled feature onto its component variables", ({
		expect,
	}) => {
		const envs = build(
			testConfig({
				smtp: {
					host: "mail.example.com",
					port: "587",
					username: "user",
					password: "pass",
					tls: "starttls",
				},
				emailFromAddress: "Oxynote <team@example.com>",
				meilisearch: {
					url: "http://meilisearch:7700",
					masterKey: "master",
				},
				githubApp: {
					appId: "123",
					appSlug: "oxynote",
					signatureSecret: "sig",
				},
				slackApp: {
					clientId: "cid",
					clientSecret: "cs",
					signatureSecret: "ss",
				},
				socialLogin: {
					google: {
						clientId: "gid",
						clientSecret: "gs",
					},
				},
				aiAssistant: {
					...testConfig().aiAssistant,
					PROVIDER: "anthropic",
					API_KEY: "sk-ant",
				},
				rateLimitDisabled: true,
				maxOrganizations: "10",
			}),
		)

		expect(envs.core.OXYNOTE_CORE_EMAIL_SMTP_HOST).toBe(
			"mail.example.com",
		)
		expect(envs.core.OXYNOTE_CORE_EMAIL_SMTP_TLS).toBe("starttls")
		expect(envs.core.OXYNOTE_CORE_EMAIL_FROM_ADDRESS).toBe(
			"Oxynote <team@example.com>",
		)
		expect(envs.core.OXYNOTE_CORE_MEILISEARCH_MASTER_KEY).toBe(
			"master",
		)
		expect(envs.core.OXYNOTE_CORE_GITHUB_APP_ID).toBe("123")
		expect(envs.core.OXYNOTE_CORE_GITHUB_PRIVATE_KEY_PATH).toBe(
			"/oxynote/github/private-key.pem",
		)
		expect(envs.core.OXYNOTE_CORE_SLACK_SIGNATURE_SECRET).toBe("ss")
		expect(envs.core.OXYNOTE_CORE_ASSISTANT_API_KEY).toBe("sk-ant")
		expect(
			envs.authRealtime
				.OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GOOGLE_CLIENT_ID,
		).toBe("gid")
		expect(
			envs.authRealtime
				.OXYNOTE_AUTH_REALTIME_RATE_LIMIT_DISABLED,
		).toBe("true")
		expect(
			envs.authRealtime
				.OXYNOTE_AUTH_REALTIME_MAX_ORGANIZATIONS,
		).toBe("10")
	})

	it("keeps the desktop deep-link schemes in the trusted origins", ({
		expect,
	}) => {
		const envs = build()

		expect(
			envs.authRealtime.OXYNOTE_AUTH_REALTIME_TRUSTED_ORIGINS,
		).toBe("https://notes.example.com,oxynote://,oxynote:/")
	})

	it("ships the in-app documentation links", ({ expect }) => {
		const envs = build()

		expect(
			envs.web.NUXT_PUBLIC_PROMETHEUS_QUERY_GUIDE_URL,
		).toContain("prometheus.io")
		expect(
			envs.web.NUXT_PUBLIC_LINK_TO_MORE_INFO_ABOUT_PRODUCT,
		).toBe("https://oxynote.io")
	})
})
