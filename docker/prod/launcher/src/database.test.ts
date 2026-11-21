import { describe, it } from "vitest"
import {
	databaseServer,
	prepareDatabase,
	type DatabaseDeps,
} from "./database.js"

// a filesystem and process runner that record what was asked of them, in
// order. existing names the paths already there; a run of the command ending
// in failing throws.
function recorder(existing: string[] = [], failing?: string) {
	const steps: string[] = []
	const inputs: string[] = []

	const deps: DatabaseDeps = {
		exists: (path) => existing.includes(path),
		remove: (path) => {
			steps.push(`remove ${path}`)
		},
		rename: (from, to) => {
			steps.push(`rename ${from} ${to}`)
		},
		makeDir: (path) => {
			steps.push(`makeDir ${path}`)
		},
		chmod: (path, mode) => {
			steps.push(`chmod ${path} ${mode.toString(8)}`)
		},
		run: (command, args, input) => {
			steps.push(`run ${command} ${args.join(" ")}`)
			inputs.push(input)

			if (
				failing !== undefined &&
				command.endsWith(failing)
			) {
				throw new Error(`${failing} failed`)
			}
		},
	}

	return { deps, steps, inputs }
}

describe("prepareDatabase", () => {
	it("builds a cluster aside and moves it into place on first boot", ({
		expect,
	}) => {
		const r = recorder()

		const created = prepareDatabase(r.deps, "generated-password")

		expect(created).toBe(true)
		expect(
			r.steps.map((step) =>
				step.split(" ").slice(0, 2).join(" "),
			),
		).toEqual([
			"makeDir /tmp/postgresql",
			"remove /oxynote/data/postgres.init",
			"run /usr/libexec/postgresql18/initdb",
			"run /usr/libexec/postgresql18/postgres",
			"rename /oxynote/data/postgres.init",
		])
		expect(r.steps.at(-1)).toBe(
			"rename /oxynote/data/postgres.init /oxynote/data/postgres",
		)
	})

	// pg_hba.conf demands a password from every connection, so a role
	// without one cannot log in at all
	it("gives only the app role a password, and the app role only its database", ({
		expect,
	}) => {
		const r = recorder()

		prepareDatabase(r.deps, "generated-password")

		expect(r.steps[2]).toContain("--username=postgres")
		expect(r.inputs[0]).toBe("")
		expect(r.inputs[1]).toBe(
			"CREATE ROLE oxynote LOGIN PASSWORD 'generated-password';\n" +
				"CREATE DATABASE oxynote OWNER oxynote;\n",
		)
	})

	// without these single-user mode exits 0 after a failed statement and
	// logs the statement, password included
	it("fails on a bootstrap error without logging the password", ({
		expect,
	}) => {
		const r = recorder()

		prepareDatabase(r.deps, "generated-password")

		expect(r.steps[3]).toContain("-c exit_on_error=on")
		expect(r.steps[3]).toContain("-c log_min_error_statement=panic")
	})

	// kubelet's fsGroup adds group permissions to the volume, and Postgres
	// refuses to start on a group-writable data directory
	it("leaves an existing cluster alone apart from its mode", ({
		expect,
	}) => {
		const r = recorder(["/oxynote/data/postgres/PG_VERSION"])

		const created = prepareDatabase(r.deps, "generated-password")

		expect(created).toBe(false)
		expect(r.steps).toEqual([
			"makeDir /tmp/postgresql",
			"chmod /oxynote/data/postgres 700",
		])
	})

	it("never moves a cluster that failed to build into place", ({
		expect,
	}) => {
		const r = recorder([], "initdb")

		expect(() =>
			prepareDatabase(r.deps, "generated-password"),
		).toThrow("initdb failed")
		expect(r.steps.some((step) => step.startsWith("rename"))).toBe(
			false,
		)
	})
})

describe("databaseServer", () => {
	it("listens on its socket alone, under the image's access rules", ({
		expect,
	}) => {
		expect(databaseServer.args).toContain("listen_addresses=")
		expect(databaseServer.args).toContain(
			"unix_socket_directories=/tmp/postgresql",
		)
		expect(databaseServer.args).toContain(
			"hba_file=/oxynote/prod/pg_hba.conf",
		)
	})
})
