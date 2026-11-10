package tools

import (
	"cmp"
	"reflect"
	"slices"

	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/assistant/markup"
	"github.com/oxynote/oxynote/server/core/internal/document"
)

// gridOps returns the operations that turn the stored grid into built,
// its rewrite. They edit the grid block by block, so a block the rewrite
// leaves as it was keeps the edits users made since the read. Removed
// blocks are deleted, and kept blocks get the attributes that changed.
// The fewest kept blocks move to reach the written order. Each of them,
// and each new block, goes after the block written before it. The first
// written block goes before the first block that stays instead. A
// changed stacked order is set last. A rewrite that keeps no block
// replaces the grid whole.
func gridOps(stored, built document.Block) []edit.Operation {
	gridUID, _ := stored.UID()
	stays := stayingUIDs(stored, built)

	if len(stays) == 0 {
		return []edit.Operation{edit.Replace(gridUID, built)}
	}

	storedByUID := map[string]document.Block{}
	builtUIDs := make([]string, 0, len(built.Content))

	for _, b := range stored.Content {
		uid, _ := b.UID()
		storedByUID[uid] = b
	}

	for _, b := range built.Content {
		uid, _ := b.UID()
		builtUIDs = append(builtUIDs, uid)
	}

	var ops []edit.Operation

	for _, b := range stored.Content {
		if uid, _ := b.UID(); !slices.Contains(builtUIDs, uid) {
			ops = append(ops, edit.Delete(uid))
		}
	}

	for i, b := range built.Content {
		if s, kept := storedByUID[builtUIDs[i]]; kept {
			if changed := changedAttrs(s, b); len(changed) > 0 {
				ops = append(ops, edit.UpdateAttrs(builtUIDs[i], changed))
			}
		}
	}

	anchor := builtUIDs[slices.IndexFunc(builtUIDs, func(uid string) bool { return stays[uid] })]

	for i, b := range built.Content {
		uid := builtUIDs[i]
		if stays[uid] {
			continue
		}

		position, reference := "after", anchor
		if i > 0 {
			reference = builtUIDs[i-1]
		} else {
			position = "before"
		}

		if _, kept := storedByUID[uid]; kept {
			ops = append(ops, edit.Move(uid, position, reference))
		} else {
			ops = append(ops, edit.Insert(position, reference, b))
		}
	}

	if order, changed := stackOrderChange(stored, built); changed {
		ops = append(ops, edit.SetStackOrder(gridUID, order))
	}

	return ops
}

// stayingUIDs returns the kept blocks of a grid rewrite that stay where
// they are. They are the most kept blocks whose written order matches
// their reading order in the stored grid. Every other block has to move.
func stayingUIDs(stored, built document.Block) map[string]bool {
	rank := map[string]int{}

	for i, b := range markup.GridReadingOrder(stored.Content) {
		uid, _ := b.UID()
		rank[uid] = i
	}

	var kept []string

	for _, b := range built.Content {
		if uid, _ := b.UID(); hasKey(rank, uid) {
			kept = append(kept, uid)
		}
	}

	length := make([]int, len(kept))
	prev := make([]int, len(kept))
	best := -1

	for i := range kept {
		length[i], prev[i] = 1, -1

		for j := range i {
			if rank[kept[j]] < rank[kept[i]] && length[j]+1 > length[i] {
				length[i], prev[i] = length[j]+1, j
			}
		}

		if best < 0 || length[i] > length[best] {
			best = i
		}
	}

	stays := map[string]bool{}

	for i := best; i >= 0; i = prev[i] {
		stays[kept[i]] = true
	}

	return stays
}

// changedAttrs returns the attributes of built that differ from stored.
// An attribute that stored sets and built leaves out maps to nil. An
// attribute stored as null is already unset, so it is left out too. The
// attributes that say where the block sits are left out as well. A
// rewrite of its grid sets those through inserts, moves and the stacked
// order, never as plain attributes.
func changedAttrs(stored, built document.Block) map[string]any {
	changed := map[string]any{}

	for key, v := range built.Attrs {
		if !markup.IsGridPlace(key) && !reflect.DeepEqual(v, stored.Attrs[key]) {
			changed[key] = v
		}
	}

	for key := range stored.Attrs {
		if _, set := stored.Attrs.Value(key); set && !markup.IsGridPlace(key) && !hasKey(built.Attrs, key) {
			changed[key] = nil
		}
	}

	return changed
}

// stackOrderChange returns the uids of built's blocks that have a
// stacked place, in place order. It also reports whether the same blocks
// stack in another order in stored. New numbers for the same order are
// no change, so nothing is written for them.
func stackOrderChange(stored, built document.Block) ([]string, bool) {
	type placed struct {
		// uid is the block's uid.
		uid string

		// place is its place in the stacked layout.
		place int
	}

	var order []placed

	for _, b := range built.Content {
		if place, has := markup.GridStackPos(b); has {
			uid, _ := b.UID()
			order = append(order, placed{uid: uid, place: place})
		}
	}

	slices.SortStableFunc(order, func(a, b placed) int { return cmp.Compare(a.place, b.place) })

	uids := make([]string, 0, len(order))

	for _, p := range order {
		uids = append(uids, p.uid)
	}

	before := make([]string, 0, len(uids))

	for _, b := range markup.GridStackOrder(stored.Content) {
		if uid, _ := b.UID(); slices.Contains(uids, uid) {
			before = append(before, uid)
		}
	}

	return uids, !slices.Equal(before, uids)
}

// isGrid reports whether b is a grid, of either name.
func isGrid(b document.Block) bool {
	return b.Type == document.BlockNodeBlockGrid || b.Type == document.BlockNodeMetricGrid
}

// sameUID reports whether a and b carry the same uid.
func sameUID(a, b document.Block) bool {
	ua, _ := a.UID()
	ub, _ := b.UID()

	return ua == ub
}

// hasKey reports whether m holds key.
func hasKey[V any](m map[string]V, key string) bool {
	_, ok := m[key]

	return ok
}
