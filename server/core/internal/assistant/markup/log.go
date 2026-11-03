package markup

import (
	jsonv2 "encoding/json/v2"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/document"
)

// _logOnlyEnums maps each enum-valued attribute only a log block has to
// its values. They mirror the editor's enums in
// web/app/components/editor/blocks/logs/utils.ts (LogTimestampFormat,
// LogOrder). A value added there has to be added here, or the assistant
// cannot write it.
var _logOnlyEnums = map[string][]string{
	document.AttrTimestampFormat: {"none", "standard", "detailed", "detailed_milli", "detailed_nano"},
	document.AttrOrder:           {"desc", "asc"},
}

// _logEnums maps each enum-valued log attribute to its values, without
// the built-in variable references. enumValues adds those for the
// decoder. The window and width take the same values as a metric's.
var _logEnums = map[string][]string{
	document.AttrTimestampFormat: _logOnlyEnums[document.AttrTimestampFormat],
	document.AttrOrder:           _logOnlyEnums[document.AttrOrder],
	document.AttrTimeRange:       document.MetricTimeRanges(),
	document.AttrRefreshInterval: document.MetricRefreshIntervals(),
	document.AttrWidth:           _gridItemWidths,
}

// LogReference describes the attributes a <logs> holds, its enum values
// taken from the validator so the two cannot drift apart.
func LogReference() string {
	var sb strings.Builder

	sb.WriteString("dataSourceId, queries ([{name, query}]), timeRange, refreshInterval and width as a metric block's")

	for _, key := range slices.Sorted(maps.Keys(_logOnlyEnums)) {
		fmt.Fprintf(&sb, ", %s (%s)", key, strings.Join(_logOnlyEnums[key], "|"))
	}

	sb.WriteString(", and optionally title. order sorts the lines by time, desc showing the newest first")

	return sb.String()
}

// logAttrs holds the log attributes that are not enums. Decoding a log
// block's JSON into it checks the type of each one.
type logAttrs struct {
	// Title is the log block's heading.
	Title string `json:"title"`

	// DataSourceID names the data source the queries run against.
	DataSourceID string `json:"dataSourceId"`

	// Queries are the queries whose rows the block lists.
	Queries []logQuery `json:"queries"`
}

// logQuery is one row of a log block's queries.
type logQuery struct {
	// Name is the query's display name. Required.
	Name *string `json:"name"`

	// Query is the query text. Required.
	Query *string `json:"query"`
}

// decodeLogAttrs reads a log block's JSON attributes and checks them:
// every enum holds one of its values, every other attribute has its
// type, and every query has a name and a query.
func decodeLogAttrs(text string) (document.Attributes, error) {
	attrs := document.Attributes{}

	if strings.TrimSpace(text) == "" {
		return attrs, nil
	}

	if err := jsonv2.Unmarshal([]byte(text), &attrs); err != nil {
		return nil, fmt.Errorf("<logs> holds a JSON object of its attributes: %w", err)
	}

	for _, key := range slices.Sorted(maps.Keys(_logEnums)) {
		v, ok := attrs.Value(key)
		if !ok {
			continue
		}

		if s, isString := v.(string); !isString || !slices.Contains(enumValues(_logEnums, key), s) {
			return nil, fmt.Errorf("log block %s must be one of: %s", key, strings.Join(enumValues(_logEnums, key), ", "))
		}
	}

	var typed logAttrs

	if err := jsonv2.Unmarshal([]byte(text), &typed); err != nil {
		return nil, fmt.Errorf("log block attributes: %w", err)
	}

	for i, q := range typed.Queries {
		if q.Name == nil || q.Query == nil {
			return nil, fmt.Errorf("log block query %d: name and query are required", i+1)
		}
	}

	return attrs, nil
}
