import {
	expect,
	type APIRequestContext,
	type Locator,
	type Page,
	type Request,
} from "@playwright/test"
import { coreURL, documentId } from "./api"
import { createDocument, documentPersisted } from "./editor"
import { authorizeMCPClient, connectMCPClient } from "./mcp"

export interface LogWindow {
	from: string
	to: string
}

export interface LogReads {
	// every log read the page sent, in the order it sent them
	urls: URL[]
	pending: () => number
}

interface LogLine {
	id: string
	time: string
}

// addLogBlocks creates a page holding one log block per config, each in a
// grid of its own, so the blocks stack down the page. Every block reads
// the demo log source. The blocks go in through the MCP surface, because
// a block's queries are configuration and not text a test can type.
export async function addLogBlocks(
	page: Page,
	request: APIRequestContext,
	configs: Record<string, unknown>[],
): Promise<{ url: string; dataSourceId: string }> {
	await createDocument(page)
	await documentPersisted(page)
	const id = documentId(page)
	const url = page.url()
	const client = await connectMCPClient(
		await authorizeMCPClient(
			page,
			request,
			"documents:read documents:write data-sources:read",
		),
	)

	const sources = await client.callTool({
		name: "list_data_sources",
		arguments: {},
	})
	const dataSourceId = (
		JSON.parse((sources.content as { text: string }[])[0]?.text ?? "{}") as {
			data_sources?: { id: string; name: string }[]
		}
	).data_sources?.find((s) => s.name === "Demo Logs")?.id
	expect(dataSourceId).toBeDefined()

	const listing = await client.callTool({
		name: "list_documents",
		arguments: {},
	})
	const branchId = (
		JSON.parse((listing.content as { text: string }[])[0]?.text ?? "{}") as {
			documents: { id: string; branches: { id: string; default: boolean }[] }[]
		}
	).documents
		.find((d) => d.id === id)
		?.branches.find((b) => b.default)?.id
	expect(branchId).toBeDefined()

	const inserted = await client.callTool({
		name: "insert_blocks",
		arguments: {
			document_id: id,
			branch_id: branchId,
			position: "end",
			content: configs
				.map(
					(config) =>
						`<grid><logs>${JSON.stringify({ dataSourceId: dataSourceId, ...config })}</logs></grid>`,
				)
				.join(""),
		},
	})
	expect(inserted.isError, JSON.stringify(inserted.content)).toBeFalsy()
	await client.close()

	return { url: url, dataSourceId: dataSourceId ?? "" }
}

// logList is the scroller of the page's one log block. The page holds
// other scrollers, such as the icon picker's grid, but only a log list
// holds rows that open.
export function logList(page: Page): Locator {
	return page
		.locator(".vue-recycle-scroller")
		.filter({ has: page.locator("button[aria-expanded]") })
}

// trackLogReads records the log reads the page sends from now on, and how
// many of them are still waiting for an answer. Only the reads it saw
// start count. A read sent before it began, which a navigation then aborts,
// would otherwise bring the count below zero.
export function trackLogReads(page: Page): LogReads {
	const urls: URL[] = []
	const pending = new Set<Request>()
	const isLogRead = (url: string) =>
		/\/api\/data-sources\/[^/]+\/logs\?/.test(url)

	page.on("request", (r) => {
		if (isLogRead(r.url())) {
			urls.push(new URL(r.url()))
			pending.add(r)
		}
	})
	page.on("requestfinished", (r) => {
		pending.delete(r)
	})
	page.on("requestfailed", (r) => {
		pending.delete(r)
	})

	return { urls: urls, pending: () => pending.size }
}

// logRowKeys reads the rows the list shows, from the top of the list
// down. A row is named by its time, in digits down to the nanosecond, so
// the block has to show times in the numeric_nano format and in UTC. The
// scroller reuses its row elements in any order, so the rows are sorted
// by where they sit.
export async function logRowKeys(list: Locator): Promise<string[]> {
	return await list.evaluate((el) => {
		const box = el.getBoundingClientRect()

		return [...el.querySelectorAll("button[aria-expanded]")]
			.map((row) => ({
				rect: row.getBoundingClientRect(),
				key: (row.querySelector(".tabular-nums")?.textContent ?? "").replace(
					/\D/g,
					"",
				),
			}))
			.filter(({ rect }) => rect.bottom > box.top && rect.top < box.bottom)
			.sort((a, b) => a.rect.top - b.rect.top)
			.map(({ key }) => key)
	})
}

// logsSettled waits until the list has every page it asked for and its
// rows stopped changing. Nothing reports when the scroller has drawn the
// rows of a page, so the rows are read until two reads in a row agree.
export async function logsSettled(
	list: Locator,
	reads: LogReads,
): Promise<void> {
	let last = ""

	await expect
		.poll(
			async () => {
				const rows = JSON.stringify(await logRowKeys(list))
				const settled = rows === last && reads.pending() === 0

				last = rows

				return settled
			},
			{ message: "the log rows keep changing" },
		)
		.toBe(true)
}

// logReadsSettled waits until the page sends no more log reads and every
// read it sent has its answer
export async function logReadsSettled(reads: LogReads): Promise<void> {
	let last = -1

	await expect
		.poll(
			() => {
				const settled = reads.urls.length === last && reads.pending() === 0

				last = reads.urls.length

				return settled
			},
			{ message: "the log reads keep coming" },
		)
		.toBe(true)
}

// scrollLogs moves the list by its height less 40 pixels. A closed row is
// 33 pixels high, so the rows before and after the move overlap by about
// a row.
export async function scrollLogs(list: Locator): Promise<void> {
	await list.evaluate((el) => {
		el.scrollTop += el.clientHeight - 40
	})
}

// atLogsEnd reports whether the list is scrolled to its bottom
export async function atLogsEnd(list: Locator): Promise<boolean> {
	return await list.evaluate(
		(el) => el.scrollTop + el.clientHeight >= el.scrollHeight - 1,
	)
}

// readLogKeys reads every line a query matches in the window straight from
// core. Each request asks for 1000 lines, the largest page core accepts,
// and the next request starts at the cursor the last one returned. It
// names each line as logRowKeys does.
export async function readLogKeys(
	page: Page,
	dataSourceId: string,
	query: string,
	window: LogWindow,
	order: string,
): Promise<{ id: string; key: string }[]> {
	const lines: LogLine[] = []
	let cursor: string | null = null

	do {
		const params = new URLSearchParams({
			q: query,
			from: window.from,
			to: window.to,
			limit: "1000",
			order: order,
		})

		if (cursor) {
			params.set("cursor", cursor)
		}

		const response = await page.request.get(
			coreURL(`/api/data-sources/${dataSourceId}/logs?${params.toString()}`),
		)
		expect(response.ok()).toBe(true)

		const body = (await response.json()) as {
			lines: LogLine[]
			nextCursor: string | null
		}

		lines.push(...body.lines)
		cursor = body.nextCursor
	} while (cursor)

	return lines.map((line) => ({ id: line.id, key: timeKey(line.time) }))
}

// timeKey writes an RFC 3339 time in UTC as the digits a row shows. Core
// drops the trailing zeros of the fraction, and a row shows all nine.
function timeKey(time: string): string {
	const [whole = "", fraction = ""] = time.replace(/Z$/, "").split(".")

	return `${whole}${fraction.padEnd(9, "0")}`.replace(/\D/g, "")
}
