package processor

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// _rateIntervalFactor multiplies the step interval so rate() windows always
// span enough data points for accurate calculations.
const _rateIntervalFactor = 4

// _prometheusMacroPattern matches a global macro, $__name or ${__...}, as a
// whole, so $__interval_ms is never $__interval followed by _ms.
var _prometheusMacroPattern = regexp.MustCompile(`\$__\w+|\$\{__[^}]*\}`)

// ProcessPrometheusQuery returns q with its global macros replaced by their
// values over tr. A name no macro has stays as written, for Prometheus to
// report.
//
//	$__rate_interval    → 4x the step interval, at least 1m, for rate() (e.g., "1h")
//	$__rate_interval_ms → the same in milliseconds
//
// The rest are the global macros globalMacro and bracedMacro describe.
func (tr TimeRange) ProcessPrometheusQuery(q string) string {
	return _prometheusMacroPattern.ReplaceAllStringFunc(q, func(r string) string {
		var (
			v  string
			ok bool
		)

		if inner, braced := strings.CutPrefix(r, _bracedMacroPrefix); braced {
			inner = strings.TrimSuffix(inner, "}")

			v, ok = tr.prometheusMacro(inner)
			if !ok {
				v, ok = tr.bracedMacro(inner)
			}
		} else {
			v, ok = tr.prometheusMacro(r[len(_macroPrefix):])
		}

		if !ok {
			return r
		}

		return v
	})
}

// prometheusMacro returns the value over tr of the macro named name: a rate
// interval or a global macro.
func (tr TimeRange) prometheusMacro(name string) (string, bool) {
	switch name {
	case "rate_interval":
		return formatInterval(tr.calculateRateInterval(tr.calculateInterval())), true
	case "rate_interval_ms":
		return strconv.FormatInt(tr.calculateRateInterval(tr.calculateInterval()).Milliseconds(), 10), true
	default:
		return tr.globalMacro(name)
	}
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
