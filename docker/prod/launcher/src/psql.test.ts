import { describe, it } from "vitest"
import { embeddedConnection, externalConnection } from "./psql.js"

describe("externalConnection", () => {
	it("hands the DSN to the client tools as their database", ({
		expect,
	}) => {
		expect(externalConnection("postgresql://db", {}).args).toEqual([
			"--dbname",
			"postgresql://db",
		])
	})
})

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
