package markup

import (
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/variable"
)

// _windowReferences maps each attribute that can follow a built-in
// variable to the reference it holds then. Metric and log blocks share
// them.
var _windowReferences = map[string]string{
	document.AttrTimeRange:       "$" + variable.KeyMetricRange,
	document.AttrRefreshInterval: "$" + variable.KeyMetricRefresh,
}

// enumValues returns every value the attribute takes in enums: its
// values and, when it can follow a built-in variable, the reference to
// it.
func enumValues(enums map[string][]string, key string) []string {
	ref, ok := _windowReferences[key]
	if !ok {
		return enums[key]
	}

	return append([]string{ref}, enums[key]...)
}

// setWindow fills the time range and refresh interval the element leaves
// out. A block of type typ that the id names keeps what it stores, so a
// re-sent block keeps its window; a new one follows the built-in
// variables. stored is the block the element's id names, or the zero
// block.
func setWindow(attrs document.Attributes, stored document.Block, typ document.BlockNodeType) {
	isNew := stored.Type != typ

	for key, ref := range _windowReferences {
		if _, set := attrs.Value(key); set {
			continue
		}

		if isNew {
			attrs[key] = ref

			continue
		}

		if v, kept := stored.Attrs.Value(key); kept {
			attrs[key] = v
		}
	}
}
