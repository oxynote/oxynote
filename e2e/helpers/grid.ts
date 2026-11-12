import { expect, type Locator, type Page } from "@playwright/test"
import { contentEditor, contentPane } from "./editor"

// a point on the page, in pixels
export interface Point {
	x: number
	y: number
}

// gridBlock finds a grid block by the id from its markup. Its node view
// carries the id on its root element.
export function gridBlock(page: Page, id: string): Locator {
	return contentEditor(page).locator(`[id="${id}"]`)
}

// pointIn is the point at a fraction of a block's width and height
export async function pointIn(
	block: Locator,
	u: number,
	v: number,
): Promise<Point> {
	const box = await block.boundingBox()
	expect(box).not.toBeNull()

	return {
		x: (box?.x ?? 0) + (box?.width ?? 0) * u,
		y: (box?.y ?? 0) + (box?.height ?? 0) * v,
	}
}

// dragBlock drags a grid block by its handle to the point, as a person
// does with a mouse. The handle shows once the block is hovered.
export async function dragBlock(
	page: Page,
	block: Locator,
	to: Point,
): Promise<void> {
	const handle = contentPane(page).locator(
		'.z-drag-handle [data-slot="dropdown-menu-trigger"]',
		{ has: page.locator('[class*="dots-line"]') },
	)

	await expect(async () => {
		await block.hover({ position: { x: 20, y: 20 } })
		await expect(handle).toBeVisible({ timeout: 1_000 })
	}).toPass()
	await handle.hover()
	await page.mouse.down()
	await page.mouse.move(to.x, to.y, { steps: 10 })
	await page.mouse.up()
}

// screenOrder reads the grid blocks with the given ids as a person does,
// top to bottom, then left to right
export async function screenOrder(
	page: Page,
	ids: string[],
): Promise<string[]> {
	const placed = await Promise.all(
		ids.map(async (id) => ({
			id,
			box: await gridBlock(page, id).boundingBox(),
		})),
	)

	return placed
		.sort(
			(a, b) =>
				(a.box?.y ?? 0) - (b.box?.y ?? 0) || (a.box?.x ?? 0) - (b.box?.x ?? 0),
		)
		.map(({ id }) => id)
}

// sideBySide reports whether two grid blocks share a row
export async function sideBySide(
	page: Page,
	a: string,
	b: string,
): Promise<boolean> {
	const [boxA, boxB] = await Promise.all([
		gridBlock(page, a).boundingBox(),
		gridBlock(page, b).boundingBox(),
	])

	return boxA?.y === boxB?.y && boxA?.x !== boxB?.x
}
