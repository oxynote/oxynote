package processor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_TimeRange_ProcessPrometheusQuery(t *testing.T) {
	cc := map[string]struct {
		TimeRange TimeRange
		Query     string
		Result    string
	}{
		"Replaces the step interval": {
			// 1 day = 86400s / 100 = 864s -> rounds to 15m
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			Query:  "rate(http_requests_total[$__interval])",
			Result: "rate(http_requests_total[15m])",
		},
		"Replaces the rate interval": {
			// 1 day interval=15m, rate_interval=15m*4=60m -> rounds to 1h
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			Query:  "rate(http_requests_total[$__rate_interval])",
			Result: "rate(http_requests_total[1h])",
		},
		"Replaces both intervals": {
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			Query:  "rate(a[$__interval]) + rate(b[$__rate_interval])",
			Result: "rate(a[15m]) + rate(b[1h])",
		},
		"Short range uses minimum rate interval": {
			// Short range: interval=15s (minimum), rate_interval=1m (minimum)
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 1, 12, 5, 0, 0, time.UTC),
			},
			Query:  "rate(x[$__rate_interval])",
			Result: "rate(x[1m])",
		},
		"Replaces a short step interval": {
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 1, 0, 5, 0, 0, time.UTC),
			},
			Query:  "rate(http_requests_total[$__interval])",
			Result: "rate(http_requests_total[15s])",
		},
		"Replaces the range": {
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			Query:  "changes(metric[$__range])",
			Result: "changes(metric[86400s])",
		},
		"Replaces the range without truncating to a unit": {
			// 90 minutes must not shrink to "1h".
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 1, 1, 30, 0, 0, time.UTC),
			},
			Query:  "max_over_time(metric[$__range])",
			Result: "max_over_time(metric[5400s])",
		},
		"Interval in milliseconds is matched whole": {
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			Query:  "sum(x) / $__interval_ms",
			Result: "sum(x) / 900000",
		},
		"Rate interval in milliseconds is matched whole": {
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			Query:  "sum(x) / $__rate_interval_ms",
			Result: "sum(x) / 3600000",
		},
		"Range in seconds is matched whole": {
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			Query:  "increase(x[$__range]) / $__range_s",
			Result: "increase(x[86400s]) / 86400",
		},
		"Range in milliseconds is matched whole": {
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			Query:  "sum(x) / $__range_ms",
			Result: "sum(x) / 86400000",
		},
		"Braced intervals": {
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			Query:  "rate(a[${__interval}]) + rate(b[${__rate_interval}])",
			Result: "rate(a[15m]) + rate(b[1h])",
		},
		"Braced date": {
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			Query:  "x > ${__from:date:seconds}",
			Result: "x > 1704067200",
		},
		"Unknown macros stay as written": {
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			Query:  "rate(x[$__nope]) + ${__nope}",
			Result: "rate(x[$__nope]) + ${__nope}",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			result := c.TimeRange.ProcessPrometheusQuery(c.Query)
			assert.Equal(t, c.Result, result)
		})
	}
}

func Test_TimeRange_prometheusMacro(t *testing.T) {
	tr := TimeRange{
		From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	}

	cc := map[string]struct {
		Name   string
		Result string
		OK     bool
	}{
		"Rate interval":                 {Name: "rate_interval", Result: "1h", OK: true},
		"Rate interval in milliseconds": {Name: "rate_interval_ms", Result: "3600000", OK: true},
		"Global macro":                  {Name: "range_s", Result: "86400", OK: true},
		"Unknown name":                  {Name: "nope"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, ok := tr.prometheusMacro(c.Name)
			assert.Equal(t, c.Result, res)
			assert.Equal(t, c.OK, ok)
		})
	}
}

func Test_TimeRange_calculateRateInterval(t *testing.T) {
	cc := map[string]struct {
		Interval time.Duration
		Result   time.Duration
	}{
		"Short interval uses minimum rate interval": {
			Interval: 10 * time.Second,
			Result:   time.Minute,
		},
		"Interval is multiplied and rounded": {
			Interval: 20 * time.Minute,
			Result:   2 * time.Hour,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, TimeRange{}.calculateRateInterval(c.Interval))
		})
	}
}
