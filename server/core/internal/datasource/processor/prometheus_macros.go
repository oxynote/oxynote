package processor

import (
	"fmt"
	"strings"
	"time"
)

// _rateIntervalFactor multiplies the step interval so rate() windows always
// span enough data points for accurate calculations.
const _rateIntervalFactor = 4

// ProcessPrometheusQuery processes the query string to replace Prometheus-specific
// macros, then applies generic macro expansion via ProcessQuery.
//
// Prometheus-specific macros:
//
//	$__rate_interval → 4x interval or 1m minimum, for rate()/increase() (e.g., "1h")
//	$__range         → total time range duration in whole seconds (e.g., "86400s")
//
// Generic macros (via ProcessQuery):
//
//	$__from     → unix milliseconds of the start time
//	$__to       → unix milliseconds of the end time
//	$__interval → calculated step interval (e.g., "15m")
func (tr TimeRange) ProcessPrometheusQuery(q string) string {
	// Resolve generic macros first ($__interval, $__from, $__to).
	q = tr.ProcessQuery(q)

	interval := tr.calculateInterval()
	rateInterval := tr.calculateRateInterval(interval)
	rangeDuration := tr.To.Sub(tr.From)

	q = strings.ReplaceAll(q, "$__rate_interval", formatInterval(rateInterval))

	// $__range is rendered as exact seconds: formatInterval truncates to one
	// integer unit, and max_over_time(x[$__range]) over "1h" instead of the
	// requested 90 minutes silently covers less than the window.
	q = strings.ReplaceAll(q, "$__range", fmt.Sprintf("%ds", int64(rangeDuration.Seconds())))

	return q
}

// calculateRateInterval calculates the rate interval for use with rate() and similar
// functions. This ensures there are enough data points for accurate rate calculations.
// The rate interval is typically 4x the scrape interval or the calculated interval,
// whichever is larger.
func (tr TimeRange) calculateRateInterval(interval time.Duration) time.Duration {
	minRateInterval := time.Minute
	rateInterval := interval * _rateIntervalFactor

	if rateInterval < minRateInterval {
		return minRateInterval
	}

	return roundInterval(rateInterval)
}
