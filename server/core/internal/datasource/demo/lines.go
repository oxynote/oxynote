package demo

import (
	"fmt"
	"maps"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
)

const (
	// _logSeedBase separates the random streams of the log sources from
	// the metric series', whose seeds count up from one.
	_logSeedBase = 1 << 20

	// _logLabelService names the label a line's service is under.
	_logLabelService = "service_name"

	// _logLabelEnv names the label a line's environment is under.
	_logLabelEnv = "env"

	// _logEnv is the environment every demo line comes from.
	_logEnv = "production"

	// _logFieldLevel names the structured metadata a line's level is
	// under, where Loki 3.1 and later put it.
	_logFieldLevel = "detected_level"

	// _logFieldTraceID names the structured metadata a line's trace id is
	// under.
	_logFieldTraceID = "trace_id"

	// _logFieldUser names the structured metadata a line's author is
	// under.
	_logFieldUser = "user"
)

// _logUsers are the people the demo's lines name.
var _logUsers = []string{
	"ada",
	"barbara",
	"dennis",
	"frances",
	"grace",
	"ken",
	"linus",
	"margaret",
}

// logSource is one stream of demo lines. It follows one bucket of one
// metric family: the more the metric reads at a tick, the more lines the
// source writes in it.
type logSource struct {
	// service is the service the lines are labelled with.
	service string

	// field is the structured metadata key naming the bucket, the same
	// name the metric's label has.
	field string

	// bucket is the bucket the lines belong to, written under field.
	bucket string

	// level is the level of every line the source writes.
	level processor.LogLevel

	// scale is how many units of the metric one line stands for.
	scale float64

	// value reports the metric's reading at a tick.
	value func(tick int64) float64

	// text returns a line's message, drawing what varies from r. It may
	// add fields of its own, such as the commit a deploy names.
	text func(r *rand.Rand, fields map[string]string) string

	// seed names the source's random stream.
	seed int64
}

// streamAt returns the random stream the source's lines in the tick draw
// from. It is a PCG stream rather than the metrics' newRand: those seed by
// adding the tick to a fixed base, which leaves two series' streams
// correlated, and a remainder drawn so would skew the line counts away
// from the metric they follow.
func (ls logSource) streamAt(tick int64) *rand.Rand {
	return rand.New(rand.NewPCG(uint64(ls.seed), uint64(tick))) //nolint:gosec // demo data wants a cheap reproducible source, not a cryptographic one
}

// lineCount reports how many lines the source writes in the tick: the
// metric's reading over the scale, plus one more with a probability equal
// to the remainder, drawn from r.
func (ls logSource) lineCount(r *rand.Rand, tick int64) int {
	v := math.Max(ls.value(tick), 0) / ls.scale
	n := int(v)

	if r.Float64() < v-float64(n) {
		n++
	}

	return n
}

// linesAt returns the source's lines in the tick, oldest first. They are
// a pure function of the source's seed and the tick.
func (ls logSource) linesAt(tick int64) []processor.LogLine {
	r := ls.streamAt(tick)
	n := ls.lineCount(r, tick)

	offsets := make([]int64, n)

	for i := range offsets {
		offsets[i] = r.Int64N(int64(_tick))
	}

	slices.Sort(offsets)

	start := timeAt(tick)
	lines := make([]processor.LogLine, n)

	for i, off := range offsets {
		fields := ls.baseFields(r)

		lines[i] = processor.LogLine{
			Time:   start.Add(time.Duration(off)),
			Level:  ls.level,
			Line:   ls.text(r, fields),
			Labels: ls.labels(),
			Fields: fields,
		}
	}

	return lines
}

// labels returns the stream labels every line of the source carries.
func (ls logSource) labels() map[string]string {
	return map[string]string{
		_logLabelService: ls.service,
		_logLabelEnv:     _logEnv,
	}
}

// baseFields returns the structured metadata every line of the source
// carries before its text adds its own, drawing the varying ones from r.
func (ls logSource) baseFields(r *rand.Rand) map[string]string {
	return map[string]string{
		ls.field:         ls.bucket,
		_logFieldLevel:   string(ls.level),
		_logFieldTraceID: fmt.Sprintf("%016x", r.Uint64()),
		_logFieldUser:    _logUsers[r.IntN(len(_logUsers))],
	}
}

// fieldNames returns the structured metadata keys the source's lines
// carry, sorted.
func (ls logSource) fieldNames() []string {
	r := ls.streamAt(0)
	fields := ls.baseFields(r)
	ls.text(r, fields)

	return slices.Sorted(maps.Keys(fields))
}

// newLogSources declares the demo's log sources over the given registry's
// metric buckets. A source's seed follows its position, so a new source
// goes at the end, or the history of the ones after it changes.
//
//nolint:funlen,maintidx // a flat table of sources, not logic
func newLogSources(reg registry) []logSource {
	deploys := _namespace + "friday_deploys_count"
	attempted := reg.bucketValue(deploys, "attempted")
	successful := reg.bucketValue(deploys, "successful")

	approvals := _namespace + "ship_it_approvals_after_5pm_count"
	merges := _namespace + "prs_merged_friday_afternoon_count"
	secrets := _namespace + "secrets_detected_commits_count"
	meetings := _namespace + "meetings_could_be_doc_count"
	syncs := _namespace + "quick_sync_meetings_same_day_count"
	circles := _namespace + "circle_back_count"
	pizzas := _namespace + "pizza_fridays_next_30_days"

	sha := func(r *rand.Rand, fields map[string]string) string {
		fields["sha"] = fmt.Sprintf("%07x", r.Int64N(1<<28))

		return fields["sha"]
	}

	pr := func(r *rand.Rand, fields map[string]string) string {
		fields["pr"] = strconv.Itoa(4000 + r.IntN(2000))

		return fields["pr"]
	}

	approved := func(r *rand.Rand, fields map[string]string) string {
		return fmt.Sprintf(`PR #%s approved by %s with "ship it" at 17:%02d`, pr(r, fields), fields[_logFieldUser], r.IntN(60))
	}

	merged := func(r *rand.Rand, fields map[string]string) string {
		return fmt.Sprintf("merged PR #%s into main at 15:%02d on a Friday", pr(r, fields), r.IntN(60))
	}

	secret := func(r *rand.Rand, fields map[string]string) string {
		fields["type"] = []string{"aws_access_key", "github_token", "slack_webhook", "private_key"}[r.IntN(4)]

		return fmt.Sprintf("secret detected in commit %s type=%s", sha(r, fields), fields["type"])
	}

	meeting := func(r *rand.Rand, _ map[string]string) string {
		title := []string{"Q3 alignment", "sync on the sync", "roadmap vibes", "quick question"}[r.IntN(4)]

		return fmt.Sprintf("meeting %q could have been a document", title)
	}

	quickSync := func(_ *rand.Rand, fields map[string]string) string {
		return "\"quick sync\" booked for today by " + fields[_logFieldUser]
	}

	circleBack := func(r *rand.Rand, _ map[string]string) string {
		topic := []string{"the migration", "pricing", "the offsite", "tech debt"}[r.IntN(4)]

		return "let's circle back on " + topic
	}

	pizza := func(r *rand.Rand, fields map[string]string) string {
		fields["count"] = strconv.Itoa(1 + r.IntN(12))

		return "pizza order placed count=" + fields["count"]
	}

	sources := []logSource{
		{
			service: "deploy-bot",
			field:   _labelOutcome,
			bucket:  "attempted",
			level:   processor.LogLevelInfo,
			scale:   15_000,
			value:   attempted,
			text: func(r *rand.Rand, fields map[string]string) string {
				return fmt.Sprintf("deploy started sha=%s by %s", sha(r, fields), fields[_logFieldUser])
			},
		},
		{
			service: "deploy-bot",
			field:   _labelOutcome,
			bucket:  "successful",
			level:   processor.LogLevelInfo,
			scale:   15_000,
			value:   successful,
			text: func(r *rand.Rand, fields map[string]string) string {
				return fmt.Sprintf("deploy finished sha=%s in %ds", sha(r, fields), 30+r.IntN(600))
			},
		},
		{
			service: "deploy-bot",
			field:   _labelOutcome,
			bucket:  "rolled_back",
			level:   processor.LogLevelError,
			scale:   15_000,
			value:   func(tick int64) float64 { return attempted(tick) - successful(tick) },
			text: func(r *rand.Rand, fields map[string]string) string {
				return fmt.Sprintf("deploy rolled back sha=%s: health check failed after %ds", sha(r, fields), 5+r.IntN(120))
			},
		},
		{
			service: "pr-bot",
			field:   "reviewer_state",
			bucket:  "sober",
			level:   processor.LogLevelInfo,
			scale:   20_000,
			value:   reg.bucketValue(approvals, "sober"),
			text:    approved,
		},
		{
			service: "pr-bot",
			field:   "reviewer_state",
			bucket:  "one_beer_in",
			level:   processor.LogLevelInfo,
			scale:   20_000,
			value:   reg.bucketValue(approvals, "one_beer_in"),
			text:    approved,
		},
		{
			service: "pr-bot",
			field:   "reviewer_state",
			bucket:  "heading_to_the_door",
			level:   processor.LogLevelWarning,
			scale:   20_000,
			value:   reg.bucketValue(approvals, "heading_to_the_door"),
			text:    approved,
		},
		{
			service: "pr-bot",
			field:   "confidence",
			bucket:  "yolo",
			level:   processor.LogLevelWarning,
			scale:   20_000,
			value:   reg.bucketValue(merges, "yolo"),
			text:    merged,
		},
		{
			service: "pr-bot",
			field:   "confidence",
			bucket:  "fingers_crossed",
			level:   processor.LogLevelInfo,
			scale:   20_000,
			value:   reg.bucketValue(merges, "fingers_crossed"),
			text:    merged,
		},
		{
			service: "pr-bot",
			field:   "confidence",
			bucket:  "vacation_starts_monday",
			level:   processor.LogLevelWarning,
			scale:   20_000,
			value:   reg.bucketValue(merges, "vacation_starts_monday"),
			text:    merged,
		},
		{
			service: "secret-scanner",
			field:   "severity",
			bucket:  "normal",
			level:   processor.LogLevelWarning,
			scale:   1_500,
			value:   reg.bucketValue(secrets, "normal"),
			text:    secret,
		},
		{
			service: "secret-scanner",
			field:   "severity",
			bucket:  "critical",
			level:   processor.LogLevelError,
			scale:   1_500,
			value:   reg.bucketValue(secrets, "critical"),
			text:    secret,
		},
		{
			service: "secret-scanner",
			field:   "severity",
			bucket:  "nuclear",
			level:   processor.LogLevelCritical,
			scale:   1_500,
			value:   reg.bucketValue(secrets, "nuclear"),
			text:    secret,
		},
		{
			service: "calendar",
			field:   "excuse",
			bucket:  "alignment",
			level:   processor.LogLevelInfo,
			scale:   40_000,
			value:   reg.bucketValue(meetings, "alignment"),
			text:    meeting,
		},
		{
			service: "calendar",
			field:   "excuse",
			bucket:  "bandwidth",
			level:   processor.LogLevelInfo,
			scale:   40_000,
			value:   reg.bucketValue(meetings, "bandwidth"),
			text:    meeting,
		},
		{
			service: "calendar",
			field:   "excuse",
			bucket:  "synergy",
			level:   processor.LogLevelInfo,
			scale:   40_000,
			value:   reg.bucketValue(meetings, "synergy"),
			text:    meeting,
		},
		{
			service: "calendar",
			field:   "excuse",
			bucket:  "optics",
			level:   processor.LogLevelInfo,
			scale:   40_000,
			value:   reg.bucketValue(meetings, "optics"),
			text:    meeting,
		},
		{
			service: "calendar",
			field:   "duration",
			bucket:  "10m_to_30m",
			level:   processor.LogLevelInfo,
			scale:   40_000,
			value:   reg.bucketValue(syncs, "10m_to_30m"),
			text:    quickSync,
		},
		{
			service: "calendar",
			field:   "duration",
			bucket:  "30m_to_1h",
			level:   processor.LogLevelInfo,
			scale:   40_000,
			value:   reg.bucketValue(syncs, "30m_to_1h"),
			text:    quickSync,
		},
		{
			service: "calendar",
			field:   "duration",
			bucket:  "1h_plus",
			level:   processor.LogLevelWarning,
			scale:   40_000,
			value:   reg.bucketValue(syncs, "1h_plus"),
			text:    quickSync,
		},
		{
			service: "calendar",
			field:   _labelOutcome,
			bucket:  "circled_back",
			level:   processor.LogLevelInfo,
			scale:   40_000,
			value:   reg.bucketValue(circles, "circled_back"),
			text:    circleBack,
		},
		{
			service: "calendar",
			field:   _labelOutcome,
			bucket:  "forgotten",
			level:   processor.LogLevelDebug,
			scale:   40_000,
			value:   reg.bucketValue(circles, "forgotten"),
			text:    circleBack,
		},
		{
			service: "calendar",
			field:   _labelOutcome,
			bucket:  "rescheduled_to_circle_back",
			level:   processor.LogLevelInfo,
			scale:   40_000,
			value:   reg.bucketValue(circles, "rescheduled_to_circle_back"),
			text:    circleBack,
		},
		{
			service: "pizza-bot",
			field:   "pizza_count",
			bucket:  "1_to_5",
			level:   processor.LogLevelDebug,
			scale:   10_000,
			value:   reg.bucketValue(pizzas, "1_to_5"),
			text:    pizza,
		},
		{
			service: "pizza-bot",
			field:   "pizza_count",
			bucket:  "5_to_10",
			level:   processor.LogLevelDebug,
			scale:   10_000,
			value:   reg.bucketValue(pizzas, "5_to_10"),
			text:    pizza,
		},
		{
			service: "pizza-bot",
			field:   "pizza_count",
			bucket:  "10_plus",
			level:   processor.LogLevelDebug,
			scale:   10_000,
			value:   reg.bucketValue(pizzas, "10_plus"),
			text:    pizza,
		},
	}

	for i := range sources {
		sources[i].seed = _logSeedBase + int64(i)
	}

	return sources
}
