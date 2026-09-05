import { describe, it, vi } from "vitest"
import { availableRowWidth, stepTagFit, type TagFitState } from "./tag-fit"

function state(overrides: Partial<TagFitState> = {}): TagFitState {
	return {
		count: 4,
		ceiling: 4,
		needed: 200,
		available: 500,
		tooMany: Number.POSITIVE_INFINITY,
		tooManyAt: 0,
		...overrides,
	}
}

describe("stepTagFit", () => {
	it("drops a pill when the row cannot hold them", ({ expect }) => {
		expect(stepTagFit(state({ needed: 600, available: 500 }))).toEqual({
			count: 3,
			tooMany: 4,
			tooManyAt: 500,
		})
	})

	it("keeps the last pill even where it does not fit", ({ expect }) => {
		expect(
			stepTagFit(state({ count: 1, needed: 600, available: 500 })).count,
		).toBe(1)
	})

	it("takes another pill while the row has room for one", ({ expect }) => {
		expect(stepTagFit(state({ count: 2 })).count).toBe(3)
	})

	it("stops at the ceiling", ({ expect }) => {
		expect(stepTagFit(state({ count: 4, ceiling: 4 })).count).toBe(4)
	})

	it("stops below the count that overflowed", ({ expect }) => {
		expect(
			stepTagFit(
				state({ count: 2, tooMany: 3, tooManyAt: 500, available: 500 }),
			).count,
		).toBe(2)
	})

	it("asks again once the row has been given more room", ({ expect }) => {
		// the count that overflowed was recorded against a narrower row, so
		// it says nothing about this one
		expect(
			stepTagFit(
				state({ count: 2, tooMany: 3, tooManyAt: 200, available: 900 }),
			),
		).toEqual({ count: 3, tooMany: Number.POSITIVE_INFINITY, tooManyAt: 200 })
	})

	it("holds its answer across a rounding-sized change in width", ({
		expect,
	}) => {
		expect(
			stepTagFit(
				state({ count: 2, tooMany: 3, tooManyAt: 500, available: 500.5 }),
			).count,
		).toBe(2)
	})
})

// happy-dom measures every box as zero-sized at the origin, so each one
// reports the geometry the reading is derived from
function boxed(
	el: HTMLElement,
	left: number,
	width: number,
	top = 0,
): HTMLElement {
	vi.spyOn(el, "getBoundingClientRect").mockReturnValue({
		left: left,
		top: top,
		right: left + width,
		bottom: top + 20,
		width: width,
		height: 20,
		x: left,
		y: top,
		toJSON: () => ({}),
	})

	return el
}

function row(options: {
	siblings: { left: number; width: number; top?: number }[]
}) {
	const parent = boxed(document.createElement("div"), 0, 1000)
	const rowElem = boxed(document.createElement("div"), 0, 200)
	const trigger = boxed(document.createElement("div"), 40, 160)

	parent.append(rowElem)
	rowElem.append(trigger)
	options.siblings.forEach((s) => {
		parent.append(boxed(document.createElement("div"), s.left, s.width, s.top))
	})

	return { rowElem, trigger }
}

describe("availableRowWidth", () => {
	it("spends what is left of the group", ({ expect }) => {
		const { rowElem, trigger } = row({ siblings: [] })

		expect(availableRowWidth(rowElem, trigger)).toBe(960)
	})

	it("gives up the width of a sibling sharing the line", ({ expect }) => {
		const { rowElem, trigger } = row({ siblings: [{ left: 210, width: 120 }] })

		expect(availableRowWidth(rowElem, trigger)).toBe(840)
	})

	it("reads the same width wherever that sibling has been pushed to", ({
		expect,
	}) => {
		// the sibling slides right as the row beside it grows, and a reading
		// taken from its left edge would rise with it
		const near = row({ siblings: [{ left: 210, width: 120 }] })
		const far = row({ siblings: [{ left: 600, width: 120 }] })

		expect(availableRowWidth(far.rowElem, far.trigger)).toBe(
			availableRowWidth(near.rowElem, near.trigger),
		)
	})

	it("keeps the width a sibling on its own line takes", ({ expect }) => {
		const { rowElem, trigger } = row({
			siblings: [{ left: 0, width: 120, top: 40 }],
		})

		expect(availableRowWidth(rowElem, trigger)).toBe(960)
	})

	it("has nothing to spend outside a group", ({ expect }) => {
		const orphan = boxed(document.createElement("div"), 0, 200)

		expect(availableRowWidth(orphan, orphan)).toBe(0)
	})
})
