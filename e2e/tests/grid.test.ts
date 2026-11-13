import { expect, test, type Page } from "@playwright/test"
import { joinAsSecondUser } from "../helpers/collaboration"
import {
	contentEditor,
	contentPane,
	diffEditor,
	openBlockMenu,
	waitForEditor,
} from "../helpers/editor"
import {
	dragBlock,
	gridBlock,
	pointIn,
	screenOrder,
	sideBySide,
} from "../helpers/grid"
import {
	addLogGrid,
	logReadsSettled,
	trackLogReads,
	type LogPage,
} from "../helpers/logs"
import { t } from "../helpers/i18n"
import { connectMCPClient } from "../helpers/mcp"
import { visit } from "../helpers/page"
import { makeReviewable, showChanges, switchToBranch } from "../helpers/review"
import { signUpWithWorkspace } from "../helpers/workspace"

// the grid switches to its 24 columns once the grid is 56rem wide. That is
// the grid's own width, not the window's. The sidebar and the page's
// margins take about 330px of a window, so a 1024px window still stacks.
const WIDTHS = [
	{ name: "side by side in a 1600px window", input: 1600, expected: true },
	{ name: "side by side in a 1280px window", input: 1280, expected: true },
	{ name: "stacked in a 1024px window", input: 1024, expected: false },
	{ name: "stacked in a 760px window", input: 760, expected: false },
]

// the deploy service's logs in the demo log source
const DEPLOYS = '{service_name="deploy-bot"}'

const WIDE = 1600
const NARROW = 760

test.describe("grid layout", () => {
	for (const c of WIDTHS) {
		test(`shows two standard blocks ${c.name}`, async ({ page, request }) => {
			await signUpWithWorkspace(page, request)
			const grid = await openLogGrid(page, request, ["a", "b"])
			const [a, b] = grid.blockIds

			await page.setViewportSize({ width: c.input, height: 900 })

			await expect
				.poll(() => sideBySide(page, a ?? "", b ?? ""), {
					message: "the grid settles on its layout",
				})
				.toBe(c.expected)
		})
	}

	test("shows a tall block as tall as a standard one once the grid stacks", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		const grid = await openLogGrid(page, request, ["a", "b"])
		const [a = "", b = ""] = grid.blockIds
		const heights = async () => {
			const [boxA, boxB] = await Promise.all(
				[a, b].map((id) => gridBlock(page, id).boundingBox()),
			)

			return { a: boxA?.height ?? 0, b: boxB?.height ?? 0 }
		}

		await page.setViewportSize({ width: WIDE, height: 900 })
		await openBlockMenu(page, contentPane(page), gridBlock(page, b))
		await page
			.getByRole("menuitem", {
				name: t("editor.drag-handle.options.grid-item.height"),
			})
			.click()
		await page
			.getByRole("menuitem", {
				name: t("editor.grid-item.height-options.tall"),
			})
			.click()
		await expect
			.poll(async () => {
				const box = await heights()

				return box.b > box.a * 1.5
			})
			.toBe(true)

		await page.setViewportSize({ width: NARROW, height: 900 })

		await expect
			.poll(async () => {
				const box = await heights()

				return box.b === box.a
			})
			.toBe(true)
	})

	test("keeps the stacked order rearranged in a narrow window when the wide layout changes", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		const { blockIds } = await openLogGrid(page, request, ["a", "b", "c"])
		const [a = "", b = "", c = ""] = blockIds

		await page.setViewportSize({ width: NARROW, height: 900 })
		await expect.poll(() => screenOrder(page, blockIds)).toEqual([a, b, c])
		await dragBlock(
			page,
			gridBlock(page, c),
			await pointIn(gridBlock(page, a), 0.5, 0.25),
		)
		await expect.poll(() => screenOrder(page, blockIds)).toEqual([c, a, b])

		await page.setViewportSize({ width: WIDE, height: 900 })
		await expect.poll(() => sideBySide(page, a, b)).toBe(true)
		await dragBlock(
			page,
			gridBlock(page, b),
			await pointIn(gridBlock(page, a), 0.05, 0.5),
		)
		await expect.poll(() => screenOrder(page, blockIds)).toEqual([b, a, c])

		await page.setViewportSize({ width: NARROW, height: 900 })
		await expect.poll(() => screenOrder(page, blockIds)).toEqual([c, a, b])
	})

	test("loads no log data again while typing or dragging blocks", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		const reads = trackLogReads(page)
		const { blockIds } = await openLogGrid(page, request, ["a", "b"])
		const [a = "", b = ""] = blockIds

		await page.setViewportSize({ width: WIDE, height: 900 })
		await logReadsSettled(reads)
		const loaded = reads.urls.length

		await contentEditor(page).locator("p").first().click()
		await page.keyboard.type("Typing beside the charts")
		await dragBlock(
			page,
			gridBlock(page, b),
			await pointIn(gridBlock(page, a), 0.05, 0.5),
		)
		await expect.poll(() => screenOrder(page, blockIds)).toEqual([b, a])
		await logReadsSettled(reads)

		expect(reads.urls).toHaveLength(loaded)
	})

	test("moves through the blocks with Tab in the order each layout shows them", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		// the blocks show no logs, so Tab stops only at their own buttons
		const { blockIds } = await openLogGrid(
			page,
			request,
			["a", "b", "c"],
			'{service_name="no-such-service"}',
		)
		const [a = "", b = "", c = ""] = blockIds

		await page.setViewportSize({ width: NARROW, height: 900 })
		await dragBlock(
			page,
			gridBlock(page, c),
			await pointIn(gridBlock(page, a), 0.5, 0.25),
		)
		await expect.poll(() => screenOrder(page, blockIds)).toEqual([c, a, b])
		expect(await tabOrder(page, c)).toEqual([c, a, b])

		await page.setViewportSize({ width: WIDE, height: 900 })
		await expect.poll(() => sideBySide(page, a, b)).toBe(true)
		expect(await tabOrder(page, a)).toEqual([a, b, c])
	})

	test("puts a block the assistant adds below another while keeping a person's resize", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		const grid = await openLogGrid(page, request, ["a", "b"])
		const [a = "", b = ""] = grid.blockIds

		await page.setViewportSize({ width: WIDE, height: 900 })
		await openBlockMenu(page, contentPane(page), gridBlock(page, b))
		await page
			.getByRole("menuitem", {
				name: t("editor.drag-handle.options.grid-item.height"),
			})
			.click()
		await page
			.getByRole("menuitem", {
				name: t("editor.grid-item.height-options.tall"),
			})
			.click()

		const client = await connectMCPClient(grid.token)
		const inserted = await client.callTool({
			name: "insert_blocks",
			arguments: {
				document_id: grid.documentId,
				branch_id: grid.branchId,
				position: "below",
				reference_block_uid: a,
				content: `<logs>${JSON.stringify(logBlock("c", { dataSourceId: grid.dataSourceId }))}</logs>`,
			},
		})
		await client.close()
		expect(inserted.isError, JSON.stringify(inserted.content)).toBeFalsy()
		const written = JSON.parse(
			(inserted.content as { text: string }[])[0]?.text ?? "{}",
		) as { content?: string }
		const c = /<logs id="([^"]+)"/.exec(written.content ?? "")?.[1] ?? ""

		await expect
			.poll(async () => {
				const [boxA, boxB, boxC] = await Promise.all(
					[a, b, c].map((id) => gridBlock(page, id).boundingBox()),
				)

				return {
					underA: boxC?.x === boxA?.x && (boxC?.y ?? 0) > (boxA?.y ?? 0),
					tallB: (boxB?.height ?? 0) > (boxA?.height ?? 0) * 1.5,
				}
			})
			.toEqual({ underA: true, tallB: true })
	})

	test("shows where a moved block sits in both layouts in the review", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)
		const { blockIds } = await openLogGrid(page, request, ["a", "b"])
		const [a = "", b = ""] = blockIds

		await page.setViewportSize({ width: WIDE, height: 900 })
		await makeReviewable(page)
		await switchToBranch(page, "draft")
		await dragBlock(
			page,
			gridBlock(page, b),
			await pointIn(gridBlock(page, a), 0.05, 0.5),
		)
		await expect.poll(() => screenOrder(page, blockIds)).toEqual([b, a])
		await showChanges(page)

		await diffEditor(page)
			.locator(`[id="${b}"] [data-slot="popover-trigger"]`)
			.click()
		const popover = page.locator('[data-slot="popover-content"]')

		await expect(popover).toContainText(
			t("editor.diff-change-marker.grid-item.wide-layout"),
		)
		await expect(popover).toContainText(
			t("editor.diff-change-marker.grid-item.wide-place", {
				row: "1",
				column: "13",
			}),
		)
		await expect(popover).toContainText(
			t("editor.diff-change-marker.grid-item.wide-place", {
				row: "1",
				column: "1",
			}),
		)
		await expect(popover).toContainText(
			t("editor.diff-change-marker.grid-item.stacked-layout"),
		)
	})

	test("keeps both people's moves when one drags in the wide layout and the other in the stacked one", async ({
		browser,
		page,
		request,
	}) => {
		test.slow()
		await signUpWithWorkspace(page, request)
		const { blockIds } = await openLogGrid(page, request, ["a", "b", "c"])
		const [a = "", b = "", c = ""] = blockIds
		const other = await joinAsSecondUser(browser, page, request)

		await page.setViewportSize({ width: WIDE, height: 900 })
		await other.page.setViewportSize({ width: NARROW, height: 900 })
		await expect
			.poll(() => screenOrder(other.page, blockIds))
			.toEqual([a, b, c])

		// the stacked move comes first. Once the stacked layout has its own
		// order, the wide move does not change it.
		await dragBlock(
			other.page,
			gridBlock(other.page, c),
			await pointIn(gridBlock(other.page, a), 0.5, 0.25),
		)
		await expect
			.poll(() => screenOrder(other.page, blockIds))
			.toEqual([c, a, b])
		await dragBlock(
			page,
			gridBlock(page, b),
			await pointIn(gridBlock(page, a), 0.05, 0.5),
		)
		await expect.poll(() => screenOrder(page, blockIds)).toEqual([b, a, c])
		await expect
			.poll(() => screenOrder(other.page, blockIds))
			.toEqual([c, a, b])

		await page.setViewportSize({ width: NARROW, height: 900 })
		await other.page.setViewportSize({ width: WIDE, height: 900 })
		await expect.poll(() => screenOrder(page, blockIds)).toEqual([c, a, b])
		await expect
			.poll(() => screenOrder(other.page, blockIds))
			.toEqual([b, a, c])
		await other.context.close()
	})
})

// tabOrder focuses the first element of the block with the given id and
// presses Tab until focus leaves the grid. It returns the ids of the
// blocks that held focus, in order. A Tab that leaves focus where it was
// fails at once, so the test does not hang until its timeout.
async function tabOrder(page: Page, first: string): Promise<string[]> {
	// the block that holds focus, and whether focus moved since the last
	// call
	const focus = () =>
		page.evaluate(() => {
			const seen = window as unknown as { tabOrderLast?: Element | null }
			const active = document.activeElement
			const moved = seen.tabOrderLast !== active

			seen.tabOrderLast = active

			return {
				block: active?.closest("[data-grid-cell]")?.id ?? null,
				moved: moved,
			}
		})
	const visited: string[] = []

	await gridBlock(page, first).locator("button, input").first().focus()

	for (let at = await focus(); at.block !== null; at = await focus()) {
		if (!at.moved) {
			throw new Error(`Tab kept focus in block ${at.block}`)
		}

		if (visited.at(-1) !== at.block) {
			visited.push(at.block)
		}

		await page.keyboard.press("Tab")
	}

	return visited
}

// openLogGrid creates a page with one grid of standard log blocks and
// opens it. Each title gets one block.
async function openLogGrid(
	page: Page,
	request: Parameters<typeof addLogGrid>[1],
	titles: string[],
	query = DEPLOYS,
): Promise<LogPage> {
	const grid = await addLogGrid(
		page,
		request,
		titles.map((title) => logBlock(title, { query: query })),
	)

	await visit(page, grid.url)
	await waitForEditor(page)
	expect(grid.blockIds).toHaveLength(titles.length)

	return grid
}

// logBlock is a standard log block. It shows the logs that match query.
// By default, query picks the deploy service. It does not refresh while a
// test runs.
function logBlock(
	title: string,
	{
		query = DEPLOYS,
		dataSourceId,
	}: { query?: string; dataSourceId?: string } = {},
): Record<string, unknown> {
	return {
		title: title,
		queries: [{ name: "Logs", query: query }],
		refreshInterval: "1d",
		...(dataSourceId ? { dataSourceId: dataSourceId } : {}),
	}
}
