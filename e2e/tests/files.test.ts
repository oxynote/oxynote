import { readFile } from "node:fs/promises"
import { expect, test, type Locator, type Page } from "@playwright/test"
import {
	contentEditor,
	createDocument,
	documentPersisted,
	openSlashMenu,
	waitForEditor,
} from "../helpers/editor"
import { t } from "../helpers/i18n"
import { visit } from "../helpers/page"
import { signUpWithWorkspace } from "../helpers/workspace"

// a zip's magic bytes followed by padding to exactly 2 KB, so the server
// detects an archive and the card shows a round size
const ZIP_BYTES = Buffer.concat([
	Buffer.from([0x50, 0x4b, 0x03, 0x04]),
	Buffer.alloc(2044, 0x41),
])

const TEXT = "hello from the attachment"

// insertFileBlock adds a file block from the slash menu and answers the
// picker it opens with the given file
async function insertFileBlock(
	page: Page,
	file: { name: string; mimeType: string; buffer: Buffer },
): Promise<void> {
	await contentEditor(page).click()
	const menu = await openSlashMenu(page)
	await page.keyboard.type("File")

	// typing narrows the menu down to the one matching command
	await expect(menu.getByRole("button")).toHaveCount(1)
	await expect(menu.getByRole("button")).toContainText("File")
	await page.keyboard.press("Enter")

	const chooser = page.waitForEvent("filechooser")
	await contentEditor(page).getByText(t("editor.file.description")).click()
	await (await chooser).setFiles(file)
}

// fileCard is the uploaded attachment's card, which is what the reader
// clicks
function fileCard(page: Page): Locator {
	return contentEditor(page).locator('[data-type="fileBlock"] a')
}

test.describe("file attachments", () => {
	test("uploads a file, keeps it across a reload and downloads it under its name", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)
		const url = page.url()

		await insertFileBlock(page, {
			name: "notes.zip",
			mimeType: "application/zip",
			buffer: ZIP_BYTES,
		})

		await expect(fileCard(page)).toContainText("notes.zip")
		await expect(fileCard(page)).toContainText("2.0 KB")
		await documentPersisted(page)

		await visit(page, url)

		await waitForEditor(page)
		await expect(fileCard(page)).toContainText("notes.zip")
		await expect(fileCard(page)).toContainText("2.0 KB")

		// an archive is not something a browser shows, so the click is a
		// download under the original name carrying the original bytes
		const download = page.waitForEvent("download")
		await fileCard(page).click()
		const downloaded = await download

		expect(downloaded.suggestedFilename()).toBe("notes.zip")
		const path = await downloaded.path()
		expect(await readFile(path)).toEqual(ZIP_BYTES)

		// the server decides the download by its disposition, which
		// carries the recorded name, and forbids sniffing the type away
		const href = await fileCard(page).getAttribute("href")
		const response = await page.request.get(href ?? "")
		expect(response.headers()["content-type"]).toBe("application/zip")
		expect(response.headers()["content-disposition"]).toBe(
			"attachment; filename=notes.zip",
		)
		expect(response.headers()["x-content-type-options"]).toBe("nosniff")
	})

	test("opens a browser-viewable file in a new tab", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		await createDocument(page)

		await insertFileBlock(page, {
			name: "readme.txt",
			mimeType: "text/plain",
			buffer: Buffer.from(TEXT),
		})
		await expect(fileCard(page)).toContainText("readme.txt")

		const popup = page.waitForEvent("popup")
		await fileCard(page).click()
		const opened = await popup

		await expect(opened).toHaveURL(/\/api\/documents\/[a-z0-9]{20}\/files\//)
		await expect(opened.locator("body")).toContainText(TEXT)

		// plain text is on the server's inline allowlist, so the tab
		// renders it rather than downloading it
		const response = await page.request.get(opened.url())
		expect(response.headers()["content-type"]).toBe("text/plain; charset=utf-8")
		expect(response.headers()["content-disposition"]).toBe(
			"inline; filename=readme.txt",
		)
		expect(response.headers()["x-content-type-options"]).toBe("nosniff")
	})
})
