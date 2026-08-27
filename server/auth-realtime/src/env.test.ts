import { describe, it } from "vitest"
import { loadEnv } from "./env.js"

// a complete, valid environment. Each test overrides only the variables it
// is about, so the rest can never be the reason a case passes or fails.
function completeEnv(
	overrides: Record<string, string | undefined> = {},
): Record<string, string | undefined> {
	return {
		OXYNOTE_AUTH_REALTIME_BACKEND_URL: "http://core:8080",
		OXYNOTE_AUTH_REALTIME_DB_DSN:
			"postgresql://devuser:devpass@postgres/devdb",
		OXYNOTE_AUTH_REALTIME_VALKEY_URL: "redis://valkey:6379",
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_BASE_URL:
			"http://localhost:8080/auth-realtime",
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SECRET: "sup3rs3cr3t",
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_COOKIE_DOMAIN: "localhost",
		OXYNOTE_AUTH_REALTIME_FRONTEND_URL: "http://localhost:8080",
		OXYNOTE_AUTH_REALTIME_BETTER_AUTH_ORGANIZATION_INVITATION_URL:
			"http://localhost:8080/accept-invite",
		OXYNOTE_AUTH_REALTIME_TRUSTED_ORIGINS:
			"http://localhost:8080,http://localhost:3000",
		OXYNOTE_AUTH_REALTIME_MCP_RESOURCE:
			"http://localhost:8080/core/api/mcp",
		...overrides,
	}
}

describe("loadEnv", () => {
	it("maps every required variable onto the typed configuration", ({
		expect,
	}) => {
		const env = loadEnv(completeEnv())

		expect(env.coreUrl).toBe("http://core:8080")
		expect(env.databaseDSN).toBe(
			"postgresql://devuser:devpass@postgres/devdb",
		)
		expect(env.valkeyUrl).toBe("redis://valkey:6379")
		expect(env.betterAuthSecret).toBe("sup3rs3cr3t")
		expect(env.cookieDomain).toBe("localhost")
		expect(env.frontendUrl).toBe("http://localhost:8080")
		expect(env.organizationInvitationUrl).toBe(
			"http://localhost:8080/accept-invite",
		)
		expect(env.mcpResource).toBe(
			"http://localhost:8080/core/api/mcp",
		)
	})

	it("reduces the public auth base URL to its origin", ({ expect }) => {
		const env = loadEnv(completeEnv())

		expect(env.publicAuthBaseUrl).toBe(
			"http://localhost:8080/auth-realtime",
		)
		expect(env.authOrigin).toBe("http://localhost:8080")
	})

	it("derives the MCP token issuer from the auth origin", ({
		expect,
	}) => {
		const env = loadEnv(completeEnv())

		expect(env.mcpTokenIssuer).toBe(
			"http://localhost:8080/api/auth",
		)
	})

	it.for([
		{
			name: "a missing required variable",
			input: { OXYNOTE_AUTH_REALTIME_DB_DSN: undefined },
			expected: "OXYNOTE_AUTH_REALTIME_DB_DSN",
		},
		{
			name: "a required variable present but empty",
			input: { OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SECRET: "" },
			expected: "OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SECRET",
		},
		{
			name: "a URL variable that is not a URL",
			input: {
				OXYNOTE_AUTH_REALTIME_BACKEND_URL: "core:8080",
			},
			expected: "OXYNOTE_AUTH_REALTIME_BACKEND_URL",
		},
	])("names $name in the failure", ({ input, expected }, { expect }) => {
		expect(() => loadEnv(completeEnv(input))).toThrow(expected)
	})

	it("reports every invalid variable at once rather than the first", ({
		expect,
	}) => {
		expect(() =>
			loadEnv(
				completeEnv({
					OXYNOTE_AUTH_REALTIME_DB_DSN: undefined,
					OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SECRET:
						undefined,
				}),
			),
		).toThrow(
			/OXYNOTE_AUTH_REALTIME_DB_DSN[\s\S]*OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SECRET/,
		)
	})

	it("accepts an absent valkey URL", ({ expect }) => {
		const env = loadEnv(
			completeEnv({
				OXYNOTE_AUTH_REALTIME_VALKEY_URL: undefined,
			}),
		)

		expect(env.valkeyUrl).toBeUndefined()
	})

	describe("counters", () => {
		it("falls back to the defaults when unset", ({ expect }) => {
			const env = loadEnv(completeEnv())

			expect(env.maxOrganizations).toBe(100)
			expect(env.maxOrganizationMembers).toBe(5)
		})

		it("reads the configured values", ({ expect }) => {
			const env = loadEnv(
				completeEnv({
					OXYNOTE_AUTH_REALTIME_MAX_ORGANIZATIONS:
						"50",
					OXYNOTE_AUTH_REALTIME_MAX_ORGANIZATION_MEMBERS:
						"25",
				}),
			)

			expect(env.maxOrganizations).toBe(50)
			expect(env.maxOrganizationMembers).toBe(25)
		})

		it.for([
			{ name: "a word", input: "many" },
			{ name: "a fraction", input: "2.5" },
			{ name: "zero", input: "0" },
			{ name: "a negative count", input: "-1" },
		])(
			"rejects $name as an organization limit",
			({ input }, { expect }) => {
				expect(() =>
					loadEnv(
						completeEnv({
							OXYNOTE_AUTH_REALTIME_MAX_ORGANIZATIONS:
								input,
						}),
					),
				).toThrow(
					"OXYNOTE_AUTH_REALTIME_MAX_ORGANIZATIONS",
				)
			},
		)
	})

	describe("rate limiting", () => {
		it.for([
			{ name: "unset", input: undefined, expected: true },
			{ name: '"false"', input: "false", expected: true },
			{ name: '"true"', input: "true", expected: false },
		])(
			"is enabled=$expected when the disable flag is $name",
			({ input, expected }, { expect }) => {
				const env = loadEnv(
					completeEnv({
						OXYNOTE_AUTH_REALTIME_RATE_LIMIT_DISABLED:
							input,
					}),
				)

				expect(env.rateLimitEnabled).toBe(expected)
			},
		)

		// a flag spelled any other way would read as false and leave
		// the limiter on while the operator believes it is off
		it("rejects a flag that is neither true nor false", ({
			expect,
		}) => {
			expect(() =>
				loadEnv(
					completeEnv({
						OXYNOTE_AUTH_REALTIME_RATE_LIMIT_DISABLED:
							"1",
					}),
				),
			).toThrow("OXYNOTE_AUTH_REALTIME_RATE_LIMIT_DISABLED")
		})
	})

	describe("trusted origins", () => {
		it("splits the list on commas", ({ expect }) => {
			const env = loadEnv(completeEnv())

			expect(env.trustedOrigins).toEqual([
				"http://localhost:8080",
				"http://localhost:3000",
			])
		})

		it("is empty when the variable is unset", ({ expect }) => {
			const env = loadEnv(
				completeEnv({
					OXYNOTE_AUTH_REALTIME_TRUSTED_ORIGINS:
						undefined,
				}),
			)

			expect(env.trustedOrigins).toEqual([])
		})

		it("drops the empty entries a trailing comma produces", ({
			expect,
		}) => {
			const env = loadEnv(
				completeEnv({
					OXYNOTE_AUTH_REALTIME_TRUSTED_ORIGINS:
						"http://localhost:8080,,oxynote://,",
				}),
			)

			expect(env.trustedOrigins).toEqual([
				"http://localhost:8080",
				"oxynote://",
			])
		})
	})

	describe("social providers", () => {
		it("registers none when no credentials are configured", ({
			expect,
		}) => {
			const env = loadEnv(completeEnv())

			expect(env.socialProviders).toEqual({})
		})

		it("registers a provider given both halves of its credentials", ({
			expect,
		}) => {
			const env = loadEnv(
				completeEnv({
					OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GITHUB_CLIENT_ID:
						"github-id",
					OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GITHUB_CLIENT_SECRET:
						"github-secret",
				}),
			)

			expect(env.socialProviders).toEqual({
				github: {
					clientId: "github-id",
					clientSecret: "github-secret",
				},
			})
		})

		it("registers each configured provider independently", ({
			expect,
		}) => {
			const env = loadEnv(
				completeEnv({
					OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GOOGLE_CLIENT_ID:
						"google-id",
					OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GOOGLE_CLIENT_SECRET:
						"google-secret",
					OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SLACK_CLIENT_ID:
						"slack-id",
					OXYNOTE_AUTH_REALTIME_BETTER_AUTH_SLACK_CLIENT_SECRET:
						"slack-secret",
				}),
			)

			expect(Object.keys(env.socialProviders).sort()).toEqual(
				["google", "slack"],
			)
		})

		it.for([
			{
				name: "the secret",
				input: {
					OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GOOGLE_CLIENT_ID:
						"google-id",
				},
				expected: "OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GOOGLE_CLIENT_SECRET",
			},
			{
				name: "the client id",
				input: {
					OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GOOGLE_CLIENT_SECRET:
						"google-secret",
				},
				expected: "OXYNOTE_AUTH_REALTIME_BETTER_AUTH_GOOGLE_CLIENT_ID",
			},
		])(
			"fails when google is configured without $name",
			({ input, expected }, { expect }) => {
				expect(() =>
					loadEnv(completeEnv(input)),
				).toThrow(expected)
			},
		)
	})
})
