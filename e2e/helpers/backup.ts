import { spawn } from "node:child_process"
import type { Client } from "@modelcontextprotocol/sdk/client/index.js"
import { expect } from "@playwright/test"
import { composeArgs, composeCwd } from "./stack"

// the address every instance of the backup profile serves, one at a time
// (docker-compose.prod.yaml). Changing the port means changing those
// services.
export const BACKUP_URL = "http://localhost:19085"

export type BackupService =
	| "oxynote-backup"
	| "oxynote-restored"
	| "oxynote-legacy"
	| "oxynote-upgraded"
	| "oxynote-external"
	| "oxynote-external-16"

const SERVICES = [
	"oxynote-backup",
	"oxynote-restored",
	"oxynote-legacy",
	"oxynote-upgraded",
	"oxynote-external",
	"oxynote-external-16",
	"backup-postgres",
	"backup-postgres-16",
	"prometheus",
]

const VOLUMES = [
	"backup-source",
	"backup-restored",
	"backup-legacy",
	"backup-external",
	"backup-external-16",
]

export interface CommandResult {
	code: number
	stdout: Buffer
	stderr: string
}

// compose runs one compose command against the prod stack with the backup
// profile on. It never rejects on a nonzero exit, because a refused
// restore is the outcome several tests wait for.
function compose(args: string[], input?: Buffer): Promise<CommandResult> {
	return command(
		"docker",
		[...composeArgs(), "--profile", "backup", ...args],
		input,
	)
}

function command(
	program: string,
	args: string[],
	input?: Buffer,
): Promise<CommandResult> {
	const child = spawn(program, args, {
		cwd: composeCwd,
		stdio: "pipe",
	})
	const chunks: Buffer[] = []
	let stderr = ""

	child.stdout.on("data", (chunk: Buffer) => chunks.push(chunk))
	child.stderr.setEncoding("utf8")
	child.stderr.on("data", (chunk: string) => {
		stderr += chunk
	})

	// a refused restore closes its input early. The exit code is the
	// answer, so the broken pipe is not.
	child.stdin.on("error", () => undefined)
	child.stdin.end(input)

	return new Promise((resolve, reject) => {
		child.on("error", reject)
		child.on("close", (code) => {
			resolve({ code: code ?? -1, stdout: Buffer.concat(chunks), stderr })
		})
	})
}

function succeeded(result: CommandResult): void {
	expect(result.code, result.stderr).toBe(0)
}

// resetBackupVolumes removes the profile's containers and volumes, so a run
// never starts from what an interrupted one left behind.
export async function resetBackupVolumes(): Promise<void> {
	succeeded(
		await compose(["rm", "--stop", "--force", "--volumes", ...SERVICES]),
	)

	const project = "oxynote-e2e-prod"

	await command("docker", [
		"volume",
		"rm",
		"--force",
		...VOLUMES.map((v) => `${project}_${v}`),
	])
}

export async function startInstance(service: BackupService): Promise<void> {
	succeeded(await compose(["up", "--detach", "--wait", service]))
}

export async function stopInstance(service: BackupService): Promise<void> {
	succeeded(await compose(["stop", service]))
}

// takeBackup runs the README's backup command against a running instance.
export async function takeBackup(service: BackupService): Promise<Buffer> {
	const result = await compose(["exec", "-T", service, "backup"])

	succeeded(result)

	return result.stdout
}

// liveRestore runs the README's restore command inside a running instance.
// It returns once the restore has staged the backup, before the restart.
export function liveRestore(
	service: BackupService,
	backup: Buffer,
): Promise<CommandResult> {
	return compose(["exec", "-T", service, "restore"], backup)
}

// restartCount says how often the runtime has restarted the instance's
// container, so a test can wait for the restart a live restore causes.
export async function restartCount(service: BackupService): Promise<number> {
	const inspect = await command("docker", [
		"inspect",
		"--format",
		"{{.RestartCount}}",
		await containerId(service),
	])

	succeeded(inspect)

	return Number(inspect.stdout.toString().trim())
}

// waitForRestart waits until the instance has restarted since before and
// is healthy again.
export async function waitForRestart(
	service: BackupService,
	before: number,
): Promise<void> {
	// the instance first shuts down in order, which takes up to a minute.
	await expect
		.poll(() => restartCount(service), {
			message: `${service} restarts after the restore`,
			timeout: 120_000,
		})
		.toBeGreaterThan(before)
	await startInstance(service)
}

async function containerId(service: BackupService): Promise<string> {
	const ps = await compose(["ps", "--quiet", service])

	succeeded(ps)

	return ps.stdout.toString().trim()
}

// runRestore runs restore on a stopped instance's volume, the way an
// instance that no longer starts is restored.
export function runRestore(
	service: BackupService,
	backup: Buffer,
	flags: string[] = [],
): Promise<CommandResult> {
	return compose(
		["run", "--rm", "--no-deps", "-T", service, "restore", ...flags],
		backup,
	)
}

// killRestore starts a restore and kills its container outright while the
// restore is still staging, the way a lost node or an OOM kill would. It
// sends half the backup and holds the rest back, so the kill always lands
// before the restore could commit.
export async function killRestore(
	service: BackupService,
	backup: Buffer,
): Promise<void> {
	const name = `${service}-restore-killed`
	const child = spawn(
		"docker",
		[
			...composeArgs(),
			"--profile",
			"backup",
			"run",
			"--rm",
			"--no-deps",
			"-T",
			"--name",
			name,
			service,
			"restore",
			"--replace",
		],
		{ cwd: composeCwd, stdio: ["pipe", "ignore", "ignore"] },
	)
	const closed = new Promise((resolve) => child.on("close", resolve))

	child.stdin.on("error", () => undefined)
	child.stdin.write(backup.subarray(0, Math.floor(backup.length / 2)))

	await expect
		.poll(
			async () =>
				(
					await command("docker", [
						"exec",
						name,
						"test",
						"-e",
						"/oxynote/data/restore",
					])
				).code,
			{ message: "the restore starts staging" },
		)
		.toBe(0)

	succeeded(await command("docker", ["kill", "--signal", "KILL", name]))
	child.stdin.destroy()
	await closed
	await command("docker", ["rm", "--force", name])
}

// volumeHash fingerprints every file on a stopped instance's volume, apart
// from a restore's staging directory, from a one-off container.
export async function volumeHash(service: BackupService): Promise<string> {
	const result = await compose([
		"run",
		"--rm",
		"--no-deps",
		"-T",
		"--entrypoint",
		"sh",
		service,
		"-c",
		"cd /oxynote/data && find . -path ./restore -prune -o -type f -print | sort | xargs sha256sum",
	])

	succeeded(result)

	return result.stdout.toString()
}

// hasStagedRestore says whether a restore's staging directory is on the
// volume of a stopped instance.
export async function hasStagedRestore(
	service: BackupService,
): Promise<boolean> {
	const result = await compose([
		"run",
		"--rm",
		"--no-deps",
		"-T",
		"--entrypoint",
		"test",
		service,
		"-e",
		"/oxynote/data/restore",
	])

	return result.code === 0
}

async function sql(service: BackupService, query: string): Promise<string> {
	const result = await compose([
		"exec",
		"-T",
		service,
		"sh",
		"-c",
		'PGPASSWORD="$(cat /oxynote/data/secrets/database-password)" psql -X -A -t -h /tmp/postgresql -U oxynote oxynote -c "$1"',
		"sh",
		query,
	])

	succeeded(result)

	return result.stdout.toString().trim()
}

export interface Snapshot {
	rowCounts: Record<string, string>
	branchContent: string
	objects: string
}

// snapshot reads what a running instance holds, for comparing an instance
// with its restored copy: the row count of every table, a hash of every
// branch's stored content, and the hash of every uploaded object.
export async function snapshot(service: BackupService): Promise<Snapshot> {
	const counts = await sql(
		service,
		"SELECT table_name || '=' || (xpath('/row/c/text()', query_to_xml(format('SELECT count(*) AS c FROM public.%I', table_name), false, true, '')))[1]::text FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE' ORDER BY table_name",
	)
	const rowCounts = Object.fromEntries(
		counts.split("\n").map((line) => line.split("=")),
	) as Record<string, string>

	const branchContent = await sql(
		service,
		"SELECT md5(string_agg(id || ':' || md5(coalesce(raw_content::text, '')), ',' ORDER BY id)) FROM document_branches",
	)

	const objects = await compose([
		"exec",
		"-T",
		service,
		"sh",
		"-c",
		"cd /oxynote/data/object-storage && find . -type f | sort | xargs sha256sum",
	])

	succeeded(objects)

	return { rowCounts, branchContent, objects: objects.stdout.toString() }
}

// documentXML reads every document the client can see through
// get_document, keyed by document id.
export async function documentXML(
	client: Client,
): Promise<Record<string, string>> {
	const listing = await client.callTool({
		name: "list_documents",
		arguments: {},
	})
	const { documents } = JSON.parse(
		(listing.content as { text: string }[])[0]?.text ?? "{}",
	) as {
		documents: { id: string; branches: { id: string; default: boolean }[] }[]
	}
	const xml: Record<string, string> = {}

	for (const document of documents) {
		const branch = document.branches.find((b) => b.default)
		const read = await client.callTool({
			name: "get_document",
			arguments: { document_id: document.id, branch_id: branch?.id },
		})

		expect(read.isError, JSON.stringify(read.content)).toBeFalsy()
		xml[document.id] = (read.content as { text: string }[])[0]?.text ?? ""
	}

	return xml
}
