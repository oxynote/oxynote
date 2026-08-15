// Package main is the entry point of the oxynote-core API server.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/meilisearch/meilisearch-go"
	"github.com/oxynote/oxynote/server/core/internal/apps/githubapp"
	"github.com/oxynote/oxynote/server/core/internal/apps/slackapp"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchanges"
	"github.com/oxynote/oxynote/server/core/internal/assistant"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/buildinfo"
	"github.com/oxynote/oxynote/server/core/internal/db"
	hookMan "github.com/oxynote/oxynote/server/core/internal/document/hook/manager"
	"github.com/oxynote/oxynote/server/core/internal/document/searchgw"
	searchMan "github.com/oxynote/oxynote/server/core/internal/document/searchgw/manager"
	"github.com/oxynote/oxynote/server/core/internal/email"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/notification/interpreter"
	"github.com/oxynote/oxynote/server/core/internal/server"
	"github.com/oxynote/oxynote/server/core/internal/server/auth"
	"github.com/oxynote/oxynote/server/core/internal/storage"
	"github.com/oxynote/oxynote/server/core/pkg/ioutil"
	"github.com/oxynote/oxynote/server/core/pkg/logutil"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/redisutil"
	"github.com/oxynote/oxynote/server/core/pkg/sentryutil"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// _serverPort specifies the port the HTTP server listens on.
	_serverPort = 8080

	// _maxNotifications specifies the maximum number of notifications
	// retained per user.
	_maxNotifications = 500

	// _sentryDedupInterval specifies how long a reported error suppresses
	// duplicate reports.
	_sentryDedupInterval = 10 * time.Minute

	// _sentryDedupCapacity specifies the maximum number of error
	// fingerprints tracked for deduplication.
	_sentryDedupCapacity = 100
)

func main() { //nolint:maintidx // main performs linear wiring of all components
	var closers []io.Closer

	log, flushLogs, ok := newLogger()
	if !ok {
		return
	}

	closers = append(closers, ioutil.CloserFunc(func() error {
		flushLogs()
		return nil
	}))

	metrics, cleanup := newMetrics(log)
	defer cleanup()

	dbc, err := db.New(log, metrics, db.Options{
		DSN:                                buildinfo.Getenv("DB_DSN"),
		MaxNotifications:                   _maxNotifications,
		DataSourceCredentialsSigningSecret: buildinfo.Getenv("DB_DATA_SOURCE_CREDENTIALS_SIGNING_SECRET"),
	})
	if err != nil {
		err = ioutil.AppendCloseErr(
			ioutil.MultiCloser(true, closers...),
			err,
		)

		log.With("error", err).
			Error("cannot create a database connection")

		return
	}

	closers = append([]io.Closer{dbc}, closers...)

	rdb, err := redisutil.NewPool(
		buildinfo.Getenv("VALKEY_NETWORK"),
		buildinfo.Getenv("VALKEY_ADDRESS"),
	)
	if err != nil {
		err = ioutil.AppendCloseErr(
			ioutil.MultiCloser(true, closers...),
			fmt.Errorf("cannot create a redis pool: %w", err),
		)

		log.With("error", err).
			Error("cannot create a redis pool")

		return
	}

	closers = append([]io.Closer{rdb}, closers...)

	// an empty GITHUB_APP_ID means the GitHub App integration is disabled.
	// The zero app ID makes githubapp.NewManager return an unconfigured
	// manager instead of failing.
	var githubAppID int64

	if v := buildinfo.Getenv("GITHUB_APP_ID"); v != "" {
		githubAppID, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			err = ioutil.AppendCloseErr(
				ioutil.MultiCloser(true, closers...),
				fmt.Errorf("invalid GITHUB_APP_ID: %w", err),
			)

			log.With("error", err).
				Error("cannot parse GITHUB_APP_ID")

			return
		}
	}

	githubMan, err := githubapp.NewManager(dbc, githubapp.Options{
		AppID:                     githubAppID,
		AppSlug:                   buildinfo.Getenv("GITHUB_APP_SLUG"),
		SignatureSecret:           buildinfo.Getenv("GITHUB_SIGNATURE_SECRET"),
		PrivateKeyPath:            buildinfo.Getenv("GITHUB_PRIVATE_KEY_PATH"),
		InstallationSigningSecret: buildinfo.Getenv("GITHUB_INSTALLATION_SIGNING_SECRET"),
	})
	if err != nil {
		err = ioutil.AppendCloseErr(
			ioutil.MultiCloser(true, closers...),
			fmt.Errorf("cannot create github app manager: %w", err),
		)

		log.With("error", err).
			Error("cannot create github app manager")

		return
	}

	notifMan := notification.NewManager(log, dbc)
	closers = append([]io.Closer{notifMan}, closers...)

	slackMan, err := slackapp.NewManager(
		log,
		dbc,
		interpreter.NewInterpreter(dbc, buildinfo.Getenv("BASE_APP_URL")),
		notifMan,
		slackapp.Options{
			RedirectURL:               buildinfo.Getenv("SLACK_REDIRECT_URL"),
			ClientID:                  buildinfo.Getenv("SLACK_CLIENT_ID"),
			ClientSecret:              buildinfo.Getenv("SLACK_CLIENT_SECRET"),
			SignatureSecret:           buildinfo.Getenv("SLACK_SIGNATURE_SECRET"),
			InstallationSigningSecret: buildinfo.Getenv("SLACK_INSTALLATION_SIGNING_SECRET"),
		},
	)
	if err != nil {
		err = ioutil.AppendCloseErr(
			ioutil.MultiCloser(true, closers...),
			fmt.Errorf("cannot create slack app manager: %w", err),
		)

		log.With("error", err).
			Error("cannot create slack app manager")

		return
	}

	closers = append([]io.Closer{slackMan}, closers...)

	webchangesClient := webchanges.NewClient(
		buildinfo.Getenv("CHANGEDETECTION_API_URL"),
		buildinfo.Getenv("CHANGEDETECTION_API_KEY"),
	)

	termCtx, termCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer termCancel()

	searchClient, err := searchgw.NewClient(
		termCtx,
		meilisearch.New(
			buildinfo.Getenv("MEILISEARCH_DSN"),
			meilisearch.WithAPIKey(buildinfo.Getenv("MEILISEARCH_MASTER_KEY")),
		),
	)
	if err != nil {
		err = ioutil.AppendCloseErr(
			ioutil.MultiCloser(true, closers...),
			fmt.Errorf("cannot create search gateway client: %w", err),
		)

		log.With("error", err).
			Error("cannot create search gateway client")

		return
	}

	storageClient, err := storage.NewClient(
		termCtx,
		storage.Options{
			URL:       buildinfo.Getenv("STORAGE_URL"),
			AccessKey: buildinfo.Getenv("STORAGE_ACCESS_KEY"),
			SecretKey: buildinfo.Getenv("STORAGE_SECRET_KEY"),
			Bucket:    buildinfo.Getenv("STORAGE_BUCKET"),
		},
	)
	if err != nil {
		err = ioutil.AppendCloseErr(
			ioutil.MultiCloser(true, closers...),
			fmt.Errorf("cannot create storage client: %w", err),
		)

		log.With("error", err).
			Error("cannot create storage client")

		return
	}

	var origins []string

	if biOrigins := strings.Split(buildinfo.Getenv("ORIGINS"), ","); len(biOrigins) > 0 {
		origins = biOrigins
	}

	emailSender, err := email.NewSender(
		log,
		email.Config{
			Host:        buildinfo.Getenv("EMAIL_SMTP_HOST"),
			Port:        buildinfo.Getenv("EMAIL_SMTP_PORT"),
			Username:    buildinfo.Getenv("EMAIL_SMTP_USERNAME"),
			Password:    buildinfo.Getenv("EMAIL_SMTP_PASSWORD"),
			TLS:         email.TLSMode(buildinfo.Getenv("EMAIL_SMTP_TLS")),
			FromAddress: buildinfo.Getenv("EMAIL_FROM_ADDRESS"),
		},
	)
	if err != nil {
		err = ioutil.AppendCloseErr(
			ioutil.MultiCloser(true, closers...),
			fmt.Errorf("cannot create email sender: %w", err),
		)

		log.With("error", err).
			Error("cannot create email sender")

		return
	}

	editClient := edit.NewClient(
		http.DefaultClient,
		buildinfo.Getenv("AUTH_REALTIME_URL"),
	)

	assistantMan := assistant.NewManager(
		log,
		dbc,
		rdb,
		buildinfo.Getenv("ANTHROPIC_API_KEY"),
		metrics,
		editClient,
		searchClient,
	)

	srv, err := server.NewServer(
		log,
		server.Options{
			PublicURL:         buildinfo.Getenv("SERVER_PUBLIC_URL"),
			DemoPrometheusURL: buildinfo.Getenv("SERVER_DEMO_PROMETHEUS_URL"),
			Port:              _serverPort,
			Origins:           origins,
			Auth: auth.Options{
				BetterAuthURL: buildinfo.Getenv("SERVER_AUTH_BETTER_AUTH_URL"),
			},
		},
		dbc,
		metrics,
		storageClient,
		assistantMan,
		githubMan,
		slackMan,
		webchangesClient,
		searchClient,
		notifMan,
		emailSender,
		http.DefaultClient,
	)
	if err != nil {
		err = ioutil.AppendCloseErr(
			ioutil.MultiCloser(true, closers...),
			err,
		)

		log.With("error", err).
			Error("cannot create a server")

		return
	}

	hooksMan := hookMan.NewManager(log, dbc, githubMan, webchangesClient, notifMan)
	searchManager := searchMan.NewManager(log, dbc, searchClient)

	closers = append([]io.Closer{srv}, closers...)

	var wg sync.WaitGroup

	wg.Go(srv.Listen)
	wg.Go(func() {
		hooksMan.Start(termCtx)
	})
	wg.Go(func() {
		searchManager.Start(termCtx)
	})

	<-termCtx.Done()

	cerr := ioutil.MultiCloser(true, closers...).Close()
	if cerr != nil {
		log.With("error", cerr).
			Error("error closing resources")
	}

	wg.Wait()
}

// newLogger initializes a new logger with Sentry support.
func newLogger() (*slog.Logger, func(), bool) {
	var level slog.Level

	buildLevel := buildinfo.Getenv("LOG_LEVEL")
	if buildLevel != "" {
		err := level.UnmarshalText([]byte(buildLevel))
		if err != nil {
			slog.Warn(`invalid log level, defaulting to "INFO"`)
		}
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})).With("version", buildinfo.Full().Version)

	dsn := buildinfo.Getenv("SENTRY_DSN")
	if dsn == "" && !buildinfo.IsDevEnv() {
		log.Error("invalid sentry dsn")
		return nil, nil, false
	}

	fn, err := sentryutil.Setup(sentryutil.Config{
		DSN:              dsn,
		Release:          buildinfo.Full().PlainVersion(),
		Environment:      buildinfo.Full().Env,
		ReleaseName:      buildinfo.Full().VersionName(),
		ReleaseCommit:    buildinfo.Full().Commit,
		ReleaseTimestamp: buildinfo.Full().FormattedTimestamp(),
		Deduplication: sentryutil.DeduplicationConfig{
			Interval: _sentryDedupInterval,
			Capacity: _sentryDedupCapacity,
		},
	})
	if err != nil {
		log.Error("cannot set up sentry")
		return nil, nil, false
	}

	return log, fn, true
}

// newMetrics initializes and starts the metrics collection.
func newMetrics(log *slog.Logger) (metricutil.Factory, func()) {
	metricsStopCh := make(chan struct{})

	metrics := metricutil.NewFactory(buildinfo.Full().Name, prometheus.NewRegistry())

	metricsCtx, metricsCancel := context.WithCancel(context.Background())

	go func() {
		defer logutil.Recover(log, nil)

		metrics.CollectRuntimeMetrics(metricsCtx, time.Second)

		close(metricsStopCh)
	}()

	return metrics, func() {
		metricsCancel()
		<-metricsStopCh
	}
}
