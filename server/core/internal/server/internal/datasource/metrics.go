package datasource

import (
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
)

// _metricMatchersQuery is the HTTP query param used to pass matchers for metric label queries.
const _metricMatchersQuery = "matchers"

// FetchDataSourceMetricMetadata handles retrieving metadata from a metrics data source.
func (h *Handler) FetchDataSourceMetricMetadata(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "dataSourceId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ds, err := h.db.FetchDataSource(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	client, err := h.runners.Runner(*ds).Metrics(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	result, err := client.Metadata(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, result, http.StatusOK)
}

// FetchDataSourceMetricLabelNames handles retrieving label names from a metrics data source.
func (h *Handler) FetchDataSourceMetricLabelNames(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "dataSourceId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ds, err := h.db.FetchDataSource(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	tr, err := processor.ParseTimeRange(true, r.URL.Query())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	matchers, err := h.substitute(r, session, r.URL.Query()[_metricMatchersQuery]...)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	client, err := h.runners.Runner(*ds).Metrics(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	result, err := client.LabelNames(r.Context(), matchers, *tr)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, result, http.StatusOK)
}

// FetchDataSourceMetricLabelValues handles retrieving label values from a metrics data source.
func (h *Handler) FetchDataSourceMetricLabelValues(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "dataSourceId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	label, err := httpserver.ExtractParam(r, "label")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ds, err := h.db.FetchDataSource(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	tr, err := processor.ParseTimeRange(true, r.URL.Query())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	matchers, err := h.substitute(r, session, r.URL.Query()[_metricMatchersQuery]...)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	client, err := h.runners.Runner(*ds).Metrics(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	result, err := client.LabelValues(r.Context(), label, matchers, *tr)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, result, http.StatusOK)
}

// FetchDataSourceMetricSeries handles retrieving series from a metrics data source.
func (h *Handler) FetchDataSourceMetricSeries(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "dataSourceId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ds, err := h.db.FetchDataSource(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	tr, err := processor.ParseTimeRange(true, r.URL.Query())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	matchers, err := h.substitute(r, session, r.URL.Query()[_metricMatchersQuery]...)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	client, err := h.runners.Runner(*ds).Metrics(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	result, err := client.Series(r.Context(), matchers, *tr)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, result, http.StatusOK)
}
