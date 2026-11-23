import { describe, it } from "vitest"
import { embeddedConnection } from "./psql.js"

describe("embeddedConnection", () => {
	it("logs in as the app role through the socket", ({ expect }) => {
		const connection = embeddedConnection("/sock", "secret", {
			PATH: "/bin",
		})

		expect(connection.args).toEqual([
			"-h",
			"/sock",
			"-U",
			"oxynote",
			"oxynote",
		])
		expect(connection.env).toEqual({
			PATH: "/bin",
			PGPASSWORD: "secret",
		})
	})
})
