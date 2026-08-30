import { describe, it } from "vitest"
import { loadConfig } from "./env.js"
import { completeEnv } from "./test-helpers.js"

describe("loadConfig", () => {
	it("maps every required variable onto the typed configuration", ({
		expect,
	}) => {
		const config = loadConfig(completeEnv())

		expect(config.databaseDsn).toBe(
			"postgresql://user:pass@postgres/oxynote",
		)
		expect(config.valkeyDsn).toBe("redis://valkey:6379")
		expect(config.objectStorage).toEqual({
			url: "http://s3.example.com",
			accessKey: "access",
			secretKey: "secret",
			bucket: "documents",
			region: "",
		})
	})

	it("ignores variables outside the OXYNOTE_ namespace", ({ expect }) => {
		const config = loadConfig(
			completeEnv({ PATH: "/usr/bin", HOME: "/oxynote" }),
		)

		expect(config.publicOrigin).toBe("https://notes.example.com")
	})

	it("treats an empty value as an absent variable", ({ expect }) => {
		expect(() =>
			loadConfig(completeEnv({ OXYNOTE_DB_DSN: "" })),
		).toThrow("OXYNOTE_DB_DSN")
	})

	it.for([
		{ name: "a typo", input: "OXYNOTE_PUBILC_URL" },
		{
			name: "an internal core variable",
			input: "OXYNOTE_CORE_DB_DSN",
		},
		{
			name: "an internal auth-realtime variable",
			input: "OXYNOTE_AUTH_REALTIME_ADDRESS",
		},
	])("rejects $name by name", ({ input }, { expect }) => {
		expect(() =>
			loadConfig(completeEnv({ [input]: "value" })),
		).toThrow(input)
	})

	describe("public URL", () => {
		it("derives the host forms and the WebSocket origin", ({
			expect,
		}) => {
			const config = loadConfig(
				completeEnv({
					OXYNOTE_PUBLIC_URL:
						"https://notes.example.com:8443",
				}),
			)

			expect(config.publicOrigin).toBe(
				"https://notes.example.com:8443",
			)
			expect(config.publicHostPort).toBe(
				"notes.example.com:8443",
			)
			expect(config.cookieDomain).toBe("notes.example.com")
			expect(config.publicWebSocketOrigin).toBe(
				"wss://notes.example.com:8443",
			)
		})

		it("swaps http for ws", ({ expect }) => {
			const config = loadConfig(
				completeEnv({
					OXYNOTE_PUBLIC_URL:
						"http://localhost:8080",
				}),
			)

			expect(config.publicWebSocketOrigin).toBe(
				"ws://localhost:8080",
			)
		})

		it("strips a trailing slash", ({ expect }) => {
			const config = loadConfig(
				completeEnv({
					OXYNOTE_PUBLIC_URL:
						"https://notes.example.com/",
				}),
			)

			expect(config.publicOrigin).toBe(
				"https://notes.example.com",
			)
		})

		it.for([
			{
				name: "a path",
				input: "https://example.com/oxynote",
			},
			{ name: "a query", input: "https://example.com/?a=b" },
			{ name: "a bare host", input: "notes.example.com" },
		])("rejects $name", ({ input }, { expect }) => {
			expect(() =>
				loadConfig(
					completeEnv({
						OXYNOTE_PUBLIC_URL: input,
					}),
				),
			).toThrow("OXYNOTE_PUBLIC_URL")
		})
	})

	describe("valkey URL", () => {
		it("accepts credentials and a database index", ({ expect }) => {
			const config = loadConfig(
				completeEnv({
					OXYNOTE_VALKEY_DSN:
						"rediss://user:pass@valkey:6379/2",
				}),
			)

			expect(config.valkeyDsn).toBe(
				"rediss://user:pass@valkey:6379/2",
			)
		})

		it.for([
			{ name: "an http URL", input: "http://valkey:6379" },
			{ name: "a bare address", input: "valkey:6379" },
		])("rejects $name", ({ input }, { expect }) => {
			expect(() =>
				loadConfig(
					completeEnv({
						OXYNOTE_VALKEY_DSN: input,
					}),
				),
			).toThrow("OXYNOTE_VALKEY_DSN")
		})
	})

	describe("storage DSN", () => {
		it("decodes the credentials and reads the region", ({
			expect,
		}) => {
			const config = loadConfig(
				completeEnv({
					OXYNOTE_OBJECT_STORAGE_DSN:
						"https://ak%40x:s%3Akey@s3.example.com/bucket?region=eu-central-1",
				}),
			)

			expect(config.objectStorage).toEqual({
				url: "https://s3.example.com",
				accessKey: "ak@x",
				secretKey: "s:key",
				bucket: "bucket",
				region: "eu-central-1",
			})
		})

		it("defaults the bucket when the path is empty", ({
			expect,
		}) => {
			const config = loadConfig(
				completeEnv({
					OXYNOTE_OBJECT_STORAGE_DSN:
						"http://a:b@s3.example.com",
				}),
			)

			expect(config.objectStorage?.bucket).toBe("oxynote")
		})

		it.for([
			{
				name: "missing credentials",
				input: "http://s3.example.com/b",
			},
			{
				name: "a redis scheme",
				input: "redis://a:b@s3.example.com",
			},
			{ name: "not a URL", input: "::" },
		])("rejects $name", ({ input }, { expect }) => {
			expect(() =>
				loadConfig(
					completeEnv({
						OXYNOTE_OBJECT_STORAGE_DSN:
							input,
					}),
				),
			).toThrow("OXYNOTE_OBJECT_STORAGE_DSN")
		})
	})

	it("runs without valkey", ({ expect }) => {
		const config = loadConfig(
			completeEnv({ OXYNOTE_VALKEY_DSN: undefined }),
		)

		expect(config.valkeyDsn).toBeUndefined()
	})

	it("runs without an object store", ({ expect }) => {
		const config = loadConfig(
			completeEnv({ OXYNOTE_OBJECT_STORAGE_DSN: undefined }),
		)

		expect(config.objectStorage).toBeUndefined()
	})

	describe("smtp", () => {
		it("splits the URL and defaults tls from the scheme", ({
			expect,
		}) => {
			const config = loadConfig(
				completeEnv({
					OXYNOTE_SMTP_DSN:
						"smtps://user:pa%40ss@mail.example.com:465",
					OXYNOTE_EMAIL_FROM_ADDRESS:
						"Oxynote <team@example.com>",
				}),
			)

			expect(config.smtp).toEqual({
				host: "mail.example.com",
				port: "465",
				username: "user",
				password: "pa@ss",
				tls: "tls",
			})
			expect(config.emailFromAddress).toBe(
				"Oxynote <team@example.com>",
			)
		})

		it("lets the query override the tls mode", ({ expect }) => {
			const config = loadConfig(
				completeEnv({
					OXYNOTE_SMTP_DSN:
						"smtp://mail.example.com:587?tls=starttls",
					OXYNOTE_EMAIL_FROM_ADDRESS:
						"team@example.com",
				}),
			)

			expect(config.smtp?.tls).toBe("starttls")
		})

		it("is disabled when unset", ({ expect }) => {
			const config = loadConfig(completeEnv())

			expect(config.smtp).toBeUndefined()
			expect(config.emailFromAddress).toBe("")
		})

		it.for([
			{
				name: "a missing port",
				input: "smtp://mail.example.com",
			},
			{
				name: "an http scheme",
				input: "http://mail.example.com:587",
			},
			{
				name: "an unknown tls mode",
				input: "smtp://mail.example.com:587?tls=maybe",
			},
			{ name: "not a URL", input: "::" },
		])("rejects $name", ({ input }, { expect }) => {
			expect(() =>
				loadConfig(
					completeEnv({
						OXYNOTE_SMTP_DSN: input,
						OXYNOTE_EMAIL_FROM_ADDRESS:
							"team@example.com",
					}),
				),
			).toThrow("OXYNOTE_SMTP_DSN")
		})

		it("requires the from address alongside the relay", ({
			expect,
		}) => {
			expect(() =>
				loadConfig(
					completeEnv({
						OXYNOTE_SMTP_DSN:
							"smtp://mail.example.com:587",
					}),
				),
			).toThrow("OXYNOTE_EMAIL_FROM_ADDRESS")
		})
	})

	describe("feature groups", () => {
		it("requires the meilisearch master key with the URL", ({
			expect,
		}) => {
			expect(() =>
				loadConfig(
					completeEnv({
						OXYNOTE_MEILISEARCH_URL:
							"http://meilisearch:7700",
					}),
				),
			).toThrow("OXYNOTE_MEILISEARCH_MASTER_KEY")
		})

		it("rejects a companion without its keying variable", ({
			expect,
		}) => {
			expect(() =>
				loadConfig(
					completeEnv({
						OXYNOTE_CHANGE_DETECTION_API_KEY:
							"key",
					}),
				),
			).toThrow("OXYNOTE_CHANGE_DETECTION_API_KEY")
		})

		it("requires the github app companions with the app id", ({
			expect,
		}) => {
			expect(() =>
				loadConfig(
					completeEnv({
						OXYNOTE_GITHUB_APP_ID: "123",
					}),
				),
			).toThrow(
				/OXYNOTE_GITHUB_APP_SLUG[\s\S]*OXYNOTE_GITHUB_APP_SIGNATURE_SECRET/,
			)
		})

		it("requires the slack app companions with the client id", ({
			expect,
		}) => {
			expect(() =>
				loadConfig(
					completeEnv({
						OXYNOTE_SLACK_APP_CLIENT_ID:
							"id",
					}),
				),
			).toThrow(
				/OXYNOTE_SLACK_APP_CLIENT_SECRET[\s\S]*OXYNOTE_SLACK_APP_SIGNATURE_SECRET/,
			)
		})

		it("rejects assistant settings without a provider", ({
			expect,
		}) => {
			expect(() =>
				loadConfig(
					completeEnv({
						OXYNOTE_AI_ASSISTANT_MODEL:
							"claude-opus-5",
					}),
				),
			).toThrow("OXYNOTE_AI_ASSISTANT_MODEL")
		})

		it("assembles an enabled feature into its config group", ({
			expect,
		}) => {
			const config = loadConfig(
				completeEnv({
					OXYNOTE_MEILISEARCH_URL:
						"http://meilisearch:7700",
					OXYNOTE_MEILISEARCH_MASTER_KEY:
						"master",
					OXYNOTE_CHANGE_DETECTION_URL:
						"http://changeDetection:5000",
					OXYNOTE_GITHUB_APP_ID: "123",
					OXYNOTE_GITHUB_APP_SLUG: "oxynote",
					OXYNOTE_GITHUB_APP_SIGNATURE_SECRET:
						"sig",
					OXYNOTE_SLACK_APP_CLIENT_ID: "cid",
					OXYNOTE_SLACK_APP_CLIENT_SECRET: "cs",
					OXYNOTE_SLACK_APP_SIGNATURE_SECRET:
						"ss",
					OXYNOTE_AI_ASSISTANT_PROVIDER:
						"anthropic",
					OXYNOTE_AI_ASSISTANT_API_KEY: "sk-ant",
				}),
			)

			expect(config.meilisearch).toEqual({
				url: "http://meilisearch:7700",
				masterKey: "master",
			})
			expect(config.changeDetection).toEqual({
				url: "http://changeDetection:5000",
				apiKey: "",
			})
			expect(config.githubApp).toEqual({
				appId: "123",
				appSlug: "oxynote",
				signatureSecret: "sig",
			})
			expect(config.slackApp).toEqual({
				clientId: "cid",
				clientSecret: "cs",
				signatureSecret: "ss",
			})
			expect(config.aiAssistant.PROVIDER).toBe("anthropic")
			expect(config.aiAssistant.API_KEY).toBe("sk-ant")
			expect(config.aiAssistant.MODEL).toBe("")
		})
	})

	describe("social login", () => {
		it("registers a provider given both halves of its credentials", ({
			expect,
		}) => {
			const config = loadConfig(
				completeEnv({
					OXYNOTE_SOCIAL_LOGIN_GITHUB_CLIENT_ID:
						"id",
					OXYNOTE_SOCIAL_LOGIN_GITHUB_CLIENT_SECRET:
						"secret",
				}),
			)

			expect(config.socialLogin).toEqual({
				github: {
					clientId: "id",
					clientSecret: "secret",
				},
			})
		})

		it("rejects a provider with a single half", ({ expect }) => {
			expect(() =>
				loadConfig(
					completeEnv({
						OXYNOTE_SOCIAL_LOGIN_GOOGLE_CLIENT_ID:
							"id",
					}),
				),
			).toThrow("OXYNOTE_SOCIAL_LOGIN_GOOGLE_CLIENT_SECRET")
		})
	})

	describe("tuning", () => {
		it("passes the values through as the strings the components parse", ({
			expect,
		}) => {
			const config = loadConfig(
				completeEnv({
					OXYNOTE_MAX_ORGANIZATIONS: "10",
					OXYNOTE_MAX_ORGANIZATION_MEMBERS: "25",
					OXYNOTE_RATE_LIMIT_DISABLED: "true",
					OXYNOTE_MAX_DOCUMENT_HISTORY_ENTRIES:
						"0",
					OXYNOTE_DOCUMENT_HISTORY_RETENTION:
						"2160h",
					OXYNOTE_LOG_LEVEL: "DEBUG",
				}),
			)

			expect(config.maxOrganizations).toBe("10")
			expect(config.maxOrganizationMembers).toBe("25")
			expect(config.rateLimitDisabled).toBe(true)
			expect(config.maxDocumentHistoryEntries).toBe("0")
			expect(config.documentHistoryRetention).toBe("2160h")
			expect(config.logLevel).toBe("DEBUG")
		})

		it("defaults the log level to WARN", ({ expect }) => {
			const config = loadConfig(completeEnv({}))

			expect(config.logLevel).toBe("WARN")
		})

		it.for([
			{
				name: "a word as a counter",
				input: { OXYNOTE_MAX_ORGANIZATIONS: "many" },
				expected: "OXYNOTE_MAX_ORGANIZATIONS",
			},
			{
				name: "a number as a retention",
				input: {
					OXYNOTE_DOCUMENT_HISTORY_RETENTION:
						"90",
				},
				expected: "OXYNOTE_DOCUMENT_HISTORY_RETENTION",
			},
			{
				name: "an unknown log level",
				input: { OXYNOTE_LOG_LEVEL: "TRACE" },
				expected: "OXYNOTE_LOG_LEVEL",
			},
			{
				name: "a flag spelled 1",
				input: { OXYNOTE_RATE_LIMIT_DISABLED: "1" },
				expected: "OXYNOTE_RATE_LIMIT_DISABLED",
			},
		])("rejects $name", ({ input, expected }, { expect }) => {
			expect(() => loadConfig(completeEnv(input))).toThrow(
				expected,
			)
		})
	})

	describe("secret overrides", () => {
		it("carries the overrides through unset by default", ({
			expect,
		}) => {
			const config = loadConfig(completeEnv())

			expect(config.authSecret).toBeUndefined()
			expect(config.dataSourceEncryptionKey).toBeUndefined()
		})

		it.for([
			{ name: "16", input: "0123456789abcdef" },
			{ name: "24", input: "0123456789abcdef01234567" },
			{
				name: "32",
				input: "0123456789abcdef0123456789abcdef",
			},
		])(
			"accepts a $name byte encryption key",
			({ input }, { expect }) => {
				const config = loadConfig(
					completeEnv({
						OXYNOTE_DATA_SOURCE_ENCRYPTION_KEY:
							input,
					}),
				)

				expect(config.dataSourceEncryptionKey).toBe(
					input,
				)
			},
		)

		it("rejects an encryption key of any other length", ({
			expect,
		}) => {
			expect(() =>
				loadConfig(
					completeEnv({
						OXYNOTE_DATA_SOURCE_ENCRYPTION_KEY:
							"tooshort",
					}),
				),
			).toThrow("OXYNOTE_DATA_SOURCE_ENCRYPTION_KEY")
		})
	})
})
