import { join } from "node:path"
import type { Readable, Writable } from "node:stream"
import { postgresDatabase } from "./mapping.js"
import { run } from "./run.js"

// how the client tools reach a database: their connection arguments, and
// the environment that carries the password.
export interface Connection {
	args: string[]
	env: Record<string, string>
}

// the app role on a server the image runs, reached through its socket.
export function embeddedConnection(
	socketDir: string,
	password: string,
	env: Record<string, string>,
): Connection {
	return {
		args: [
			"-h",
			socketDir,
			"-U",
			postgresDatabase,
			postgresDatabase,
		],
		env: { ...env, PGPASSWORD: password },
	}
}

export interface Client {
	// runs one statement and returns its value.
	query(sql: string): Promise<string>
	// writes a plain dump without owners or grants, so it loads into a
	// database any role owns.
	dump(output: Writable): Promise<void>
	// runs a script that stops at the first error. A script that opens a
	// transaction and fails before its commit changes nothing, because the
	// server rolls back when psql disconnects.
	load(script: Readable): Promise<void>
}

export function client(binDir: string, connection: Connection): Client {
	const psql = join(binDir, "psql")
	const options = { env: connection.env }

	return {
		async query(sql) {
			const out = await run(
				psql,
				[
					"-X",
					"-A",
					"-t",
					"-v",
					"ON_ERROR_STOP=1",
					"-c",
					sql,
					...connection.args,
				],
				options,
			)

			return out.trim()
		},
		async dump(output) {
			await run(
				join(binDir, "pg_dump"),
				[
					"--format=plain",
					"--no-owner",
					"--no-acl",
					...connection.args,
				],
				{ ...options, output },
			)
		},
		async load(script) {
			await run(
				psql,
				[
					"-X",
					"-q",
					"-v",
					"ON_ERROR_STOP=1",
					...connection.args,
				],
				{ ...options, input: script },
			)
		},
	}
}
