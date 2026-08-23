package datasource

import (
	"net/http"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
)

// _prometheusMatchersQuery is the HTTP query param used to pass matchers for Prometheus label queries.
const _prometheusMatchersQuery = "matchers"

// QueryPrometheusDataSource handles executing a query against a data source.
func (h *Handler) QueryPrometheusDataSource(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
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

	query := r.URL.Query().Get("q")
	if query == "" {
		httpserver.RespondError(h.log, w, ErrQueryRequired)
		return
	}

	tr, err := processor.ParseTimeRange(false, r.URL.Query())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	client, err := h.runners.Runner(*ds).Prometheus(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	result, err := client.QueryRange(r.Context(), query, *tr)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, result, http.StatusOK)
}

// FetchPrometheusDataSourceMetadata handles retrieving metadata from a data source.
func (h *Handler) FetchPrometheusDataSourceMetadata(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
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

	client, err := h.runners.Runner(*ds).Prometheus(r.Context())
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

// FetchPrometheusDataSourceLabelNames handles retrieving label names from a data source.
func (h *Handler) FetchPrometheusDataSourceLabelNames(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
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

	client, err := h.runners.Runner(*ds).Prometheus(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	result, err := client.LabelNames(r.Context(), r.URL.Query()[_prometheusMatchersQuery], *tr)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, result, http.StatusOK)
}

// FetchPrometheusDataSourceLabelValues handles retrieving label values from a data source.
func (h *Handler) FetchPrometheusDataSourceLabelValues(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
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

	client, err := h.runners.Runner(*ds).Prometheus(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	result, err := client.LabelValues(r.Context(), label, r.URL.Query()[_prometheusMatchersQuery], *tr)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, result, http.StatusOK)
}

// FetchPrometheusDataSourceSeries handles retrieving series from a data source.
func (h *Handler) FetchPrometheusDataSourceSeries(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
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

	client, err := h.runners.Runner(*ds).Prometheus(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	result, err := client.Series(r.Context(), r.URL.Query()[_prometheusMatchersQuery], *tr)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, result, http.StatusOK)
}
