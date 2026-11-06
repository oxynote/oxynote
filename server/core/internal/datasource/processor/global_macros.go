package processor

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// globalMacro returns the value over tr of the global macro written
// $__name, and whether there is one by that name. A name is matched whole,
// so $__interval_ms is never $__interval followed by _ms.
//
//	$__from, $__to        → unix milliseconds of a range end
//	$__interval           → the step interval, such as 5m
//	$__interval_ms        → the step interval in milliseconds
//	$__range              → the range length in whole seconds, such as 3600s
//	$__range_s, $__range_ms → the range length in seconds or milliseconds
func (tr TimeRange) globalMacro(name string) (string, bool) {
	switch name {
	case _fromKey:
		return strconv.FormatInt(tr.From.UnixMilli(), 10), true
	case "to":
		return strconv.FormatInt(tr.To.UnixMilli(), 10), true
	case "interval":
		return formatInterval(tr.calculateInterval()), true
	case "interval_ms":
		return strconv.FormatInt(tr.calculateInterval().Milliseconds(), 10), true
	case "range":
		return fmt.Sprintf("%ds", int64(tr.To.Sub(tr.From).Seconds())), true
	case "range_s":
		return strconv.FormatInt(int64(tr.To.Sub(tr.From).Seconds()), 10), true
	case "range_ms":
		return strconv.FormatInt(tr.To.Sub(tr.From).Milliseconds(), 10), true
	default:
		return "", false
	}
}

// bracedMacro returns the value over tr of the global macro written
// ${__inner}: any global macro in braces, or a range end with a format.
//
//	${__from:date}, ${__from:date:iso} → ISO 8601 with milliseconds
//	${__from:date:seconds}             → unix seconds
//	${__from:date:YYYY-MM}             → a custom date format
func (tr TimeRange) bracedMacro(inner string) (string, bool) {
	name, format, ok := strings.Cut(inner, ":")
	if !ok {
		return tr.globalMacro(name)
	}

	var t time.Time

	switch name {
	case _fromKey:
		t = tr.From.UTC()
	case "to":
		t = tr.To.UTC()
	default:
		return "", false
	}

	switch format {
	case "date", "date:iso":
		return t.Format(_isoMillisFormat), true
	case "date:seconds":
		return strconv.FormatInt(t.Unix(), 10), true
	}

	layout, ok := strings.CutPrefix(format, "date:")
	if !ok || layout == "" {
		return "", false
	}

	return t.Format(toGoTimeFormat(layout)), true
}
