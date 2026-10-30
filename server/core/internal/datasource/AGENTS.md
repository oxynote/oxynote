# AGENTS.md

Outbound data-source connections and the built-in demo source. Go standards
live in [server/core/AGENTS.md](../../AGENTS.md).

## The demo data sources

Every organization is seeded, unconditionally, with the Prometheus data
source "Demo Metrics" at `demo://engineering`, and the welcome document whose
charts query it. The Loki "Demo Logs" at the same URL is ready but stays
commented out in `NewDemoDataSources` until the web app shows loki data
sources.

- `demo/` synthesises both in-process. `Manager.newClient` picks it for
  exactly that URL under either type; any other `demo://` is
  `demo.ErrUnknownSource`. The types stay `prometheus` and `loki`, so
  nothing downstream can tell.
- One `demo.Client` lives on `datasource.Manager` for the whole process,
  because its registry walks cache replayed history. Runners share it and
  never close it.
- The timeline is a pure function of the tick (fixed epoch, one-minute
  tick, per-series seed), so history never rewrites and any window
  backfills. `walk.checkpoint` caches one value per day-long segment.
- Metric queries run through the official `promql` engine over a
  `storage.Queryable` synthesising only the requested window, at the
  query's step.
- `registry.go` declares every metric family. `Test_welcomeDocumentQueries`
  runs every query in `internal/document/files/available_metrics.json`
  against it.
- Loki's LogQL code cannot be imported (its go.mod needs 8 `replace`
  directives); `logql.go` answers the subset of selector, line filters and
  label filters.

## Connections

- `Manager` keeps one runner per data source, so its processor's
  connections serve every call. `Runner` hands out a runner it does not
  keep yet when `updatedAt` differs; the first query through it is what
  replaces the kept one (`adopt`), so testing an edit never closes the
  connections readers use. The update and delete handlers evict, as does
  a test that changes the stored status; five idle minutes close a runner.
- Calls wait on one connection test per data source. A pass holds for
  10s, a failure for 30s: each test of a broken source is a failed login.
  A failure also hands the runner's place to a fresh one that remembers
  it (`renew`), because a pooled session keeps the role settings it
  started with. `runner.TestConnection` always tests through a throwaway
  processor.
- Read-only is checked, never enforced: the connection test refuses an
  account that is not read-only (Postgres role, database or standby;
  MySQL grants). Queries run in no transaction and sessions are not reset,
  so whatever a query changes stays on its pooled connection; keeping the
  account read-only is the user's job.

## Query cache

- `Prometheus`, `PostgreSQL`, `MySQL` and `Logs` hand out clients whose
  query methods answer from a 10s cache: Valkey when core has a pool,
  memkit capped at 64 MiB otherwise. Metadata, labels and `SQL()` are not
  cached.
- The key hashes the data source ID and `updatedAt`, the method, its
  arguments and the aligned range. Prometheus keys on the step-aligned
  range but is given the raw one: it aligns it itself, and aligning twice
  can widen the window. SQL and logs key and run on a 15s grid, so a
  window widens by under 15s rather than by a step that can reach 24h.
  Errors
  and answers without data are never kept.
- Identical calls share one run on a context no caller can cancel, so
  every processor method must bound itself with a timeout.
- Answers are kept as JSON and must decode to what their readers expect:
  `PrometheusQueryResult.UnmarshalJSON` restores the `model.Matrix` the
  assistant's caps assert, and MySQL keeps `ColumnTypes` to classify
  columns.

## Processor tests

`processor/` starts throwaway MariaDB, Postgres and Loki (oldest supported
and newest) containers (gnomock) to cover real connection, query and
read-only-check paths; docker is a unit-test dependency here, as in
`internal/db`. Loki holds a 6000-line corpus built to break paging (one
timestamp holding more lines than a page); its config keeps backfilled
lines in the ingesters, which otherwise flush them out of query reach.
