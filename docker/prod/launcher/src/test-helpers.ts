import type { Config } from "./env.js"
import type { Secrets } from "./secrets.js"

// a complete, valid environment. Each test overrides only the variables it
// is about, so the rest can never be the reason a case passes or fails.
export function completeEnv(
	overrides: Record<string, string | undefined> = {},
): Record<string, string | undefined> {
	return {
		OXYNOTE_PUBLIC_URL: "https://notes.example.com",
		OXYNOTE_DB_DSN: "postgresql://user:pass@postgres/oxynote",
		OXYNOTE_VALKEY_DSN: "redis://valkey:6379",
		OXYNOTE_OBJECT_STORAGE_DSN:
			"http://access:secret@s3.example.com/documents",
		...overrides,
	}
}

// the configuration completeEnv() parses into, for tests that start after
// loading.
export function testConfig(overrides: Partial<Config> = {}): Config {
	return {
		publicOrigin: "https://notes.example.com",
		publicHostPort: "notes.example.com",
		cookieDomain: "notes.example.com",
		publicWebSocketOrigin: "wss://notes.example.com",
		databaseDsn: "postgresql://user:pass@postgres/oxynote",
		valkeyDsn: "redis://valkey:6379",
		objectStorage: {
			url: "http://s3.example.com",
			accessKey: "access",
			secretKey: "secret",
			bucket: "documents",
			region: "",
		},
		smtp: undefined,
		emailFromAddress: "",
		meilisearch: undefined,
		changeDetection: undefined,
		githubApp: undefined,
		slackApp: undefined,
		socialLogin: {},
		aiAssistant: {
			PROVIDER: "",
			MODEL: "",
			API_KEY: "",
			BASE_URL: "",
			MAX_TOKENS: "",
			REQUEST_TIMEOUT: "",
			SUMMARY_MODEL: "",
			AZURE_API_VERSION: "",
			BEDROCK_REGION: "",
			BEDROCK_ACCESS_KEY: "",
			BEDROCK_SECRET_ACCESS_KEY: "",
			BEDROCK_SESSION_TOKEN: "",
			VERTEX_PROJECT_ID: "",
			VERTEX_REGION: "",
			VERTEX_SERVICE_ACCOUNT_JSON: "",
		},
		maxOrganizations: "",
		maxOrganizationMembers: "",
		rateLimitDisabled: false,
		maxDocumentHistoryEntries: "",
		documentHistoryRetention: "",
		logLevel: "WARN",
		termsOfServiceUrl: "",
		privacyPolicyUrl: "",
		crashReportingDisabled: false,
		authSecret: undefined,
		dataSourceEncryptionKey: undefined,
		...overrides,
	}
}

export function testSecrets(): Secrets {
	return {
		authSecret: "test-auth-secret",
		dataSourceEncryptionKey: "0123456789abcdef0123456789abcdef",
		githubInstallationSigningSecret: "github-installation-secret",
		slackInstallationSigningSecret: "slack-installation-secret",
	}
}
