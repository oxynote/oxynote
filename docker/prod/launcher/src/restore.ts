import {
	existsSync,
	mkdirSync,
	readdirSync,
	readFileSync,
	rmSync,
} from "node:fs"
import { join } from "node:path"
import type { Readable } from "node:stream"
import {
	databaseEntry,
	objectsDir,
	readArchive,
	refusal,
	remoteObjectsNote,
	secretsDir,
	type ImageVersion,
	type Manifest,
} from "./archive.js"
import { dumpScript } from "./dump.js"
import type { Logger } from "./logging.js"
import {
	client,
	embeddedConnection,
	type Client,
	type Connection,
} from "./psql.js"
import { run } from "./run.js"
import { generatedAsciiPattern } from "./secrets.js"
import { commit, committed, finishRestore, stagingDir } from "./staging.js"

export interface RestoreOptions {
	dataDir: string
	binDir: string
	// the database, when it is not the embedded one.
	external: Connection | undefined
	// the environment the database tools run with.
	env: Record<string, string>
	// the temporary server an embedded restore loads its dump into.
	socketDir: string
	hbaFile: string
	remoteObjects: boolean
	image: ImageVersion
	// replaces an instance on a stopped volume. Inside a running instance
	// a restore always replaces it.
	replace: boolean
	input: Readable
	inputIsTerminal: boolean
	// whether this container serves the instance the restore replaces.
	instanceRunning(): Promise<boolean>
	// stops the instance, so its runtime starts it again and the boot
	// finishes the restore.
	restartInstance(): void
	createCluster(dir: string, password: string): void
	restoreId(): string
	log: Logger
}

// restore replaces the instance with a backup read from input. It builds
// the new instance aside, and swaps it in only once nothing runs: inside a
// running container it restarts the instance and the boot finishes the
// swap, and on a stopped one it swaps at once. A failure before the commit
// point leaves the old instance as it was.
export async function restore(options: RestoreOptions): Promise<void> {
	if (options.inputIsTerminal) {
		throw new Error(
			"restore reads a backup from standard input: restore < oxynote-backup.tar",
		)
	}

	const external =
		options.external && client(options.binDir, options.external)
	const live = await options.instanceRunning()
	const staging = stagingDir(options.dataDir)

	if (live) {
		if (committed(options.dataDir)) {
			throw new Error(
				"a restore is already waiting for the instance to restart. Restart it first",
			)
		}
	} else {
		await refuseRunning(options.dataDir, external)
		await finishRestore(options.dataDir, external, options.log)

		if (
			!options.replace &&
			(await holdsInstance(options.dataDir, external))
		) {
			throw new Error(
				"this volume already holds an instance. Run restore --replace to replace it",
			)
		}
	}

	// exclusive, so a second restore stops here and leaves the staging of
	// the first alone. The next boot clears one that was interrupted.
	try {
		mkdirSync(staging, { mode: 0o700 })
	} catch {
		throw new Error(
			"another restore is running, or an interrupted one left restore/ behind. Wait for it, or restart the instance to clear it",
		)
	}

	try {
		warnAbout(options, await stage(options, staging))

		if (!external) {
			await buildCluster(options, staging)
		}
	} catch (err) {
		rmSync(staging, { recursive: true, force: true })
		throw err
	}

	commit(
		options.dataDir,
		external
			? { database: "external", id: options.restoreId() }
			: { database: "embedded" },
	)

	if (live) {
		options.log.info(
			"the backup is staged. The instance restarts now and finishes the restore",
		)
		options.restartInstance()

		return
	}

	await finishRestore(options.dataDir, external, options.log)
	options.log.info("restore complete. Start the instance")
}

async function refuseRunning(
	dataDir: string,
	external: Client | undefined,
): Promise<void> {
	if (external) {
		const others = await external.query(
			"SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND pid <> pg_backend_pid()",
		)

		if (others !== "0") {
			throw new Error(
				`the database has ${others} other connections. Run restore inside the running instance, or stop it first`,
			)
		}

		return
	}

	// a clean stop removes the file, and a crash leaves it behind.
	if (existsSync(join(dataDir, "postgres", "postmaster.pid"))) {
		throw new Error(
			"the instance is running in another container, or it did not stop cleanly. Run restore inside the running instance, or stop it first. After a crash, start and stop it once",
		)
	}
}

async function holdsInstance(
	dataDir: string,
	external: Client | undefined,
): Promise<boolean> {
	const secrets = join(dataDir, secretsDir)

	if (existsSync(secrets) && readdirSync(secrets).length > 0) {
		return true
	}

	if (external) {
		return (
			(await external.query(
				"SELECT to_regclass('public.goose_db_version') IS NOT NULL",
			)) === "t"
		)
	}

	return existsSync(join(dataDir, "postgres", "PG_VERSION"))
}

// stage reads the backup into staging. The manifest is checked before
// anything is written, and every checksum before anything is loaded.
async function stage(
	options: RestoreOptions,
	staging: string,
): Promise<Manifest> {
	const manifest = await readArchive(options.input, {
		accept(manifest) {
			const reason = refusal(manifest, options.image)

			if (reason !== undefined) {
				throw new Error(reason)
			}

			// both parts are staged even when the backup holds nothing
			// for them, so the swap also replaces what the old instance
			// kept there.
			mkdirSync(join(staging, secretsDir), { mode: 0o700 })

			if (!options.remoteObjects) {
				mkdirSync(join(staging, objectsDir), {
					mode: 0o700,
				})
			}
		},
		target(name) {
			if (
				name === databaseEntry ||
				name.startsWith(`${secretsDir}/`)
			) {
				return join(staging, name)
			}

			if (name.startsWith(`${objectsDir}/`)) {
				return options.remoteObjects
					? undefined
					: join(staging, name)
			}

			throw new Error(`it holds an unknown entry: ${name}`)
		},
	})

	if (!existsSync(join(staging, databaseEntry))) {
		throw new Error(
			"cannot read the backup: it holds no database dump",
		)
	}

	return manifest
}

function warnAbout(options: RestoreOptions, manifest: Manifest): void {
	if (options.remoteObjects) {
		options.log.info(remoteObjectsNote)
	} else if (manifest.skipped.includes(objectsDir)) {
		options.log.warn(
			"the backup holds no objects, because its instance kept them in a remote store. Uploaded files are missing until they are copied in",
		)
	}

	if (manifest.secretsFromEnv.length > 0) {
		options.log.warn(
			`the backed-up instance took ${manifest.secretsFromEnv.join(", ")} from its environment. Set the same variables on this one`,
		)
	}
}

// buildCluster loads the staged dump into a new cluster in staging, on a
// temporary server of its own, so it can run beside the live one.
async function buildCluster(
	options: RestoreOptions,
	staging: string,
): Promise<void> {
	// the cluster takes the backed-up password, because the restored
	// secrets are what core logs in with. The password reaches SQL as the
	// superuser, so it must have the shape the image generates.
	const passwordPath = join(staging, secretsDir, "database-password")
	const password = existsSync(passwordPath)
		? readFileSync(passwordPath, "utf8").trim()
		: ""

	if (!generatedAsciiPattern.test(password)) {
		throw new Error(
			"cannot read the backup: it holds no database password this image generates",
		)
	}

	const cluster = join(staging, "postgres")
	const pgCtl = join(options.binDir, "pg_ctl")
	const env = options.env

	options.createCluster(cluster, password)
	mkdirSync(options.socketDir, { recursive: true, mode: 0o700 })

	options.log.info("loading the database")
	await run(
		pgCtl,
		[
			"start",
			"-w",
			"-D",
			cluster,
			"-l",
			join(staging, "postgres.log"),
			"-o",
			`-c listen_addresses= -c unix_socket_directories=${options.socketDir} -c hba_file=${options.hbaFile}`,
		],
		{ env },
	)

	try {
		await client(
			options.binDir,
			embeddedConnection(options.socketDir, password, env),
		).load(dumpScript(join(staging, databaseEntry)))
	} finally {
		await run(pgCtl, ["stop", "-w", "-m", "fast", "-D", cluster], {
			env,
		})
	}

	rmSync(join(staging, databaseEntry))
	rmSync(join(staging, "postgres.log"), { force: true })
}
