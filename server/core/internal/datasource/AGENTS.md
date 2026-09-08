# AGENTS.md

Outbound data-source connections and the built-in demo source. Go standards
live in [server/core/AGENTS.md](../../AGENTS.md).

## The demo data source

Every organization is seeded, unconditionally, with a Prometheus data source
"Demo" at `demo://engineering`, and the welcome document whose charts query it.

- `demo/` synthesises the metrics in-process. `runner.ensurePrepared` picks
  it for exactly that URL; any other `demo://` is `demo.ErrUnknownSource`.
  The type stays `prometheus`, so nothing downstream can tell.
- One `demo.Client` lives on `datasource.Manager` for the whole process,
  because its registry walks cache replayed history; a runner is built per
  call.
- The timeline is a pure function of the tick (fixed epoch, one-minute
  tick, per-series seed), so history never rewrites and any window
  backfills. `walk.checkpoint` caches one value per day-long segment.
- Queries run through the official `promql` engine over a
  `storage.Queryable` synthesising only the requested window, at the
  query's step.
- `registry.go` declares every metric family. `Test_welcomeDocumentQueries`
  runs every query in `internal/document/files/available_metrics.json`
  against it.

## Processor tests

`processor/` starts throwaway MariaDB and Postgres containers (gnomock) to
cover real connection, query and read-only-check paths; docker is a
unit-test dependency here, as in `internal/db`.
