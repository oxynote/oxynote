// Package main is the entry point of the oxynote-core API server.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/gomodule/redigo/redis"
	"github.com/jellydator/xync"
	"github.com/meilisearch/meilisearch-go"
	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/apps/slack"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/oxynote/oxynote/server/core/internal/assistant"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/assistant/provider"
	"github.com/oxynote/oxynote/server/core/internal/buildinfo"
	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/db"
	fileMan "github.com/oxynote/oxynote/server/core/internal/document/file/manager"
	hookMan "github.com/oxynote/oxynote/server/core/internal/document/hook/manager"
	"github.com/oxynote/oxynote/server/core/internal/email"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/notification/interpreter"
	"github.com/oxynote/oxynote/server/core/internal/search"
	searchMan "github.com/oxynote/oxynote/server/core/internal/search/manager"
	"github.com/oxynote/oxynote/server/core/internal/server"
	"github.com/oxynote/oxynote/server/core/internal/storage"
	storageFS "github.com/oxynote/oxynote/server/core/internal/storage/fs"
	storageS3 "github.com/oxynote/oxynote/server/core/internal/storage/s3"
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

	// _maxSlackMessages specifies the maximum number of Slack messages
	// retained per organization.
	_maxSlackMessages = 500

	// _defaultAssistantMaxTokens caps the assistant's response length for
	// one turn when the environment says nothing.
	_defaultAssistantMaxTokens = 64000

	// _defaultMaxDocumentHistoryEntries specifies how many history entries
	// are retained per document branch when the environment says nothing.
	_defaultMaxDocumentHistoryEntries = 100

	// _defaultDocumentHistoryRetention specifies how long history entries
	// are retained when the environment says nothing.
	_defaultDocumentHistoryRetention = time.Hour * 24 * 90

	// _editClientTimeout bounds one edit-operations request to the Node
	// service.
	_editClientTimeout = 30 * time.Second

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

	maxDocumentHistoryEntries, err := parseUintEnv("DB_MAX_DOCUMENT_HISTORY_ENTRIES", _defaultMaxDocumentHistoryEntries)
	if err != nil {
		fail(log, closers, "cannot read the configuration", err)
		return
	}

	documentHistoryRetention, err := parseDurationEnv(
		"DB_DOCUMENT_HISTORY_RETENTION",
		_defaultDocumentHistoryRetention,
	)
	if err != nil {
		fail(log, closers, "cannot read the configuration", err)
		return
	}

	dbc, err := db.New(log, metrics, db.Options{
		DSN:                                buildinfo.Getenv("DB_DSN"),
		MaxNotifications:                   _maxNotifications,
		MaxSlackMessages:                   _maxSlackMessages,
		MaxDocumentHistoryEntries:          maxDocumentHistoryEntries,
		DocumentHistoryRetention:           documentHistoryRetention,
		DataSourceCredentialsSigningSecret: buildinfo.Getenv("DB_DATA_SOURCE_CREDENTIALS_SIGNING_SECRET"),
	})
	if err != nil {
		fail(log, closers, "cannot create a database connection", err)
		return
	}

	closers = append([]io.Closer{dbc}, closers...)

	// an empty VALKEY_DSN is a deployment running without valkey at
	// all. The nil pool is what tells the assistant to keep its
	// conversations in this process instead.
	var rdb *redis.Pool

	if dsn := buildinfo.Getenv("VALKEY_DSN"); dsn != "" {
		rdb, err = redisutil.NewPool(dsn)
		if err != nil {
			fail(log, closers, "cannot create a redis pool", err)
			return
		}

		closers = append([]io.Closer{rdb}, closers...)
	}

	// an empty GITHUB_APP_ID means the GitHub App integration is disabled.
	// The zero app ID makes github.NewManager return an unconfigured
	// manager instead of failing.
	var githubAppID int64

	if v := buildinfo.Getenv("GITHUB_APP_ID"); v != "" {
		githubAppID, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			fail(log, closers, "cannot parse GITHUB_APP_ID", err)
			return
		}
	}

	githubMan, err := github.NewManager(dbc, github.Options{
		AppID:                     githubAppID,
		AppSlug:                   buildinfo.Getenv("GITHUB_APP_SLUG"),
		SignatureSecret:           buildinfo.Getenv("GITHUB_SIGNATURE_SECRET"),
		PrivateKeyPath:            buildinfo.Getenv("GITHUB_PRIVATE_KEY_PATH"),
		InstallationSigningSecret: buildinfo.Getenv("GITHUB_INSTALLATION_SIGNING_SECRET"),
	})
	if err != nil {
		fail(log, closers, "cannot create github app manager", err)
		return
	}

	notifMan := notification.NewManager(log, dbc)
	closers = append([]io.Closer{notifMan}, closers...)

	slackMan, err := slack.NewManager(
		log,
		dbc,
		interpreter.NewInterpreter(dbc, interpreter.NewSlackFormatter(), buildinfo.Getenv("BASE_APP_URL")),
		notifMan,
		http.DefaultClient,
		slack.Options{
			RedirectURL:               buildinfo.Getenv("SLACK_REDIRECT_URL"),
			ClientID:                  buildinfo.Getenv("SLACK_CLIENT_ID"),
			ClientSecret:              buildinfo.Getenv("SLACK_CLIENT_SECRET"),
			SignatureSecret:           buildinfo.Getenv("SLACK_SIGNATURE_SECRET"),
			InstallationSigningSecret: buildinfo.Getenv("SLACK_INSTALLATION_SIGNING_SECRET"),
		},
	)
	if err != nil {
		fail(log, closers, "cannot create slack app manager", err)
		return
	}

	closers = append([]io.Closer{slackMan}, closers...)

	webchangeClient := webchange.NewClient(
		buildinfo.Getenv("CHANGEDETECTION_API_URL"),
		buildinfo.Getenv("CHANGEDETECTION_API_KEY"),
	)

	termCtx, termCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer termCancel()

	// an empty MEILISEARCH_URL means search is disabled: nothing is
	// indexed and the search surfaces refuse. A set URL that cannot be
	// reached stays a boot error inside NewClient.
	var meiliMan meilisearch.ServiceManager

	if url := buildinfo.Getenv("MEILISEARCH_URL"); url != "" {
		meiliMan = meilisearch.New(
			url,
			meilisearch.WithAPIKey(buildinfo.Getenv("MEILISEARCH_MASTER_KEY")),
		)
	}

	searchClient, err := search.NewClient(termCtx, meiliMan)
	if err != nil {
		fail(log, closers, "cannot create search gateway client", err)
		return
	}

	searchJobs := search.NewJobs(searchClient.Configured())

	objectStorageURL := buildinfo.Getenv("OBJECT_STORAGE_URL")

	var storageClient storage.Store

	if objectStorageURL != "" {
		storageClient, err = storageS3.NewClient(
			termCtx,
			storageS3.Options{
				URL:       objectStorageURL,
				Region:    buildinfo.Getenv("OBJECT_STORAGE_REGION"),
				AccessKey: buildinfo.Getenv("OBJECT_STORAGE_ACCESS_KEY"),
				SecretKey: buildinfo.Getenv("OBJECT_STORAGE_SECRET_KEY"),
				Bucket:    buildinfo.Getenv("OBJECT_STORAGE_BUCKET"),
			},
		)
	} else {
		storageClient, err = storageFS.NewClient(buildinfo.Getenv("OBJECT_STORAGE_LOCAL_PATH"))
	}

	if err != nil {
		fail(log, closers, "cannot create storage client", err)
		return
	}

	origins := parseOrigins(buildinfo.Getenv("ORIGINS"))

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
		fail(log, closers, "cannot create email sender", err)
		return
	}

	// the sender delivers on its own goroutines, so shutdown drains them.
	closers = append([]io.Closer{emailSender}, closers...)

	editClient := edit.NewClient(
		&http.Client{Timeout: _editClientTimeout},
		buildinfo.Getenv("AUTH_REALTIME_URL"),
	)

	chatModel, summaryModel, assistantOpts, assistantStatus, err := assistantModels(termCtx, log)
	if err != nil {
		fail(log, closers, "cannot set up the assistant", err)
		return
	}

	datasourceMan := datasource.NewManager(log, dbc)

	assistantMan := assistant.NewManager(
		log,
		dbc,
		rdb,
		chatModel,
		summaryModel,
		metrics,
		editClient,
		searchClient,
		searchJobs,
		datasourceMan,
		string(assistantOpts.Provider),
	)

	// a disabled integration is a deliberate state, but it is also the
	// first thing an operator looks for when a feature is missing from
	// the product, so each one announces itself at boot.
	warnDisabled(log, githubMan.Configured(), "github app integration is disabled")
	warnDisabled(log, slackMan.Configured(), "slack app integration is disabled")
	warnDisabled(log, assistantMan.Configured(), "assistant is disabled")
	warnDisabled(log, searchClient.Configured(), "search is disabled")
	warnDisabled(log, webchangeClient.Configured(), "changedetection integration is disabled")

	serverHost, serverPort, err := parseServerAddress(buildinfo.Getenv("SERVER_ADDRESS"))
	if err != nil {
		fail(log, closers, "cannot read the configuration", err)
		return
	}

	srv, err := server.NewServer(
		log,
		server.Options{
			PublicURL: buildinfo.Getenv("SERVER_PUBLIC_URL"),
			Host:      serverHost,
			Port:      serverPort,
			Origins:   origins,
			Assistant: server.AssistantCapability{
				Status: assistantStatus,
				Model:  assistantOpts.Model,
			},
			Auth: server.AuthOptions{
				BetterAuthURL: buildinfo.Getenv("SERVER_AUTH_BETTER_AUTH_URL"),
			},
			MCP: server.MCPOptions{
				SessionURL:  buildinfo.Getenv("SERVER_MCP_SESSION_URL"),
				ResourceURL: buildinfo.Getenv("SERVER_MCP_RESOURCE_URL"),
			},
		},
		dbc,
		metrics,
		storageClient,
		assistantMan,
		datasourceMan,
		githubMan,
		slackMan,
		webchangeClient,
		searchClient,
		searchJobs,
		notifMan,
		emailSender,
		http.DefaultClient,
	)
	if err != nil {
		fail(log, closers, "cannot create a server", err)
		return
	}

	hooksMan := hookMan.NewManager(log, dbc, githubMan, webchangeClient, notifMan)
	filesMan := fileMan.NewManager(log, dbc, storageClient, fileMan.Options{
		HistoryRetention: documentHistoryRetention,
	})

	closers = append([]io.Closer{srv}, closers...)

	// the managers own the process's periodic work, so they run under a
	// supervisor that contains a panic none of their own recovery plans
	// caught rather than letting it take the process down.
	backgroundSupv := xync.NewSupervisor(
		xync.WithSupervisorBaseContext(termCtx),
		xync.WithSupervisorRecovery(
			logutil.RecoveryValue(log, logutil.NewRecoveryPlan("recovered from a panic in a background manager")),
		),
	)
	defer backgroundSupv.Close()

	var serverWg sync.WaitGroup

	serverWg.Go(func() {
		srv.Listen()

		// Listen also returns when the listener could not be bound at all,
		// which has to bring the process down rather than leave it up with
		// no server answering.
		termCancel()
	})

	backgroundSupv.Go(hooksMan.Start)
	backgroundSupv.Go(filesMan.Start)
	backgroundSupv.Go(assistantMan.Start)

	// without search there are no queued jobs to drain, so the manager
	// is not started at all.
	if searchClient.Configured() {
		backgroundSupv.Go(searchMan.NewManager(log, dbc, searchClient).Start)
	}

	<-termCtx.Done()

	// the managers query the database and redis, so let them finish their
	// current pass before the closer chain tears those pools down.
	backgroundSupv.Wait()

	cerr := ioutil.MultiCloser(true, closers...).Close()
	if cerr != nil {
		log.With("error", cerr).
			Error("error closing resources")
	}

	// closing the server above is what unblocks Listen.
	serverWg.Wait()
}

// fail logs a wiring failure and releases everything opened so far. The
// message is logged before the closers run, since the log flusher is itself
// one of them and would otherwise flush before the message it must carry.
func fail(log *slog.Logger, closers []io.Closer, msg string, err error) {
	log.With("error", err).Error(msg)

	if cerr := ioutil.MultiCloser(true, closers...).Close(); cerr != nil {
		log.With("error", cerr).Error("cannot release the opened resources")
	}
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

	// an empty DSN disables sentry; sentryutil.Setup tolerates it.
	dsn := buildinfo.Getenv("SENTRY_DSN")

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

		// deferred so a recovered panic still releases the shutdown wait
		// below instead of blocking it forever.
		defer close(metricsStopCh)

		metrics.CollectRuntimeMetrics(metricsCtx, time.Second)
	}()

	return metrics, func() {
		metricsCancel()
		<-metricsStopCh
	}
}

// parseUintEnv reads an unsigned integer from the environment, falling back
// to the given default when the variable is unset.
func parseUintEnv(name string, def uint64) (uint64, error) {
	v := buildinfo.Getenv(name)
	if v == "" {
		return def, nil
	}

	res, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}

	return res, nil
}

// parseDurationEnv reads a duration from the environment, falling back to
// the given default when the variable is unset.
func parseDurationEnv(name string, def time.Duration) (time.Duration, error) {
	v := buildinfo.Getenv(name)
	if v == "" {
		return def, nil
	}

	res, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}

	return res, nil
}

// parseServerAddress splits a listen address into its host and port parts,
// falling back to all interfaces on the default port when the value is
// empty. An empty host part binds all interfaces.
func parseServerAddress(v string) (string, uint, error) {
	if v == "" {
		return "", _serverPort, nil
	}

	host, rawPort, err := net.SplitHostPort(v)
	if err != nil {
		return "", 0, fmt.Errorf("invalid SERVER_ADDRESS: %w", err)
	}

	port, err := strconv.ParseUint(rawPort, 10, 16)
	if err != nil {
		return "", 0, fmt.Errorf("invalid SERVER_ADDRESS: %w", err)
	}

	return host, uint(port), nil
}

// warnDisabled announces a disabled integration at boot.
func warnDisabled(log *slog.Logger, configured bool, msg string) {
	if configured {
		return
	}

	log.Warn(msg)
}

// parseOrigins splits the configured allowed-origin list. An empty value
// yields no origins at all, which is what leaves origin checking disabled:
// strings.Split would instead return a one-element slice holding "", which
// matches no browser origin and so blocks every cross-origin request.
func parseOrigins(val string) []string {
	if val == "" {
		return nil
	}

	return strings.Split(val, ",")
}

// assistantProviderOptions assembles the assistant's model configuration
// from the environment. An empty ASSISTANT_MODEL falls back to the
// provider's default model, and provider-specific credentials are only
// read for the provider actually selected, so an operator running
// Ollama is never asked about AWS regions.
func assistantProviderOptions() (provider.Options, error) {
	opts := provider.Options{
		Provider: provider.ParseProvider(buildinfo.Getenv("ASSISTANT_PROVIDER")),
		Model:    buildinfo.Getenv("ASSISTANT_MODEL"),
		APIKey:   buildinfo.Getenv("ASSISTANT_API_KEY"),
		BaseURL:  buildinfo.Getenv("ASSISTANT_BASE_URL"),
	}

	if opts.Model == "" {
		opts.Model = opts.Provider.DefaultModel()
	}

	maxTokens, err := parseUintEnv("ASSISTANT_MAX_TOKENS", _defaultAssistantMaxTokens)
	if err != nil {
		return provider.Options{}, err
	}

	opts.MaxTokens = int(maxTokens)

	timeout, err := parseDurationEnv("ASSISTANT_REQUEST_TIMEOUT", 0)
	if err != nil {
		return provider.Options{}, err
	}

	opts.RequestTimeout = timeout

	switch opts.Provider {
	case provider.ProviderOpenAI:
		opts.Azure = provider.AzureOptions{
			Enabled:    buildinfo.Getenv("ASSISTANT_AZURE_API_VERSION") != "",
			APIVersion: buildinfo.Getenv("ASSISTANT_AZURE_API_VERSION"),
		}
	case provider.ProviderAnthropic:
		opts.Bedrock = provider.BedrockOptions{
			Enabled:         buildinfo.Getenv("ASSISTANT_BEDROCK_REGION") != "",
			Region:          buildinfo.Getenv("ASSISTANT_BEDROCK_REGION"),
			AccessKey:       buildinfo.Getenv("ASSISTANT_BEDROCK_ACCESS_KEY"),
			SecretAccessKey: buildinfo.Getenv("ASSISTANT_BEDROCK_SECRET_ACCESS_KEY"),
			SessionToken:    buildinfo.Getenv("ASSISTANT_BEDROCK_SESSION_TOKEN"),
		}
		opts.Vertex = provider.VertexOptions{
			Enabled:            buildinfo.Getenv("ASSISTANT_VERTEX_PROJECT_ID") != "",
			ProjectID:          buildinfo.Getenv("ASSISTANT_VERTEX_PROJECT_ID"),
			Region:             buildinfo.Getenv("ASSISTANT_VERTEX_REGION"),
			ServiceAccountJSON: []byte(buildinfo.Getenv("ASSISTANT_VERTEX_SERVICE_ACCOUNT_JSON")),
		}
	case provider.ProviderGoogle, provider.ProviderOllama, provider.ProviderOpenRouter:
		// no vendor-specific settings beyond the common ones.
	}

	return opts, nil
}

// assistantModels reads the assistant configuration, judges the
// configured model, and builds the chat and summarization models when
// the verdict lets the assistant run. An empty ASSISTANT_PROVIDER means
// the assistant is disabled: nothing is built, the status stays
// inactive, and the remaining ASSISTANT_* values are ignored, matching
// the other integrations. A set provider still validates everything it
// needs and fails boot, but the model itself is judged rather than
// trusted: one too weak to run the assistant, or one outside the
// provider's supported list, disables the assistant with a warning
// instead of failing boot, and the capabilities endpoint reports why.
func assistantModels(ctx context.Context, log *slog.Logger) (
	model.ToolCallingChatModel,
	model.ToolCallingChatModel,
	provider.Options,
	provider.Status,
	error,
) {
	if buildinfo.Getenv("ASSISTANT_PROVIDER") == "" {
		return nil, nil, provider.Options{}, provider.StatusInactive, nil
	}

	opts, err := assistantProviderOptions()
	if err != nil {
		return nil, nil, provider.Options{}, provider.StatusInactive, fmt.Errorf("reading the configuration: %w", err)
	}

	status, err := opts.ModelStatus()
	if err != nil {
		return nil, nil, provider.Options{}, provider.StatusInactive, fmt.Errorf("reading the configuration: %w", err)
	}

	switch status {
	case provider.StatusActive, provider.StatusInactive:
		// nothing to warn about: full strength needs no caveat, and
		// inactive only arrives alongside an error handled above.
	case provider.StatusActiveButWeak:
		log.Warn("assistant model is weaker than recommended", slog.String("model", opts.Model))
	case provider.StatusInactiveTooWeak:
		log.Warn("assistant model is too weak to run the assistant", slog.String("model", opts.Model))
	}

	if !status.Active() {
		return nil, nil, opts, status, nil
	}

	chatModel, err := provider.New(ctx, opts)
	if err != nil {
		return nil, nil, provider.Options{}, provider.StatusInactive, fmt.Errorf("creating the chat model: %w", err)
	}

	summaryModel, err := assistantSummaryModel(ctx, opts, chatModel)
	if err != nil {
		return nil, nil, provider.Options{}, provider.StatusInactive, fmt.Errorf("creating the summarization model: %w", err)
	}

	return chatModel, summaryModel, opts, status, nil
}

// assistantSummaryModel builds the model used to summarise long
// conversations. It defaults to the chat model, so summarisation costs
// nothing to configure; pointing it at a cheaper model is opt-in.
func assistantSummaryModel(
	ctx context.Context,
	opts provider.Options,
	chatModel model.ToolCallingChatModel,
) (model.ToolCallingChatModel, error) {
	name := buildinfo.Getenv("ASSISTANT_SUMMARY_MODEL")
	if name == "" {
		return chatModel, nil
	}

	opts.Model = name

	return provider.New(ctx, opts)
}
