import { join } from "node:path"
import {
	postgresDataDir,
	postgresDatabase,
	postgresSocketDir,
} from "./mapping.js"

// the image's own PostgreSQL, run for a deployment that sets no
// OXYNOTE_DB_DSN. Its major is fixed by the package the image installs and by
// these paths: a data directory only opens under the major that wrote it, so
// moving to another one needs an upgrade path for existing volumes.
export const binDir = "/usr/libexec/postgresql18"

// a first boot builds the cluster here and renames it into place only once
// it is complete, so a boot that dies halfway starts over instead of leaving
// a cluster without its role.
const scratchDir = `${postgresDataDir}.init`

// the access rules every server the image runs reads.
export const hbaFile = "/oxynote/prod/pg_hba.conf"

export interface DatabaseDeps {
	exists(path: string): boolean
	// removes a directory tree; a missing one is not an error.
	remove(path: string): void
	rename(from: string, to: string): void
	makeDir(path: string): void
	chmod(path: string, mode: number): void
	// runs a command to completion with the given standard input and
	// throws when it fails.
	run(command: string, args: string[], input: string): void
}

// prepareDatabase makes sure the data volume holds a cluster with the app
// role and its database, creating them on first boot, and reports whether it
// did.
export function prepareDatabase(deps: DatabaseDeps, password: string): boolean {
	deps.makeDir(postgresSocketDir)

	// the volume can gain group permissions outside the image, as kubelet's
	// fsGroup does. Postgres then refuses to start, so reset the mode.
	if (deps.exists(join(postgresDataDir, "PG_VERSION"))) {
		deps.chmod(postgresDataDir, 0o700)

		return false
	}

	deps.remove(scratchDir)
	createCluster(deps, scratchDir, password)
	deps.rename(scratchDir, postgresDataDir)

	return true
}

// createCluster builds a new cluster in dir, with the app role and its
// database.
export function createCluster(
	deps: Pick<DatabaseDeps, "run">,
	dir: string,
	password: string,
): void {
	// the bootstrap superuser gets no password, and pg_hba.conf demands one
	// from every connection, so nothing can log in as it.
	deps.run(
		`${binDir}/initdb`,
		[
			`--pgdata=${dir}`,
			"--username=postgres",
			"--auth=reject",
			"--encoding=UTF8",
			"--locale-provider=builtin",
			"--builtin-locale=C.UTF-8",
			"--no-instructions",
		],
		"",
	)

	// single-user mode exits 0 after a failed statement, and logs the
	// statement with the password in it, unless both settings say
	// otherwise. The app role owns its database and nothing else, so a
	// query it runs cannot reach a superuser's COPY ... PROGRAM.
	deps.run(
		`${binDir}/postgres`,
		[
			"--single",
			"-D",
			dir,
			"-c",
			"exit_on_error=on",
			"-c",
			"log_min_error_statement=panic",
			"postgres",
		],
		`CREATE ROLE ${postgresDatabase} LOGIN PASSWORD '${password}';\n` +
			`CREATE DATABASE ${postgresDatabase} OWNER ${postgresDatabase};\n`,
	)
}

// the server the supervisor runs. An empty listen_addresses leaves the socket
// as the only way in.
export const databaseServer = {
	command: `${binDir}/postgres`,
	args: [
		"-D",
		postgresDataDir,
		"-c",
		"listen_addresses=",
		"-c",
		`unix_socket_directories=${postgresSocketDir}`,
		"-c",
		`hba_file=${hbaFile}`,
		"-c",
		"log_checkpoints=off",
	],
}

// exits 0 once the server accepts connections, without logging in.
export const databaseReadiness = {
	command: `${binDir}/pg_isready`,
	args: ["-q", "-h", postgresSocketDir],
}
