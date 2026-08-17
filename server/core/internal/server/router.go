package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/wetsocks/wsserver"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	// _userImageLocationFormat is the URL format for user images.
	_userImageLocationFormat = "/api/users/%s/image"

	// _organizationLogoLocation is the URL path for organization logos.
	_organizationLogoLocation = "/api/organizations/logo"

	// _requestTimeout bounds the processing of a single HTTP request.
	_requestTimeout = 30 * time.Second

	// _corsMaxAgeSeconds is how long browsers may cache CORS preflight
	// responses.
	_corsMaxAgeSeconds = 300
)

// wsRouter prepares all websocket routes/topics.
func (s *Server) wsRouter() *wsserver.Router {
	r := wsserver.NewRouter()
	binderFn := wsMetricsBinder(r, s.fc)

	// version
	binderFn("ping@version",
		s.bindVersionPing,
	)

	// documents
	binderFn("change@document-tree",
		func(tpc wsserver.Topic) {
			s.handlers.document.BindTreeChange(tpc)
		},
	)

	binderFn("change@documents.{documentId}.comments",
		func(tpc wsserver.Topic) {
			s.handlers.comment.BindCommentsChange(tpc)
		},
	)

	binderFn("change@documents.{documentId}.metadata",
		func(tpc wsserver.Topic) {
			s.handlers.document.BindMetadataChange(tpc)
		},
	)

	binderFn("change@documents.{documentId}.reviewers",
		func(tpc wsserver.Topic) {
			s.handlers.document.BindReviewersChange(tpc)
		},
	)

	binderFn("change@documents.{documentId}.maintainers",
		func(tpc wsserver.Topic) {
			s.handlers.document.BindMaintainersChange(tpc)
		},
	)

	// slack messages
	binderFn("post@slack.messages",
		func(tpc wsserver.Topic) {
			s.handlers.slack.BindPostMessage(tpc)
		},
	)

	// notifications
	binderFn("creation@notifications",
		func(tpc wsserver.Topic) {
			s.handlers.notification.BindNotifications(tpc)
		},
	)

	return r
}

// httpRouter prepares all http routes.
func (s *Server) httpRouter() chi.Router {
	r := chi.NewRouter()

	r.Use(httpserver.Timeout(_requestTimeout))
	r.Use(httpserver.Recoverer(s.log))
	r.MethodNotAllowed(httpserver.MethodNotAllowed(s.log))
	r.NotFound(httpserver.NotFound(s.log))
	r.Use(wrapMetrics(s.fc))

	r.Route("/api", func(sr chi.Router) {
		sr.Mount("/", s.router())

		// Note that private routes are not protected by any
		// authentication middlewares (a reverse proxy like Caddy
		// should handle that).
		sr.Mount("/x", s.internalRouter())
	})

	return r
}

// internalRouter prepares all super admin routes.
func (s *Server) internalRouter() chi.Router {
	r := chi.NewRouter()

	r.Get("/version", s.fetchVersion)
	r.Mount("/debug", middleware.Profiler())
	r.Handle("/metrics", promhttp.InstrumentMetricHandler(
		s.fc, promhttp.HandlerFor(s.fc, promhttp.HandlerOpts{}),
	))

	r.Route("/github", func(sr chi.Router) {
		sr.Use(s.handlers.github.VerifySignature)
		sr.Post("/events", s.handlers.github.HandleEvent)
	})

	r.Post("/organizations/{organizationId}/initialize", s.handlers.organization.InitializeOrganization)
	r.Post("/organizations/{organizationId}/teardown", s.handlers.organization.TeardownOrganization)

	r.Route("/slack", func(sr chi.Router) {
		sr.Get("/install", s.handlers.slack.InstallApp)

		sr.Group(func(ssr chi.Router) {
			ssr.Use(s.handlers.slack.VerifySignature)
			ssr.Post("/events", s.handlers.slack.HandleEvent)
			ssr.Post("/commands", s.handlers.slack.HandleCommand)
			ssr.Post("/slash", s.handlers.slack.HandleSlashCommand)
		})
	})

	r.Route("/documents/{documentId}", func(sr chi.Router) {
		sr.Get("/branches", s.handlers.document.FetchDocumentBranchesUnsafe)
		sr.Route("/branch/{branchId}", func(ssr chi.Router) {
			ssr.Get("/", s.handlers.document.FetchDocumentBranchByIDUnsafe)
			ssr.Put("/", s.handlers.document.UpdateDocumentBranchByIDUnsafe)
		})
	})

	r.Post("/email", s.handlers.email.SendEmail)

	return r
}

// router prepares all main application routes.
func (s *Server) router() chi.Router {
	r := chi.NewRouter()

	s.setupCORS(r)
	r.Use(auth.Middleware(s.log, s.opts.Auth, s.client))

	r.Handle("/ws", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.ws.ServeHTTP(w, r)
	}))

	r.Route("/github", func(sr chi.Router) {
		// the status endpoint stays available when the GitHub App is not
		// configured: it is the capability signal for the frontend.
		sr.Get("/", s.handlers.github.CheckInstallation)

		sr.Group(func(ssr chi.Router) {
			ssr.Use(s.handlers.github.RequireConfigured)
			ssr.Delete("/", s.handlers.github.DisconnectOrganization)
			ssr.Get("/install", s.handlers.github.FetchInstall)
			ssr.Get("/connect", s.handlers.github.ConnectOrganization)
			ssr.Get("/issues", s.handlers.github.FetchIssues)
			ssr.Route("/repositories", func(sssr chi.Router) {
				sssr.Get("/", s.handlers.github.FetchRepositories)
				sssr.Route("/{repository}", func(ssssr chi.Router) {
					ssssr.Get("/branches", s.handlers.github.FetchRepositoryBranches)
					ssssr.Get("/tree", s.handlers.github.FetchRepositoryTree)
				})
			})
		})
	})

	r.Route("/users", func(sr chi.Router) {
		sr.Put("/image", s.handlers.user.UploadUserImage)
		sr.Get("/{userId}/image", s.handlers.user.RetrieveUserImage)
	})
	r.Route("/organizations/logo", func(sr chi.Router) {
		sr.Put("/", s.handlers.organization.UploadOrganizationLogo)
		sr.Get("/", s.handlers.organization.RetrieveOrganizationLogo)
	})

	r.Route("/slack", func(sr chi.Router) {
		// the status endpoint stays available when the Slack App is not
		// configured: it is the capability signal for the frontend.
		sr.Get("/", s.handlers.slack.CheckInstallation)

		sr.Group(func(ssr chi.Router) {
			ssr.Use(s.handlers.slack.RequireConfigured)
			ssr.Delete("/", s.handlers.slack.DisconnectOrganization)
			ssr.Get("/install", s.handlers.slack.FetchInstall)
			ssr.Get("/connect", s.handlers.slack.ConnectOrganization)
			ssr.Get("/messages", s.handlers.slack.FetchMessages)
			ssr.Route("/users", func(sssr chi.Router) {
				sssr.Get("/", s.handlers.slack.FetchUserLink)
				sssr.Put("/settings", s.handlers.slack.UpdateUserLink)
				sssr.Delete("/", s.handlers.slack.DeleteUserLink)
				sssr.Get("/link", s.handlers.slack.LinkUser)
			})
		})
	})

	r.Route("/notifications", func(sr chi.Router) {
		sr.Get("/", s.handlers.notification.FetchManyNotifications)
		sr.Get("/count", s.handlers.notification.FetchNotificationsCount)
		sr.Put("/read-status", s.handlers.notification.MarkReadManyNotifications)
	})

	r.Route("/data-sources", func(sr chi.Router) {
		sr.Get("/", s.handlers.datasource.FetchDataSources)
		sr.Post("/", s.handlers.datasource.CreateDataSource)
		sr.Route("/{dataSourceId}", func(ssr chi.Router) {
			ssr.Get("/", s.handlers.datasource.FetchDataSource)
			ssr.Put("/", s.handlers.datasource.UpdateDataSource)
			ssr.Delete("/", s.handlers.datasource.DeleteDataSource)
			ssr.Get("/connection", s.handlers.datasource.TestDataSourceConnection)
			ssr.Get("/query", s.handlers.datasource.QueryDataSource)
			ssr.Route("/prometheus", func(sssr chi.Router) {
				sssr.Get("/query", s.handlers.datasource.QueryPrometheusDataSource)
				sssr.Get("/metadata", s.handlers.datasource.FetchPrometheusDataSourceMetadata)
				sssr.Get("/series", s.handlers.datasource.FetchPrometheusDataSourceSeries)
				sssr.Route("/labels", func(ssssr chi.Router) {
					ssssr.Get("/", s.handlers.datasource.FetchPrometheusDataSourceLabelNames)
					ssssr.Get("/{label}/values", s.handlers.datasource.FetchPrometheusDataSourceLabelValues)
				})
			})
			ssr.Route("/sql", func(sssr chi.Router) {
				sssr.Get("/metadata", s.handlers.datasource.FetchSQLMetadata)
				sssr.Get("/query-labels", s.handlers.datasource.FetchSQLQueryLabels)
			})
		})
	})

	r.Get("/ai/chat", s.handlers.ai.HandleChat)

	r.Route("/documents", func(sr chi.Router) {
		sr.Get("/search", s.handlers.document.SearchDocuments)
		sr.Post("/", s.handlers.document.CreateDocument)
		sr.Route("/{documentId}", func(ssr chi.Router) {
			ssr.Get("/maintainers", s.handlers.document.FetchDocumentMaintainers)
			ssr.Delete("/", s.handlers.document.DeleteDocument)
			ssr.Post("/duplicate", s.handlers.document.DuplicateDocument)
			ssr.Put("/merge", s.handlers.document.MergeBranches)
			ssr.Route("/branches", func(sssr chi.Router) {
				sssr.Get("/", s.handlers.document.FetchDocumentBranches)
				sssr.Post("/", s.handlers.document.CreateDocumentBranch)
				sssr.Route("/{branchId}", func(ssssr chi.Router) {
					ssssr.Put("/", s.handlers.document.UpdateDocumentBranch)
					ssssr.Delete("/", s.handlers.document.DeleteDocumentBranch)
					ssssr.Put("/review-approve", s.handlers.document.UpdateBranchReviewApproval)
					ssssr.Route("/reviewers", func(sssssr chi.Router) {
						sssssr.Get("/", s.handlers.document.FetchBranchReviewers)
						sssssr.Post("/", s.handlers.document.RequestBranchReviewer)
						sssssr.Delete("/", s.handlers.document.RemoveBranchReviewer)
					})
				})
			})
			ssr.Route("/hooks", func(sssr chi.Router) {
				sssr.Get("/", s.handlers.hook.FetchDocumentHooks)
				sssr.Post("/", s.handlers.hook.CreateDocumentHook)
				sssr.Route("/{hookId}", func(ssssr chi.Router) {
					ssssr.Put("/", s.handlers.hook.UpdateDocumentHook)
					ssssr.Delete("/", s.handlers.hook.DeleteDocumentHook)
					ssssr.Put("/reset", s.handlers.hook.ResetDocumentHook)
				})
			})
			ssr.Route("/files", func(sssr chi.Router) {
				sssr.Post("/", s.handlers.files.UploadDocumentFile)
				sssr.Get("/{id}", s.handlers.files.RetrieveDocumentFile)
			})

			ssr.Route("/comments", func(sssr chi.Router) {
				sssr.Get("/", s.handlers.comment.FetchDocumentComments)
				sssr.Post("/", s.handlers.comment.CreateDocumentComment)
				sssr.Route("/{commentId}", func(ssssr chi.Router) {
					// TODO: Uncomment this once comment history is implemented.
					// ssssr.Put("/unresolve", s.handlers.document.UnresolveDocumentComment)
					ssssr.Get("/", s.handlers.comment.FetchDocumentComment)
					ssssr.Put("/", s.handlers.comment.UpdateDocumentComment)
					ssssr.Put("/resolve", s.handlers.comment.ResolveDocumentComment)
					ssssr.Delete("/", s.handlers.comment.DeleteDocumentComment)
					ssssr.Route("/replies", func(sssssr chi.Router) {
						sssssr.Post("/", s.handlers.comment.CreateDocumentCommentReply)
						sssssr.Route("/{replyId}", func(ssssssr chi.Router) {
							ssssssr.Put("/", s.handlers.comment.UpdateDocumentCommentReply)
							ssssssr.Delete("/", s.handlers.comment.DeleteDocumentCommentReply)
						})
					})
				})
			})
		})

		sr.Route("/tree", func(ssr chi.Router) {
			ssr.Put("/", s.handlers.document.UpdateDocumentTree)
			ssr.Get("/", s.handlers.document.FetchDocumentTree)
		})
	})

	return r
}

// setupCORS sets up CORS middleware for the server.
func (s *Server) setupCORS(r *chi.Mux) {
	if len(s.opts.Origins) == 0 {
		s.log.Info("CORS is disabled")
		return
	}

	c := cors.New(cors.Options{
		AllowedOrigins: s.opts.Origins,
		AllowedHeaders: []string{
			"Origin",
			"Accept",
			"Content-Type",
			"Cookie",
			"User-Agent",
		},
		ExposedHeaders: []string{
			"Location",
		},
		AllowedMethods: []string{
			http.MethodHead,
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
		},
		AllowCredentials: true,
		MaxAge:           _corsMaxAgeSeconds,
		Debug:            true,
	})

	c.Log = slog.NewLogLogger(s.log.Handler(), slog.LevelDebug)

	r.Use(c.Handler)
}
