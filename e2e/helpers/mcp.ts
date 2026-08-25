import { createHash, randomUUID } from "node:crypto"
import { Client } from "@modelcontextprotocol/sdk/client/index.js"
import { StreamableHTTPClientTransport } from "@modelcontextprotocol/sdk/client/streamableHttp.js"
import { expect, type APIRequestContext, type Page } from "@playwright/test"
import { BASE_URL, MCP_REDIRECT_URL, MCP_URL } from "./config"
import { t } from "./i18n"
import { visit } from "./page"

interface AuthorizationServer {
	authorization_endpoint: string
	token_endpoint: string
	registration_endpoint: string
}

// authorizeMCPClient walks the whole flow a real MCP client walks:
// discover the authorization server, register for a client id (RFC
// 7591), send the signed-in user through the consent screen, and trade
// the resulting code for a token with PKCE. Nothing is minted behind the
// product's back, so the token that comes back carries whatever
// organization the user actually granted it for.
export async function authorizeMCPClient(
	page: Page,
	request: APIRequestContext,
	scope = "documents:read",
): Promise<string> {
	const metadata = await request.get(
		`${BASE_URL}/.well-known/oauth-authorization-server`,
	)
	expect(metadata.ok()).toBe(true)

	const server = (await metadata.json()) as AuthorizationServer

	const registration = await request.post(server.registration_endpoint, {
		data: {
			client_name: `e2e-${randomUUID()}`,
			application_type: "native",
			redirect_uris: [MCP_REDIRECT_URL],
			grant_types: ["authorization_code"],
			response_types: ["code"],
			token_endpoint_auth_method: "none",
		},
	})
	expect(registration.ok()).toBe(true)

	const client = (await registration.json()) as { client_id: string }

	const verifier = `${randomUUID()}${randomUUID()}`
	const challenge = createHash("sha256").update(verifier).digest("base64url")

	const authorize = new URL(server.authorization_endpoint)
	authorize.searchParams.set("response_type", "code")
	authorize.searchParams.set("client_id", client.client_id)
	authorize.searchParams.set("redirect_uri", MCP_REDIRECT_URL)
	authorize.searchParams.set("scope", scope)
	authorize.searchParams.set("code_challenge", challenge)
	authorize.searchParams.set("code_challenge_method", "S256")
	authorize.searchParams.set("state", randomUUID())
	authorize.searchParams.set("resource", MCP_URL)

	await visit(page, authorize.toString())
	await page.getByRole("button", { name: t("oauth.consent.approve") }).click()
	await page.waitForURL(/[?&]code=/)

	const code = new URL(page.url()).searchParams.get("code")
	expect(code).not.toBeNull()

	const token = await request.post(server.token_endpoint, {
		form: {
			grant_type: "authorization_code",
			code: code ?? "",
			redirect_uri: MCP_REDIRECT_URL,
			client_id: client.client_id,
			code_verifier: verifier,
			resource: MCP_URL,
		},
	})
	expect(token.ok()).toBe(true)

	const granted = (await token.json()) as { access_token: string }

	return granted.access_token
}

// connectMCPClient points the official MCP client at core's surface with
// the given bearer token, which is what a third-party client does with
// the token the flow above produced.
export async function connectMCPClient(token: string): Promise<Client> {
	const client = new Client({ name: "oxynote-e2e", version: "1.0.0" })

	await client.connect(
		new StreamableHTTPClientTransport(new URL(MCP_URL), {
			requestInit: {
				headers: { authorization: `Bearer ${token}` },
			},
		}),
	)

	return client
}

// documentResourceURI is how the mcp surface addresses a document.
export function documentResourceURI(id: string): string {
	return `oxynote://documents/${id}`
}
