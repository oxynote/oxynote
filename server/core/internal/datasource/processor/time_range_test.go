package processor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_TimeRange_ProcessQuery(t *testing.T) {
	tr := TimeRange{
		From: time.Date(2024, 1, 15, 10, 30, 45, 254000000, time.UTC),
		To:   time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
	}

	tests := map[string]struct {
		TimeRange TimeRange
		Query     string
		Result    string
	}{
		// Bare $__from / $__to (no braces)
		"Bare $__from returns unix milliseconds": {
			TimeRange: tr,
			Query:     "SELECT * FROM metrics WHERE ts >= $__from",
			Result:    "SELECT * FROM metrics WHERE ts >= 1705314645254",
		},
		"Bare $__to returns unix milliseconds": {
			TimeRange: tr,
			Query:     "SELECT * FROM metrics WHERE ts <= $__to",
			Result:    "SELECT * FROM metrics WHERE ts <= 1705316400000",
		},

		// ${__from} / ${__to} without modifier
		"${__from} returns unix milliseconds": {
			TimeRange: tr,
			Query:     "SELECT * FROM metrics WHERE ts >= ${__from}",
			Result:    "SELECT * FROM metrics WHERE ts >= 1705314645254",
		},
		"${__to} returns unix milliseconds": {
			TimeRange: tr,
			Query:     "SELECT * FROM metrics WHERE ts <= ${__to}",
			Result:    "SELECT * FROM metrics WHERE ts <= 1705316400000",
		},

		// ${__from:date} / ${__from:date:iso}
		"${__from:date} returns ISO 8601 with milliseconds": {
			TimeRange: tr,
			Query:     "SELECT * FROM metrics WHERE ts >= '${__from:date}'",
			Result:    "SELECT * FROM metrics WHERE ts >= '2024-01-15T10:30:45.254Z'",
		},
		"${__from:date:iso} returns ISO 8601 with milliseconds": {
			TimeRange: tr,
			Query:     "SELECT * FROM metrics WHERE ts >= '${__from:date:iso}'",
			Result:    "SELECT * FROM metrics WHERE ts >= '2024-01-15T10:30:45.254Z'",
		},
		"${__to:date} returns ISO 8601 with milliseconds": {
			TimeRange: tr,
			Query:     "SELECT * FROM metrics WHERE ts <= '${__to:date}'",
			Result:    "SELECT * FROM metrics WHERE ts <= '2024-01-15T11:00:00.000Z'",
		},

		// ${__from:date:seconds}
		"${__from:date:seconds} returns unix seconds": {
			TimeRange: tr,
			Query:     "SELECT * FROM metrics WHERE ts >= ${__from:date:seconds}",
			Result:    "SELECT * FROM metrics WHERE ts >= 1705314645",
		},
		"${__to:date:seconds} returns unix seconds": {
			TimeRange: tr,
			Query:     "SELECT * FROM metrics WHERE ts <= ${__to:date:seconds}",
			Result:    "SELECT * FROM metrics WHERE ts <= 1705316400",
		},

		// ${__from:date:FORMAT}
		"${__from:date:YYYY-MM} returns custom format": {
			TimeRange: tr,
			Query:     "SELECT * FROM metrics WHERE month = '${__from:date:YYYY-MM}'",
			Result:    "SELECT * FROM metrics WHERE month = '2024-01'",
		},
		"${__from:date:YYYY-MM-DD} returns custom format": {
			TimeRange: tr,
			Query:     "SELECT '${__from:date:YYYY-MM-DD}'",
			Result:    "SELECT '2024-01-15'",
		},
		"${__from:date:YYYY-MM-DD HH} returns custom format with hours": {
			TimeRange: tr,
			Query:     "SELECT '${__from:date:YYYY-MM-DD HH}'",
			Result:    "SELECT '2024-01-15 10'",
		},

		// $__interval
		"Replaces $__interval with calculated step": {
			// 1 day = 86400s / 100 = 864s -> rounds to 15m
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			Query:  "rate(http_requests_total[$__interval])",
			Result: "rate(http_requests_total[15m])",
		},
		"Replaces $__interval with short step": {
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 1, 0, 5, 0, 0, time.UTC),
			},
			Query:  "rate(x[$__interval])",
			Result: "rate(x[15s])",
		},

		// Mixed
		"Multiple modifiers in one query": {
			TimeRange: tr,
			Query:     "SELECT * FROM metrics WHERE ts >= ${__from:date:seconds} AND ts <= ${__to:date:seconds} AND label = '${__from:date:YYYY-MM}'",
			Result:    "SELECT * FROM metrics WHERE ts >= 1705314645 AND ts <= 1705316400 AND label = '2024-01'",
		},
		"No macros leaves query unchanged": {
			TimeRange: tr,
			Query:     "SELECT * FROM metrics",
			Result:    "SELECT * FROM metrics",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result := tc.TimeRange.ProcessQuery(tc.Query)
			assert.Equal(t, tc.Result, result)
		})
	}
}

func Test_toGoTimeFormat(t *testing.T) {
	tests := map[string]struct {
		Input  string
		Result string
	}{
		"YYYY-MM":          {Input: "YYYY-MM", Result: "2006-01"},
		"YYYY-MM-DD":       {Input: "YYYY-MM-DD", Result: "2006-01-02"},
		"YYYY-MM-DD HH":   {Input: "YYYY-MM-DD HH", Result: "2006-01-02 15"},
		"YY":               {Input: "YY", Result: "06"},
		"M-D":              {Input: "M-D", Result: "1-2"},
		"HH mm ss":         {Input: "HH mm ss", Result: "15 04 05"},
		"hh A":             {Input: "hh A", Result: "03 PM"},
		"SSS":              {Input: "SSS", Result: ".000"},
		"YYYY-MM-DD ddd":   {Input: "YYYY-MM-DD ddd", Result: "2006-01-02 Mon"},
		"YYYY-MM-DD dddd":  {Input: "YYYY-MM-DD dddd", Result: "2006-01-02 Monday"},
		"MMMM":             {Input: "MMMM", Result: "January"},
		"MMM":              {Input: "MMM", Result: "Jan"},
		"ZZ":               {Input: "YYYY-MM-DDZZ", Result: "2006-01-02-0700"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result := toGoTimeFormat(tc.Input)
			assert.Equal(t, tc.Result, result)
		})
	}
}

func Test_TimeRange_Normalize(t *testing.T) {
	tests := map[string]struct {
		TimeRange    TimeRange
		ExpectedStep time.Duration
		Result       TimeRange
	}{
		"Short range uses minimum 15s step": {
			// ~5 min range -> calculated interval < 15s -> uses minimum 15s
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 12, 0, 4, 0, time.UTC),
				To:   time.Date(2024, 1, 1, 12, 5, 35, 0, time.UTC),
			},
			ExpectedStep: time.Second * 15,
			Result: TimeRange{
				From: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 1, 12, 5, 45, 0, time.UTC),
			},
		},
		"1 hour range uses 1m step": {
			// 1 hour = 3600s / 100 = 36s -> rounds to 1m
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 12, 0, 4, 0, time.UTC),
				To:   time.Date(2024, 1, 1, 13, 0, 4, 0, time.UTC),
			},
			ExpectedStep: time.Minute,
			Result: TimeRange{
				From: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 1, 13, 1, 0, 0, time.UTC),
			},
		},
		"1 day range uses 15m step": {
			// 1 day = 86400s / 100 = 864s -> rounds to 15m
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 30, 0, time.UTC),
				To:   time.Date(2024, 1, 2, 0, 0, 30, 0, time.UTC),
			},
			ExpectedStep: time.Minute * 15,
			Result: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 2, 0, 15, 0, 0, time.UTC),
			},
		},
		"7 day range uses 2h step": {
			// 7 days = 604800s / 100 = 6048s -> rounds to 2h
			TimeRange: TimeRange{
				From: time.Date(2024, 1, 1, 0, 5, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 8, 0, 5, 0, 0, time.UTC),
			},
			ExpectedStep: time.Hour * 2,
			Result: TimeRange{
				From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2024, 1, 8, 2, 0, 0, 0, time.UTC),
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.ExpectedStep, tc.TimeRange.QueryStep())

			res := tc.TimeRange.Normalize()
			assert.Equal(t, tc.Result, res)
		})
	}
}
