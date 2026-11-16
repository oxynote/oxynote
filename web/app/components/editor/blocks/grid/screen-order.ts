// the width in rem from which the grid shows its 24 columns. A narrower
// grid stacks its blocks. GridView's @4xl classes switch at the same
// width.
const WIDE_GRID_REM = 56

// the elements that Tab can reach
const TAB_STOPS =
	'a[href], button, input, select, textarea, [tabindex], [contenteditable="true"]'

// the direction of the Tab key while it is down, or null after any other
// key, a pointer press or leaving the window. One set of listeners keeps
// it for every grid.
let tabbing: "forward" | "backward" | null = null
let trackingTabs = false

// isWideGrid reports whether a grid this many pixels wide shows its 24
// columns
export function isWideGrid(width: number): boolean {
	const rem =
		parseFloat(getComputedStyle(document.documentElement).fontSize) || 16

	return width >= WIDE_GRID_REM * rem
}

// trackTabbing starts to follow the Tab key for tabDirection. Every grid
// calls it, but it adds the listeners only once.
export function trackTabbing(): void {
	if (trackingTabs) {
		return
	}

	trackingTabs = true
	document.addEventListener(
		"keydown",
		(event) => {
			const plainTab =
				event.key === "Tab" && !event.altKey && !event.ctrlKey && !event.metaKey

			tabbing = plainTab ? (event.shiftKey ? "backward" : "forward") : null
		},
		{ capture: true },
	)

	// the keyup of a Tab that leaves the page never reaches the document
	for (const type of ["keyup", "pointerdown"]) {
		document.addEventListener(type, stopTabbing, { capture: true })
	}

	window.addEventListener("blur", stopTabbing)
}

function stopTabbing(): void {
	tabbing = null
}

// tabDirection returns the direction of a Tab press while the Tab key is
// down, or null at any other time
export function tabDirection(): "forward" | "backward" | null {
	return tabbing
}

// moveTabFocus handles Tab and Shift+Tab inside a grid. It moves focus to
// the next or previous element in the order the grid shows its blocks.
// From the last element, Tab goes on to the first one after the grid. From
// the first element, Shift+Tab goes back to the last one before the grid.
// The browser would follow document order instead and land in another
// block of the grid. blocks returns the blocks in screen order. It runs
// only for a Tab press, because every key typed in a block reaches here.
export function moveTabFocus(
	event: KeyboardEvent,
	grid: HTMLElement,
	blocks: () => HTMLElement[],
): void {
	if (event.key !== "Tab" || event.altKey || event.ctrlKey || event.metaKey) {
		return
	}

	const stops = blocks().flatMap(tabStopsIn)
	const at = stops.findIndex((stop) => stop === document.activeElement)

	if (at < 0) {
		return
	}

	const next = event.shiftKey
		? (stops[at - 1] ?? stopBefore(grid))
		: (stops[at + 1] ?? stopAfter(grid))

	// if no element comes next, focus stays. The browser would follow
	// document order and move it into another block of the grid.
	event.preventDefault()
	next?.focus()
}

// redirectTabEntry handles focus that Tab brings into a grid from outside.
// It moves focus to the first block on screen, or to the last one with
// Shift+Tab. The browser would pick the first or last block in document
// order instead.
export function redirectTabEntry(
	event: FocusEvent,
	grid: HTMLElement,
	blocks: HTMLElement[],
	backward: boolean,
): void {
	if (
		event.relatedTarget instanceof Node &&
		grid.contains(event.relatedTarget)
	) {
		return
	}

	const stops = blocks.flatMap(tabStopsIn)
	const target = backward ? stops.at(-1) : stops[0]

	if (target && target !== event.target) {
		target.focus()
	}
}

// the elements inside root that Tab can reach, in document order
function tabStopsIn(root: ParentNode): HTMLElement[] {
	return Array.from(root.querySelectorAll<HTMLElement>(TAB_STOPS)).filter(
		(element) =>
			element.tabIndex >= 0 &&
			!element.matches(":disabled") &&
			!element.closest("[inert]") &&
			element.getClientRects().length > 0,
	)
}

// the first element that Tab can reach after the grid
function stopAfter(grid: HTMLElement): HTMLElement | undefined {
	return tabStopsIn(document).find(
		(element) =>
			!grid.contains(element) &&
			!!(
				grid.compareDocumentPosition(element) & Node.DOCUMENT_POSITION_FOLLOWING
			),
	)
}

// the last element that Tab can reach before the grid. It can also be an
// element that contains the grid, such as the editor.
function stopBefore(grid: HTMLElement): HTMLElement | undefined {
	return tabStopsIn(document).findLast(
		(element) =>
			!!(
				grid.compareDocumentPosition(element) & Node.DOCUMENT_POSITION_PRECEDING
			),
	)
}
