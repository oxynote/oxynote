// Package demo serves a self-contained Prometheus data source: its metrics
// are synthesized on demand from a fixed seed instead of being scraped from
// a running server, so a fresh install shows populated charts with nothing
// else deployed.
package demo

import (
	"net/http"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

const (
	// Scheme prefixes every URL this package answers for.
	Scheme = "demo://"

	// URL is the only demo data source that exists.
	URL = Scheme + "engineering"
)

// ErrUnknownSource is returned when a demo URL names a source that does
// not exist.
var ErrUnknownSource = errutil.New(
	http.StatusBadRequest,
	"data_source.unknown_demo_source",
	"Unknown demo data source. The only demo data source is %q.",
	URL,
)
