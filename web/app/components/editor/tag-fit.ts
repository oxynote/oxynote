// availableRowWidth is the width the pills have to spend: what is left of
// the group holding the row once everything sharing their line has taken
// its share.
//
// It is measured from the group and the widths of those siblings, never
// from where they happen to start. A sibling's left edge moves as the row
// beside it grows, so a boundary read from it would rise every time a pill
// was added and invite another — the row would never settle.
export function availableRowWidth(
	row: HTMLElement,
	trigger: HTMLElement,
): number {
	const parent = row.parentElement
	if (!parent) {
		return 0
	}

	const rowRect = row.getBoundingClientRect()
	const gap = Number.parseFloat(getComputedStyle(parent).columnGap) || 0
	let taken = 0

	for (let el = row.nextElementSibling; el; el = el.nextElementSibling) {
		const rect = el.getBoundingClientRect()
		// under the sm breakpoint the group stacks, and a sibling on a line
		// of its own takes nothing away from this one
		if (rect.top < rowRect.bottom && rect.bottom > rowRect.top) {
			taken += rect.width + gap
		}
	}

	return (
		parent.getBoundingClientRect().right -
		trigger.getBoundingClientRect().left -
		taken
	)
}

// TagFitState is one reading of the tag row: what it is showing, what that
// needs, and what it has to spend. Widths are in pixels.
export interface TagFitState {
	count: number
	// the most pills the row may ever show
	ceiling: number
	// the width the pills at this count would take
	needed: number
	// the width they have to take it from
	available: number
	// the count that last overflowed, and the width it overflowed at
	tooMany: number
	tooManyAt: number
}

export type TagFitStep = Pick<TagFitState, "count" | "tooMany" | "tooManyAt">

// stepTagFit moves the row one pill towards the most that fit. It is a
// single step rather than an answer because each change is a render, and
// only the render says what the next count would need.
//
// Remembering the count that overflowed is what stops the two directions
// chasing each other: dropping a pill frees the room that would otherwise
// invite it straight back. That memory is tied to the width it was taken
// at, so a row that has since been given more room asks the question
// again rather than living with an answer from when it was narrower.
export function stepTagFit(state: TagFitState): TagFitStep {
	const tooMany =
		Math.abs(state.available - state.tooManyAt) > 1
			? Number.POSITIVE_INFINITY
			: state.tooMany

	// the last pill has nothing to give way to, so it stays and truncates.
	// Even then the overflow is recorded, or the row would take another
	// pill on the next step and drop it again on the one after
	if (state.needed > state.available) {
		return {
			count: Math.max(state.count - 1, 1),
			tooMany: Math.max(state.count, 2),
			tooManyAt: state.available,
		}
	}

	const ceiling = Math.min(state.ceiling, tooMany - 1)

	return {
		count: state.count < ceiling ? state.count + 1 : state.count,
		tooMany: tooMany,
		tooManyAt: state.tooManyAt,
	}
}
