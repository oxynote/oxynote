package processor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_TimeRange_ProcessPrometheusQuery(t *testing.T) {
	tests := map[string]struct {
		TimeRange TimeRange
		Query     string
		Result    string
	}{
		"Replaces $__interval": {
			// 1 day = 86400s / 100 = 864s -> rounds to 15m
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			Query:  "rate(http_requests_total[$__interval])",
			Result: "rate(http_requests_total[15m])",
		},
		"Replaces $__rate_interval": {
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
		"Replaces short $__interval": {
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 1, 0, 5, 0, 0, time.UTC),
			},
			Query:  "rate(http_requests_total[$__interval])",
			Result: "rate(http_requests_total[15s])",
		},
		"Replaces $__range": {
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			Query:  "changes(metric[$__range])",
			Result: "changes(metric[1d])",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result := tc.TimeRange.ProcessPrometheusQuery(tc.Query)
			assert.Equal(t, tc.Result, result)
		})
	}
}
