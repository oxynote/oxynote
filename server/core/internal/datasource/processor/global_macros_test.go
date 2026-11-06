package processor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_TimeRange_globalMacro(t *testing.T) {
	tr := TimeRange{
		From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	}

	cc := map[string]struct {
		Name   string
		Result string
		OK     bool
	}{
		"Range start":                 {Name: "from", Result: "1704067200000", OK: true},
		"Range end":                   {Name: "to", Result: "1704153600000", OK: true},
		"Interval":                    {Name: "interval", Result: "15m", OK: true},
		"Interval in milliseconds":    {Name: "interval_ms", Result: "900000", OK: true},
		"Range":                       {Name: "range", Result: "86400s", OK: true},
		"Range in seconds":            {Name: "range_s", Result: "86400", OK: true},
		"Range in milliseconds":       {Name: "range_ms", Result: "86400000", OK: true},
		"Unknown name":                {Name: "nope"},
		"Name with a suffix is whole": {Name: "interval_x"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, ok := tr.globalMacro(c.Name)
			assert.Equal(t, c.OK, ok)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_TimeRange_bracedMacro(t *testing.T) {
	tr := TimeRange{
		From: time.Date(2024, 1, 15, 10, 30, 45, 254000000, time.UTC),
		To:   time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
	}

	cc := map[string]struct {
		Inner  string
		Result string
		OK     bool
	}{
		"Global macro":                 {Inner: "from", Result: "1705314645254", OK: true},
		"Global macro in milliseconds": {Inner: "interval_ms", Result: "30000", OK: true},
		"Range start date":             {Inner: "from:date", Result: "2024-01-15T10:30:45.254Z", OK: true},
		"Range start ISO date":         {Inner: "from:date:iso", Result: "2024-01-15T10:30:45.254Z", OK: true},
		"Range end date":               {Inner: "to:date", Result: "2024-01-15T11:00:00.000Z", OK: true},
		"Range start in seconds":       {Inner: "from:date:seconds", Result: "1705314645", OK: true},
		"Range end in seconds":         {Inner: "to:date:seconds", Result: "1705316400", OK: true},
		"Year and month format":        {Inner: "from:date:YYYY-MM", Result: "2024-01", OK: true},
		"Format including the hour":    {Inner: "from:date:YYYY-MM-DD HH", Result: "2024-01-15 10", OK: true},
		"Unknown global macro":         {Inner: "nope"},
		"Format on something else":     {Inner: "interval:date"},
		"Format other than a date":     {Inner: "from:raw"},
		"Empty date format":            {Inner: "from:date:"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, ok := tr.bracedMacro(c.Inner)
			assert.Equal(t, c.OK, ok)
			assert.Equal(t, c.Result, res)
		})
	}
}
