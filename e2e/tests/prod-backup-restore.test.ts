import { createHash, randomUUID } from "node:crypto"
import { expect, test, type Page } from "@playwright/test"
import { submitLoginForm, type Credentials } from "../helpers/auth"
import {
	BACKUP_URL,
	documentXML,
	hasStagedRestore,
	killRestore,
	liveRestore,
	restartCount,
	resetBackupVolumes,
	runRestore,
	snapshot,
	startInstance,
	stopInstance,
	takeBackup,
	volumeHash,
	waitForRestart,
	type Snapshot,
} from "../helpers/backup"
import {
	contentEditor,
	createDocument,
	documentPersisted,
	insertFileBlock,
	titleEditor,
	waitForEditor,
} from "../helpers/editor"
import { authorizeMCPClient, connectMCPClient } from "../helpers/mcp"
import { visit } from "../helpers/page"
import { signUpWithWorkspaceWithoutEmail } from "../helpers/without-email"

// the attachment the source instance holds, compared byte for byte after
// the restore.
const FILE = {
	name: "runbook.txt",
	mimeType: "text/plain",
	buffer: Buffer.from(`restore drill ${randomUUID()}`),
}

// a word only the source document carries, so a search hit proves the
// restored index was rebuilt from the restored database. It is the page's
// whole title, and short enough for the URL to carry it uncut.
const WORD = `drill${randomUUID().slice(0, 8)}`

// the tests share one source instance, its backup and the volume it is
// restored into, so they run in order and without retries.
let backup: Buffer = Buffer.alloc(0)
let source: Snapshot | undefined
let sourceXML: Record<string, string> = {}
let credentials: Credentials = { email: "", password: "" }
let documentUrl = ""
let dataSourceId = ""

test.describe("backing up and restoring the image", () => {
	test.describe.configure({ mode: "serial", retries: 0 })

	// every instance in the file boots from cold at least once, and a boot
	// takes up to a minute.
	test.setTimeout(300_000)

	test.beforeAll(resetBackupVolumes)

	test.afterAll(resetBackupVolumes)

	test("restores a backup into a fresh install as the same instance", async ({
		page,
		request,
		browser,
	}) => {
		await startInstance("oxynote-backup")

		const signedUp = await signUpWithWorkspaceWithoutEmail(page, BACKUP_URL)

		credentials = signedUp.credentials

		await createDocument(page)
		await titleEditor(page).click()
		await page.keyboard.press("ControlOrMeta+A")
		await page.keyboard.type(WORD)
		await expect(page).toHaveURL(new RegExp(`${WORD}-[a-z0-9]{20}$`))
		documentUrl = page.url()

		await insertFileBlock(page, FILE)
		await expect(fileCard(page)).toContainText(FILE.name)
		await documentPersisted(page)

		// the settings form for data sources has no e2e path yet, and the
		// test needs only the stored credential the form would send. Core
		// reports a credential it cannot decrypt before it dials, so a
		// working connection after the restore proves the key came back.
		const created = await page.request.post(
			`${BACKUP_URL}/core/api/data-sources`,
			{
				headers: { origin: BACKUP_URL },
				data: {
					type: "prometheus",
					name: "Restore drill",
					url: "http://prometheus:9090",
					credentials: {
						kind: "basic",
						username: "drill",
						password: randomUUID(),
					},
				},
			},
		)
		expect(created.ok(), await created.text()).toBe(true)
		dataSourceId = ((await created.json()) as { id: string }).id

		const client = await connectMCPClient(
			await authorizeMCPClient(page, request, "documents:read", BACKUP_URL),
			BACKUP_URL,
		)
		sourceXML = await documentXML(client)
		await client.close()

		source = await snapshot("oxynote-backup")
		backup = await takeBackup("oxynote-backup")
		await stopInstance("oxynote-backup")

		// a fresh install, restored the way the README says.
		await startInstance("oxynote-restored")
		const before = await restartCount("oxynote-restored")
		const restored = await liveRestore("oxynote-restored", backup)
		expect(restored.code, restored.stderr).toBe(0)

		await waitForRestart("oxynote-restored", before)
		expect(await snapshot("oxynote-restored")).toEqual(source)

		const context = await browser.newContext()
		const reader = await context.newPage()

		await submitLoginForm(reader, credentials, BACKUP_URL)
		await expect(reader).not.toHaveURL(/\/login/, { timeout: 15_000 })

		await visit(reader, documentUrl)
		await waitForEditor(reader)
		await expect(titleEditor(reader)).toContainText(WORD)

		const href = await fileCard(reader).getAttribute("href")
		const download = await reader.request.get(
			new URL(href ?? "", BACKUP_URL).href,
		)
		expect(download.ok()).toBe(true)
		expect(sha256(await download.body())).toBe(sha256(FILE.buffer))

		await expect
			.poll(
				async () => {
					const found = await reader.request.get(
						`${BACKUP_URL}/core/api/documents/search?q=${WORD}`,
					)

					return JSON.stringify(await found.json())
				},
				{ message: "the rebuilt index finds the document" },
			)
			.toContain(new URL(documentUrl).pathname.slice(-20))

		const connection = await reader.request.get(
			`${BACKUP_URL}/core/api/data-sources/${dataSourceId}/connection`,
		)
		expect(connection.ok()).toBe(true)
		expect(await connection.json()).toEqual({ status: "success" })

		const restoredClient = await connectMCPClient(
			await authorizeMCPClient(reader, request, "documents:read", BACKUP_URL),
			BACKUP_URL,
		)
		expect(await documentXML(restoredClient)).toEqual(sourceXML)
		await restoredClient.close()
		await context.close()
	})

	test("leaves a running instance alone when the backup is damaged", async () => {
		const before = await restartCount("oxynote-restored")
		const refused = await liveRestore(
			"oxynote-restored",
			backup.subarray(0, Math.floor(backup.length / 2)),
		)

		expect(refused.code).not.toBe(0)
		expect(refused.stderr).toContain("cannot read the backup")
		expect(await restartCount("oxynote-restored")).toBe(before)
		expect((await snapshot("oxynote-restored")).branchContent).toBe(
			source?.branchContent,
		)
	})

	test("refuses a restore from another container while the instance runs", async () => {
		const refused = await runRestore("oxynote-restored", backup, ["--replace"])

		expect(refused.code).not.toBe(0)
		expect(refused.stderr).toContain(
			"the instance is running in another container",
		)
	})

	test("refuses to replace an instance without --replace", async () => {
		await stopInstance("oxynote-restored")

		const before = await volumeHash("oxynote-restored")
		const refused = await runRestore("oxynote-restored", backup)

		expect(refused.code).not.toBe(0)
		expect(refused.stderr).toContain(
			"this volume already holds an instance. Run restore --replace to replace it",
		)
		expect(await volumeHash("oxynote-restored")).toBe(before)
	})

	const damaged = [
		{
			name: "a truncated backup",
			input: () => backup.subarray(0, Math.floor(backup.length / 2)),
			expected: "cannot read the backup",
		},
		{
			name: "a backup with one flipped byte in its dump",
			input: () => flipped(backup, "database.sql.gz"),
			expected: "database.sql.gz does not match its checksum",
		},
		{
			name: "a backup from a newer image",
			input: () => newerMigration(backup),
			expected: "newer than this image's",
		},
	]

	for (const c of damaged) {
		test(`leaves the volume unchanged when given ${c.name}`, async () => {
			const before = await volumeHash("oxynote-restored")
			const refused = await runRestore("oxynote-restored", c.input(), [
				"--replace",
			])

			expect(refused.code).not.toBe(0)
			expect(refused.stderr).toContain(c.expected)
			expect(await volumeHash("oxynote-restored")).toBe(before)
		})
	}

	test("leaves the volume unchanged when a restore is killed, and drops its leftovers at the next boot", async () => {
		const before = await volumeHash("oxynote-restored")

		await killRestore("oxynote-restored", backup)

		expect(await volumeHash("oxynote-restored")).toBe(before)
		expect(await hasStagedRestore("oxynote-restored")).toBe(true)

		await startInstance("oxynote-restored")
		expect((await snapshot("oxynote-restored")).branchContent).toBe(
			source?.branchContent,
		)
		await stopInstance("oxynote-restored")
		expect(await hasStagedRestore("oxynote-restored")).toBe(false)
	})

	const externals = [
		{ name: "PostgreSQL 18", input: "oxynote-external" },
		{ name: "PostgreSQL 16", input: "oxynote-external-16" },
	] as const

	for (const c of externals) {
		test(`restores into an external ${c.name} another role owns, and backs it up from there`, async ({
			page,
		}) => {
			await startInstance(c.input)

			const busy = await runRestore(c.input, backup, ["--replace"])

			expect(busy.code).not.toBe(0)
			expect(busy.stderr).toContain("other connections")

			const before = await restartCount(c.input)
			const restored = await liveRestore(c.input, backup)
			expect(restored.code, restored.stderr).toBe(0)

			await waitForRestart(c.input, before)
			await submitLoginForm(page, credentials, BACKUP_URL)
			await expect(page).not.toHaveURL(/\/login/, { timeout: 15_000 })
			await visit(page, documentUrl)
			await waitForEditor(page)
			await expect(titleEditor(page)).toContainText(WORD)

			const external = await takeBackup(c.input)

			expect(external.toString("latin1")).toContain('"database":"external"')
			await stopInstance(c.input)
		})
	}

	test("restores a volume that the last release created and this image upgraded", async ({
		page,
	}) => {
		await startInstance("oxynote-legacy")

		const legacy = await signUpWithWorkspaceWithoutEmail(page, BACKUP_URL)

		await createDocument(page)
		const legacyUrl = page.url()

		await contentEditor(page).click()
		await page.keyboard.type(WORD)
		await documentPersisted(page)
		await stopInstance("oxynote-legacy")

		await startInstance("oxynote-upgraded")
		const upgraded = await snapshot("oxynote-upgraded")
		const upgradedBackup = await takeBackup("oxynote-upgraded")
		await stopInstance("oxynote-upgraded")

		const restored = await runRestore("oxynote-restored", upgradedBackup, [
			"--replace",
		])
		expect(restored.code, restored.stderr).toBe(0)

		await startInstance("oxynote-restored")
		expect(await snapshot("oxynote-restored")).toEqual(upgraded)

		await page.context().clearCookies()
		await submitLoginForm(page, legacy.credentials, BACKUP_URL)
		await expect(page).not.toHaveURL(/\/login/, { timeout: 15_000 })
		await visit(page, legacyUrl)
		await waitForEditor(page)
		await expect(contentEditor(page)).toContainText(WORD)
		await stopInstance("oxynote-restored")
	})
})

function fileCard(page: Page) {
	return contentEditor(page).locator('[data-type="fileBlock"] a')
}

function sha256(bytes: Buffer): string {
	return createHash("sha256").update(bytes).digest("hex")
}

// flipped returns a copy of a backup with one byte of an entry's body
// changed. The body starts in the block after the entry's header.
function flipped(tar: Buffer, entry: string): Buffer {
	const copy = Buffer.from(tar)
	const at = copy.indexOf(entry) + 512 + 100

	copy[at] = (copy[at] ?? 0) ^ 0xff

	return copy
}

// newerMigration rewrites the manifest's migration version to all nines,
// keeping its length, so the tar stays readable and only the version gate
// can refuse it.
function newerMigration(tar: Buffer): Buffer {
	const copy = Buffer.from(tar)
	const key = '"migrationVersion":'
	const start = copy.indexOf(key) + key.length
	let end = start

	while (
		copy[end] !== undefined &&
		/\d/.test(String.fromCharCode(copy[end] ?? 0))
	) {
		end++
	}

	copy.fill("9", start, end)

	return copy
}
