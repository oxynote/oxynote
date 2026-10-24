package variable

import (
	"slices"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

const (
	// KeyMetricRange is the key of the built-in time range every metric
	// follows by default.
	KeyMetricRange = "metric_range"

	// KeyMetricRefresh is the key of the built-in refresh interval every
	// metric follows by default.
	KeyMetricRefresh = "metric_refresh"

	// _builtinGroupName is the name of the group every organization
	// starts with.
	_builtinGroupName = "Metrics"

	// _defaultTimeRange is the default of the built-in time range.
	_defaultTimeRange = "last_24_hours"

	// _defaultRefreshInterval is the default of the built-in refresh
	// interval.
	_defaultRefreshInterval = "5m"
)

// SeedBuiltins returns the built-in group a fresh organization starts
// with: Metrics, holding a time range and a refresh interval.
func SeedBuiltins(organizationID string) GroupTree {
	now := timeutil.Now()

	g := Group{
		ID:             xid.New(),
		OrganizationID: organizationID,
		GroupName:      _builtinGroupName,
		Color:          "blue",
		Builtin:        true,
		CreatedAt:      now,
	}

	timeRange := Variable{
		ID:             xid.New(),
		OrganizationID: organizationID,
		GroupID:        g.ID,
		Key:            KeyMetricRange,
		Name:           "Metric time range",
		Type:           TypeTimeRange,
		DefaultValue:   null.StringFrom(_defaultTimeRange),
		Builtin:        true,
		CreatedAt:      now,
		Options:        []Option{},
	}

	refresh := Variable{
		ID:             xid.New(),
		OrganizationID: organizationID,
		GroupID:        g.ID,
		Key:            KeyMetricRefresh,
		Name:           "Metric refresh interval",
		Type:           TypeList,
		SortIndex:      1,
		Builtin:        true,
		CreatedAt:      now,
	}

	// the first option is the default, so the default interval leads and
	// the rest follow shortest first.
	values := slices.DeleteFunc(document.MetricRefreshIntervals(), func(v string) bool { return v == _defaultRefreshInterval })
	values = slices.Insert(values, 0, _defaultRefreshInterval)

	refresh.Options = make([]Option, 0, len(values))

	for i, v := range values {
		refresh.Options = append(refresh.Options, Option{
			ID:         xid.New(),
			VariableID: refresh.ID,
			Value:      v,
			SortIndex:  i,
		})
	}

	refresh.DefaultValue = null.StringFrom(refresh.Options[0].ID.String())

	return GroupTree{
		Group: g,
		Variables: []Entry{
			{Variable: timeRange},
			{Variable: refresh},
		},
	}
}
