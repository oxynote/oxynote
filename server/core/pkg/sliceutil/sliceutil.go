package sliceutil

// Move moves the element at oldIndex to newIndex in the slice.
// Only elements in-between are shifted.
func Move[T any](slice []T, oldIndex, newIndex int) {
	if oldIndex == newIndex || oldIndex < 0 || oldIndex >= len(slice) || newIndex < 0 || newIndex >= len(slice) {
		return
	}

	elem := slice[oldIndex]

	if oldIndex < newIndex {
		copy(slice[oldIndex:], slice[oldIndex+1:newIndex+1])
	} else {
		copy(slice[newIndex+1:], slice[newIndex:oldIndex])
	}

	slice[newIndex] = elem
}
