import { expect, test, type Locator, type Page } from "@playwright/test"
import {
	addLogBlocks,
	atLogsEnd,
	logList,
	logReadsSettled,
	logRowKeys,
	logsSettled,
	readLogKeys,
	scrollLogs,
	trackLogReads,
	type LogReads,
	type LogWindow,
} from "../helpers/logs"
import { visit } from "../helpers/page"
import { signUpWithWorkspace } from "../helpers/workspace"

// the two queries share the deploy service's lines, so the list has to show
// each of those lines once
const QUERIES = [
	{ name: "Reviews and deploys", query: '{service_name=~"pr-bot|deploy-bot"}' },
	{
		name: "Deploys and secrets",
		query: '{service_name=~"deploy-bot|secret-scanner"}',
	},
]

// a row is named by its time, so the block shows times to the nanosecond.
// It does not refresh while a test runs.
const BLOCK = {
	title: "Release activity",
	queries: QUERIES,
	timestampFormat: "numeric_nano",
	refreshInterval: "1d",
}

// the demo source writes a line once the clock reaches the line's time.
// The block reads its two queries at slightly different moments, so their
// newest seconds can differ. The comparisons leave out the newest
// SETTLING_MS. That assumes both reads are answered within it.
const SETTLING_MS = 2_000

const ORDERS = [
	{ name: "newest first", input: "desc" },
	{ name: "oldest first", input: "asc" },
]

test.describe("log blocks", () => {
	// a row shows its time in the reader's zone, and core's times are UTC
	test.use({ timezoneId: "UTC" })

	for (const c of ORDERS) {
		test(`lists every line of two overlapping queries once, ${c.name}, to the end of the window`, async ({
			page,
			request,
		}) => {
			// the hour holds several hundred lines, read a screen at a time
			test.slow()
			await signUpWithWorkspace(page, request)
			const { url, dataSourceId } = await addLogBlocks(page, request, [
				{ ...BLOCK, timeRange: "last_1_hour", order: c.input },
			])
			const reads = trackLogReads(page)
			await visit(page, url)
			const list = logList(page)
			await expect(list).toBeVisible({ timeout: 30_000 })

			const shown = await readDown(list, reads, () => atLogsEnd(list))

			const expected = await expectedKeys(
				page,
				dataSourceId,
				firstWindow(reads),
				c.input,
			)
			const bound = settledBound(shown, c.input)
			expect(shown.filter((key) => key <= bound)).toEqual(
				expected.filter((key) => key <= bound),
			)
			// the demo wrote about 630 of these lines in an hour when this was
			// measured. More than two pages means the list was paged.
			expect(expected.length).toBeGreaterThan(200)
			expect(
				reads.urls.filter((u) => u.searchParams.has("cursor")).length,
			).toBeGreaterThan(2)
		})
	}

	test("shows the same lines after scrolling past the most the list keeps and back to the top", async ({
		page,
		request,
	}) => {
		// twelve hours of lines are about twice the most the list keeps, read
		// a page at a time
		test.slow()
		await signUpWithWorkspace(page, request)
		const { url, dataSourceId } = await addLogBlocks(page, request, [
			{ ...BLOCK, timeRange: "last_12_hours", order: "desc" },
		])
		const reads = trackLogReads(page)
		await visit(page, url)
		const list = logList(page)
		await expect(list).toBeVisible({ timeout: 30_000 })
		const top = await readDown(list, reads, screens(5))

		await jumpTo(list, reads, "end")
		const bottom = await logRowKeys(list)
		await jumpTo(list, reads, "start")
		const topAgain = await readDown(list, reads, screens(5))

		expect(topAgain).toEqual(top)
		// a read back up the list goes in the other order, so the top rows
		// were dropped and read again
		expect(
			reads.urls.filter((u) => u.searchParams.get("order") === "asc").length,
		).toBeGreaterThan(0)
		const expected = await expectedKeys(
			page,
			dataSourceId,
			firstWindow(reads),
			"desc",
		)
		const bound = settledBound(top, "desc")
		const settledTop = top.filter((key) => key <= bound)
		expect(settledTop).toEqual(
			expected.filter((key) => key <= bound).slice(0, settledTop.length),
		)
		expect(bottom).toEqual(expected.slice(-bottom.length))
	})

	test("reads only the log blocks near the screen, and a far block once the reader scrolls to it", async ({
		page,
		request,
	}) => {
		// a block is 20rem high before its margins, so the tenth starts at
		// least 2880 pixels down. The default window is 720 pixels high, so
		// that is more than a screen below the first screen.
		const count = 10
		await signUpWithWorkspace(page, request)
		// each block filters out a text no line holds, so it lists every line,
		// and its reads can be told apart by the query
		const { url } = await addLogBlocks(
			page,
			request,
			Array.from({ length: count }, (_, i) => ({
				title: `Block ${String(i + 1)}`,
				queries: [
					{
						name: "All",
						query: `{env="production"} != "block ${String(i + 1)}"`,
					},
				],
				timeRange: "last_5_minutes",
				refreshInterval: "1d",
			})),
		)
		const reads = trackLogReads(page)
		const readBlocks = () =>
			reads.urls.map(
				(u) => /block (\d+)/.exec(u.searchParams.get("q") ?? "")?.[1],
			)
		await visit(page, url)
		await expect(page.getByText("Block 1", { exact: true })).toBeVisible({
			timeout: 30_000,
		})
		await logReadsSettled(reads)
		const first = readBlocks()

		const last = page.getByText(`Block ${String(count)}`, { exact: true })
		await last.scrollIntoViewIfNeeded()
		await expect
			.poll(() => readBlocks().includes(String(count)), {
				message: "the last block never read its lines",
			})
			.toBe(true)

		expect(first).toContain("1")
		expect(first).not.toContain(String(count))
		expect(first.length).toBeLessThan(count)
		expect(new Set(readBlocks()).size).toBe(readBlocks().length)
	})
})

// readDown reads the rows a screen at a time from where the list is, down
// to where done says to stop, and returns them in order. Each screen
// starts with rows the screen before it ended with, which shows that no
// row was skipped between them.
async function readDown(
	list: Locator,
	reads: LogReads,
	done: (shown: string[]) => Promise<boolean> | boolean,
): Promise<string[]> {
	const shown: string[] = []

	for (;;) {
		await logsSettled(list, reads)
		const rows = await logRowKeys(list)
		const from = shown.length && rows[0] ? shown.lastIndexOf(rows[0]) : 0

		expect(from, "a screen does not start inside the one before it").not.toBe(
			-1,
		)
		expect(rows.slice(0, shown.length - from)).toEqual(shown.slice(from))
		shown.push(...rows.slice(shown.length - from))

		if (await done(shown)) {
			return shown
		}

		await scrollLogs(list)
	}
}

// screens stops readDown after the given number of screens
function screens(count: number): () => boolean {
	let left = count

	return () => --left <= 0
}

// jumpTo moves the list to one of its ends, again and again, until no
// more pages load there
async function jumpTo(
	list: Locator,
	reads: LogReads,
	end: "start" | "end",
): Promise<void> {
	for (;;) {
		const before = reads.urls.length

		await list.evaluate((el, to) => {
			el.scrollTop = to === "end" ? el.scrollHeight : 0
		}, end)
		await logsSettled(list, reads)

		if (reads.urls.length === before) {
			return
		}
	}
}

// firstWindow is the window the block's first read asked for. Every later
// page reads the same one.
function firstWindow(reads: LogReads): LogWindow {
	const first = reads.urls[0]

	return {
		from: first?.searchParams.get("from") ?? "",
		to: first?.searchParams.get("to") ?? "",
	}
}

// expectedKeys reads every line of the block's queries from core, and
// orders them as the list should: once each, by time
async function expectedKeys(
	page: Page,
	dataSourceId: string,
	window: LogWindow,
	order: string,
): Promise<string[]> {
	const lines = (
		await Promise.all(
			QUERIES.map((q) =>
				readLogKeys(page, dataSourceId, q.query, window, order),
			),
		)
	).flat()
	const keys = [...new Map(lines.map((line) => [line.id, line.key])).values()]

	keys.sort()

	return order === "desc" ? keys.reverse() : keys
}

// settledBound is the newest time the comparisons look at: the newest row
// shown, less SETTLING_MS
function settledBound(shown: string[], order: string): string {
	const newest = (order === "desc" ? shown[0] : shown.at(-1)) ?? ""
	const seconds = new Date(
		Date.UTC(
			Number(newest.slice(0, 4)),
			Number(newest.slice(4, 6)) - 1,
			Number(newest.slice(6, 8)),
			Number(newest.slice(8, 10)),
			Number(newest.slice(10, 12)),
			Number(newest.slice(12, 14)),
		) - SETTLING_MS,
	)

	return `${seconds.toISOString().replace(/\D/g, "").slice(0, 14)}${newest.slice(14)}`
}
