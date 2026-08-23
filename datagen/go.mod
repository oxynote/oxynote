module github.com/oxynote/oxynote/datagen

go 1.27.0

require (
	github.com/go-chi/chi/v5 v5.3.0
	github.com/go-sql-driver/mysql v1.10.0
	github.com/jackc/pgx/v5 v5.9.2
	github.com/oxynote/oxynote/server/core v0.0.0
	github.com/prometheus/client_golang v1.23.2
)

replace github.com/oxynote/oxynote/server/core => ../server/core

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/getsentry/sentry-go v0.46.2 // indirect
	github.com/gorilla/schema v1.4.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/procfs v0.20.1 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/rs/xid v1.6.0 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
