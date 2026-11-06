package datasource

import (
	"net/http"

	datasourceCore "github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
)

// QueryDataSource handles executing a query against a data source and returning
// a unified response format regardless of data source type.
func (h *Handler) QueryDataSource(w http.ResponseWriter, r *http.Request) {
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

	query := r.URL.Query().Get("q")
	if query == "" {
		httpserver.RespondError(h.log, w, ErrQueryRequired)
		return
	}

	ct := processor.ChartType(r.URL.Query().Get("chartType"))
	if err = ct.Validate(); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	tr, err := processor.ParseTimeRange(false, r.URL.Query())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	vars, err := h.values(r, session, query)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	res, err := datasourceCore.Query(r.Context(), h.runners.Runner(*ds), query, vars, *tr, ct)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(h.log, w, res, http.StatusOK)
}
