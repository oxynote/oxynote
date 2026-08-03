package connector

import (
	"github.com/go-chi/chi/v5"
)

// router prepares all main application routes.
func (s *Server) router() chi.Router {
	r := chi.NewRouter()

	r.Route("/api/data-sources/", func(sr chi.Router) {
		sr.Post("/connection", s.handlers.datasource.TestDataSourceConnection)
		sr.Route("/prometheus", func(ssr chi.Router) {
			ssr.Post("/query", s.handlers.datasource.QueryPrometheusDataSource)
			ssr.Post("/metadata", s.handlers.datasource.FetchPrometheusDataSourceMetadata)
			ssr.Post("/series", s.handlers.datasource.FetchPrometheusDataSourceSeries)
			ssr.Route("/labels", func(ssssr chi.Router) {
				ssssr.Post("/", s.handlers.datasource.FetchPrometheusDataSourceLabelNames)
				ssssr.Post("/values", s.handlers.datasource.FetchPrometheusDataSourceLabelValues)
			})
		})
		sr.Route("/sql", func(ssr chi.Router) {
			ssr.Post("/metadata", s.handlers.datasource.FetchSQLMetadata)
			ssr.Post("/query-labels", s.handlers.datasource.FetchSQLQueryLabels)
		})
		sr.Route("/postgresql", func(ssr chi.Router) {
			ssr.Post("/query", s.handlers.datasource.QueryPostgreSQLDataSource)
		})
		sr.Route("/mysql", func(ssr chi.Router) {
			ssr.Post("/query", s.handlers.datasource.QueryMySQLDataSource)
		})
	})

	return r
}
