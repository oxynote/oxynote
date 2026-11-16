import { describe, test } from "vitest"
import { gridItemOf, layoutGrid } from "./grid"

// 200 standard blocks, two in each row
const placed = Array.from({ length: 200 }, (_, i) =>
	gridItemOf({ widePosX: (i % 2) * 12, widePosY: Math.floor(i / 2) }),
)

// the same grid after every other block lost its place, as a block does
// when it is pasted into the grid
const half = placed.map((item, i) =>
	i % 2 === 0 ? item : { ...item, x: null, y: null },
)

// a block without a place has to search the grid for free cells. A block
// with one only keeps its cells. So comparing the two grids shows what
// that search costs. When the second one gets much slower than the first,
// the search has become slow.
describe("layoutGrid", () => {
	test("a grid of 200 blocks", async ({ bench }) => {
		await bench.compare(
			bench("every block with a place", () => {
				layoutGrid(placed)
			}),
			bench("half the blocks without a place", () => {
				layoutGrid(half)
			}),
		)
	})
})
