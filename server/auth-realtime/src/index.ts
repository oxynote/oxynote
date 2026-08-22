import { serve } from "@hono/node-server"
import { Hono } from "hono"
import { createNodeWebSocket } from "@hono/node-ws"
import { auth } from "./auth.js"
import { hocuspocus } from "./hocuspocus.js"
import routes from "./routes.js"

const app = new Hono()
const { injectWebSocket, upgradeWebSocket } = createNodeWebSocket({ app })

app.route("/api", routes)

// the RFC 9728 protected-resource metadata for the MCP surface is served
// by the mcp plugin's request hook, which matches root-anchored
// well-known paths — the request must reach better-auth with its
// original path, unlike the basePath'd /api/auth endpoints.
app.on(
	["GET", "HEAD"],
	[
		"/.well-known/oauth-protected-resource",
		"/.well-known/oauth-protected-resource/*",
	],
	(c) => auth.handler(c.req.raw),
)
app.get(
	"/hocuspocus",
	upgradeWebSocket((c) => ({
		onOpen(_evt, ws) {
			hocuspocus.handleConnection(ws.raw, c.req.raw as any)
		},
	})),
)

const server = serve(
	{
		fetch: app.fetch,
		port: 8081,
	},
	(info) => {
		hocuspocus.hooks("onListen", {
			instance: hocuspocus,
			configuration: hocuspocus.configuration,
			port: info.port,
		})

		console.log(
			`Server is running on http://localhost:${info.port}`,
		)
	},
)
injectWebSocket(server)

// graceful shutdown
process.on("SIGINT", () => {
	server.close()
	process.exit(0)
})

process.on("SIGTERM", () => {
	server.close((err) => {
		if (err) {
			console.error(err)
			process.exit(1)
		}

		process.exit(0)
	})
})
