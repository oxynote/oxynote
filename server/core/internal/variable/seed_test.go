package variable

import (
	"testing"

	"github.com/guregu/null/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_SeedBuiltins(t *testing.T) {
	t.Parallel()

	g := SeedBuiltins("org")

	assert.NotZero(t, g.ID)
	assert.Equal(t, "org", g.OrganizationID)
	assert.Equal(t, "Metrics", g.GroupName)
	assert.Equal(t, "blue", g.Color)
	assert.True(t, g.Builtin)
	require.Len(t, g.Variables, 2)

	timeRange := g.Variables[0]
	assert.Equal(t, "metric_range", timeRange.Key)
	assert.Equal(t, "Metric time range", timeRange.Name)
	assert.Equal(t, TypeTimeRange, timeRange.Type)
	assert.Equal(t, null.StringFrom("last_24_hours"), timeRange.DefaultValue)
	assert.Equal(t, 0, timeRange.SortIndex)
	assert.Empty(t, timeRange.Options)

	refresh := g.Variables[1]
	assert.Equal(t, "metric_refresh", refresh.Key)
	assert.Equal(t, "Metric refresh interval", refresh.Name)
	assert.Equal(t, TypeList, refresh.Type)
	assert.Equal(t, 1, refresh.SortIndex)

	values := make([]string, 0, len(refresh.Options))

	for i, o := range refresh.Options {
		assert.Equal(t, refresh.ID, o.VariableID)
		assert.Equal(t, i, o.SortIndex)

		values = append(values, o.Value)
	}

	assert.Equal(t, []string{"5m", "5s", "10s", "30s", "1m", "15m", "30m", "1h", "2h", "1d"}, values)
	assert.Equal(t, null.StringFrom(refresh.Options[0].ID.String()), refresh.DefaultValue)

	for _, v := range g.Variables {
		assert.NotZero(t, v.ID)
		assert.Equal(t, "org", v.OrganizationID)
		assert.Equal(t, g.ID, v.GroupID)
		assert.True(t, v.Builtin)
		assert.NoError(t, v.ValidateValue(v.DefaultValue.String))
	}
}
