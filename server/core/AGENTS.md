# AGENTS.md — Go standards

Mandatory for all Go code in `server/core` and `datagen`; this file wins
over surrounding code for new and modified code. Shared helpers live in
`pkg/`; supervisors come from `github.com/jellydator/xync`. Where this file
names an exemplar file, copy its shape.

```sh
make build          # goreleaser snapshot -> bin/ + ghcr.io/oxynote/core:dev
make test           # go test -race ./...
make check-coverage # fails below COVERAGE_MIN
make lint           # golangci-lint run --fix;  check-lint = verify only
```

## Layout

- `cmd/core/main.go`: the only place that reads the environment, builds the
  root `Options` and wires everything. `log.Fatal` only here.
- `internal/`: all business logic, nested to mirror the domain tree; a
  nested `internal/` locks sub-packages to their parent. `pkg/`:
  dependency-light, domain-agnostic helpers only.
- Package names short and lowercase; helper siblings `<parentabbrev><role>`
  (`redkit`) or a bare role when nesting disambiguates. Files snake_case.
- Generated mocks: in-package `0mock_<name>_test.go` (sorts first) and
  exported `_mock/` sibling packages (`package mock`).
- Every package has `// Package x <verb phrase>.` on its primary file.

## Style

**Comments.** Every comment ends with a period (godot). Doc comments on
everything, unexported included: `// <Name> <indicative verb phrase>.`;
interface methods use `should`, implementations indicative. `// NOCOV:
<lowercase reason>.` is the first line of a branch that genuinely cannot be
reproduced; it is a reviewer covenant, not a tool directive, and never
excuses a testable path.
`// NOTE: <why>.` for design rationale. `//nolint:<linter> // <lowercase
reason>`, reason mandatory (deferred Rollback/Close: `errcheck // error
provides no meaningful info`). Comments never narrate the next line, justify
a diff, or point at design docs or agent-instruction files.

**Globals.** Every unexported package-level var/const is `_`-prefixed and
doc-commented (`var` for values tests shorten). Exported globals: `Err*`
sentinels only. `init()` is banned except for build-info parsing.
Compile-time interface assertions (`var _ Iface = &Impl{}`) only when a
human asks: interfaces are consumer-declared, so the assertion imports the
consumer and risks a cycle.

**Naming.** Receivers are 1–3 letters from the type's initials, consistent
per type. Shorts: `cfg`, `opts`, `inp`, `supv`, `res`, `evt`; secondary
errors `cerr`/`rerr`/`perr`. Acronyms stay uppercase (`httpClient`, `ID`),
except `Ws` in compounds. `any`, never `interface{}`. Unused parameters are
`_`. An exported identifier used only inside its package becomes unexported;
an unused one is deleted.

**Shape (wsl).** Guard clauses, no `else` after a returning `if`. `if err :=
f(); err != nil {` when `err` is not reused. Blank line before every `return`
(unless sole statement), before each new logical step, and after a
`Lock`/`defer Unlock` pair. Multi-argument calls one parameter per line.
Named returns only so a `defer` can observe or replace the result; naked
returns are banned. `defer cancel()`/`defer mu.Unlock()`/`defer
resp.Body.Close()` immediately after the acquiring call. Imports: one stdlib
block, then one block of everything else; alias collisions as lowercase
concatenations (`hookMan`), mocks as `<pkg>Mock`.

## API design

**Interfaces are consumer-side.** A package declares an interface for every
dependency, listing only the methods it calls, at the bottom of the file
that uses them. Interfaces are small (1–4 methods); larger ones only as
facades composed by embedding. The `//go:generate` mock directive sits in
the doc block after a bare `//` line.

**Constructors.** `New` for the package namesake, else `New<Type>` /
`new<Type>`. Parameters: `ctx` (only if I/O) → `log` → dependencies → `opts
Options` last; first statement `opts.validate()`. `(T, error)` only if
construction can fail. Multi-stage constructors clean up with
`ioutil.MultiCloser`/`AppendCloseErr`, as in `cmd/core/main.go`. Every
resource-owning type has `Close() error`.

**Options are plain structs**, never functional options: one `Options` per
configurable package, next to its constructor, every field doc-commented
with units and zero-value behaviour. Options nest to mirror the dependency
tree and `validate()` cascades (exported `Validate()` when a parent cascades
into it). Config is environment only, read once in `main`.

**Structs.** Service structs: unexported fields ordered `log *slog.Logger`,
dependencies, each mutex directly above the fields it guards (blank lines
around the group), `supv`, `opts` last. Composite literals with any field or
more than one element break one element per line with a trailing comma,
except test-table rows. Data structs: exported fields separated by blank
lines, `// X specifies ...` (`// X indicates ...` for booleans), tags
`json:"camelCase" db:"snake_case"`, foreign keys `db:"fk_<table>_id"`,
embedded fields first. Tiny stateless embeds are the mixin mechanism.

## Errors

- User-facing: `errutil.New(httpStatus, "domain.reason", "lowercase
  message, no punctuation")` (plus `Wrap`, `NewWithData`, `StatusCode`,
  `InternalCode`, `ErrNotFound`); codes are dotted literals written inline.
  Internal: `errors.New("lowercase message")`.
- Sentinels are `Err*` vars documented `// ErrX is returned when ...`.
  Sentinel handling nests inside a single `if err != nil`; `errors.Is` never
  runs on the success path.
- Never discard `json.Unmarshal` errors. Permission checks are methods
  returning `error` (`AllowsX() error`).
- Wrap with `fmt.Errorf("<gerund phrase>: %w", err)` at every hop that adds
  context.
- At trust boundaries classify once in `error.go`: 4xx pass through,
  5xx/unknown are reported and replaced by an opaque `ErrInternal`.
- No `panic` in production code except programmer-error guards in
  reflection plumbing. Multierror only for parallel fan-out and close paths.

## Logging (slog)

`log *slog.Logger` is the first field and first constructor parameter. Child
loggers are made at wiring time (`log.With("component", ...)`). Message
first, then typed attrs one per line when two or more
(`slog.String("error", err.Error())`); messages lowercase, no period, no
interpolated errors, `"cannot <verb> <object>"`; keys snake_case.
`logutil.Critical(log, err).Error(...)` reports to Sentry for anything a
human must act on. Levels: Error/Critical actionable, Warn skipped malformed
input, Info lifecycle only, Debug teardown detail. **Log or return, never
both**; a swallowed error is logged with a comment saying why.

## Concurrency

- Every goroutine is owned by an `xync.Supervisor`; raw `go` only in
  `main`; never `sync.WaitGroup` directly.
- Mutexes named `mu` (or `<thing>Mu`); lock-then-defer-unlock. For
  operations a caller may abandon use a context-aware mutex
  (`syncutil.Mutex`, to be ported into `pkg/` before first use).
- Callback registries return an unsubscribe closure; dispatch reads under
  `RLock` and fans out through the supervisor, never under the write lock.
- Periodic work: `timeutil.NewPeriodicExec` or `timeutil.NewCron`, never
  hand-rolled tickers. Shutdown via a once-closed `chan struct{}` exposed as
  `ShutDownCh()`. Sleeps `select` on `ctx.Done()`, never bare `time.Sleep`.
- Retries: `cenkalti/backoff` with context and max count. Bounded parallel
  work: `errgroup`/`multierror.Group` plus a mutex for shared writes.

## Time, IDs, numbers, enums, validation

- `timeutil.Now()`, never `time.Now()` in production code. Durations are
  `_`-prefixed documented package vars.
- IDs are `rs/xid`. Exact numerics are `shopspring/decimal`; non-trivial
  decimal math lives in `pkg/mathutil`, never inline.
- Nullable columns use `guregu/null/v5`, never empty-string sentinels.
- Typed string enums: the `const` block precedes the type, values
  kebab-case, each variant documented (or one `// All available X
  constants.` header). `iota` only for context keys, bitmasks, or `iota + 1`
  when zero means unset. Enums implement `Validate() error` (returning a
  package `ErrInvalidX`) plus `MarshalText`/`UnmarshalText` with trim +
  lowercase. JSON-in-SQL types implement `driver.Valuer`/`sql.Scanner`
  exactly like `internal/notification/notification.go`.
- User-input domain types implement `ValidateWithContext(ctx) error` with
  `jellydator/validation` (value receiver, one `ValidateStructWithContext`
  call, fields in declaration order, embedded configs first; decimal bounds
  via `.CmpFunc(mathutil.CmpDecimal)`). Options structs use hand-rolled
  `validate()` with `errors.New`. Aggregate mutations go through `Set*`
  methods returning `(changed bool, err error)`, orchestrated by
  `ApplyInput(inp)`.

## Database — the agent pattern

`internal/db/db.go` is the implementation: an unexported `agent` embedded in
both `DB` and `Tx`, so **every query method is defined on `*agent`** and
available in both modes. `DB.BeginTx(ctx, dest any)` sets a `*Tx` into the
pointer it is given; `sqlutil.WrapTx` runs a multi-statement write
atomically from inside an agent method, reusing an ambient transaction.

Each consumer package declares, at the bottom of its main file, `DBAgent`
(the query methods), `DB` (`sqlutil.DB` + `DBAgent`) and `Tx` (`sqlutil.Tx`
+ `DBAgent`), each with its `//go:generate` mock directive. Parents compose
children's `DB` by embedding, so one `*db.DB` satisfies the tree; leaf
consumers that never transact declare a flat `DB`.

The transaction idiom: `var tx Tx; err := m.db.BeginTx(ctx, &tx)`; `defer
tx.Rollback() //nolint:errcheck // error provides no meaningful info`;
statements; `tx.Commit()`; **in-memory state is mutated only after a
successful commit**.

**Entity files** (`internal/db/document.go` is the exemplar): one per
entity, row structs in the domain packages, sections ordered `Create*`,
`Fetch*`, `Update*`/`Mark*`, `Delete*`/`Cleanup*`, `select<Entity>` (the
column list, `FROM`, and the mandatory owner/organization scope), then the
`apply<Entity>Filter`/`Sort` whitelists. Methods are
`<Verb><Entity>[By<Key>]`. squirrel with `.MustSql()`, then sqlx;
pagination through `sqlutil.Select`; empty results are a non-nil empty
slice; always a deterministic tiebreaker after the user sort. IDs are
generated in the domain layer, no `RETURNING`. Soft deletes set
`deleted_at`; hard deletes happen in `Cleanup*` sweeps. A sqlhooks error
hook maps `sql.ErrNoRows` → `errutil.ErrNotFound` and constraint violations
→ user errors, so callers only `errors.Is`. Migrations are
`migrations/NNN_snake_case.sql`, embedded with `//go:embed`.

## HTTP & WebSocket

- `internal/server` owns router, auth, metrics; handlers live in
  `server/internal/...` mirroring the domain tree, each leaf exactly
  `http.go` + `ws.go`, with its narrow `DB` at the bottom of `http.go`.
  Handler packages do not register routes: `router.go` owns the URL tree in
  `http<Area>Router()` functions, with trust zones as explicit, commented
  mounts.
- `Handler` holds `log` + process-wide deps; methods are plain `func (h
  *Handler) X(w, r)` named `<DomainVerb><Entity>`, never the HTTP verb:
  `FetchX`/`FetchXs` (not `ListX`), action verbs (`DuplicateDocument`),
  `CreateX`/`UpdateX`/`DeleteX`; `Handle` only where the route has no
  entity (`HandleChat`).
- A session handler's first statement is `auth.RequireSession(h.log, w, r)`.
  Decoding only through `DecodeJSON`, `DecodeForm`, `ParseQuery`,
  `ExtractTargetID`; responding only through `Respond` and `RespondError`,
  then `return`. Mutations 204, creates 201 + Location. Handlers never build
  error payloads.
- `ws.go` holds only `Bind*` methods: subscribe to a domain callback on the
  first subscriber, unsubscribe on the last, republish inline anonymous
  payloads. Topics are registered next to the router as
  `<event>@<domain.path>`.

## Metrics

A factory `fc` is threaded through constructors; the registry is created
once in `main`. Each instrumented package has `metrics.go` with an
unexported `metrics` struct + `newMetrics(fc)`. Counters end `_total`,
histograms carry a unit suffix, `Help` is a sentence ending with a period.

## Testing

White-box, same package, every success and failure path, every dependency
mocked with moq. Only `internal/db` and `internal/datasource/processor`
touch real databases.

**Naming & layout.** One test function per production function, never
merged or split; test files pair 1:1 (`expand.go` → `expand_test.go`).
`Test_<Type>_<Method>` / `Test_<Func>`, unexported names keeping their
casing (`Test_agent_CreateNotification`); only `TestMain` lacks the
underscore. Every package has `func TestMain(m *testing.M) {
goleak.VerifyTestMain(m) }`, with `IgnoreCurrent`/`IgnoreTopFunction` only
for known third-party leaks. Serialization types get `Test_X_UnmarshalJSON`,
`Test_X_MarshalJSON` (`assert.JSONEq`), `Test_X_Value`, `Test_X_Scan`.

**Table tests** are the default for anything with more than one path: a
`map[string]struct{...}` named `cc`, iterated `for cn, c := range cc` with
`t.Run(cn, ...)` and `t.Parallel()` first, then
`testutil.AssertEqualError(t, c.Err, err)` and `if err != nil { return }`
before the result assertions.

- Case names are sentence case without punctuation (`"Successful
  creation"`, `"Context cancelled"`); error paths name the collaborator
  verbatim (`"Error returned by Tx.Commit"`). No `name` field.
- Case fields are PascalCase, inputs and collaborators first (`DB`, `Tx`,
  `Inp`, `JSON`, `Context`), expectations (`Result`, `Err`, `Checks`,
  `RespCode`, `RespJSON`) last.
- **Cases live in the table, never in their own function.** A scenario that
  does not fit earns a new field (a flag, or a closure such as `Check
  func(t *testing.T, got X)`). Helpers exist only when reused (`prep*`,
  `stub*`, `check`). Cases needing setup or a retained mock reference hoist
  `type tcase struct` and build rows with immediately-invoked closures.
- Single-path functions may use a linear test with `// error` / `//
  success` phases. Constructor tests exercise the error path,
  `require.NotNil`, then assert every field.

**The `assert.AnError` protocol.** `Err` has three states: nil → no error;
`assert.AnError` → some error; a concrete error → deep equality. Failing
mocks return `assert.AnError`. Never `assert.ErrorIs`/`errors.Is` in tests.

**require vs assert.** `require` when the test cannot continue (`Len`
before indexing, `NotNil` before dereferencing); `assert` for outcomes.
`testutil.AssertFilterEqual(t, exp, act, xid.ID{}, time.Time{})` ignores
nondeterministic types; `assert.JSONEq` for JSON; decimals as strings.

**Mocks.** `//go:generate ../../scripts/codegen/mock -t internal DB db`
(in-package `DBMock`), `-t external` (importable `_mock/` package,
`mock.DB`), `-t both`. All use `-stub`, so `&DBMock{}` is a valid don't-care
collaborator that still records calls. `_mock/` holds only generated code;
third-party mocks go in `internal/_mock`. Configure by struct literal with
only the needed funcs; repeated shapes are local `stub*` closures
parameterized by the errors to return. Assert calls only through recorders
(`db.CreateItemCalls()`); test `On*` APIs by invoking the captured callback.

**check/checks combinator.** For functions touching several mocks, a
`Checks []check` field replaces per-field expectations: `type check
func(*testing.T, *DBMock, *TxMock, error)`, builders named strictly **`has*`
for state/outputs, `was*Called(count)` for call counts and params**,
zero-count checks included, run in the subtest epilogue. Exemplar:
`internal/server/internal/notification/http_test.go`.

**Transactions.** The DB mock injects the Tx mock through `dest` as the real
`BeginTx` does, via a `stubDB(tx, beginErr)` closure, and the `*TxMock` is
stored on the case for `was*` checks. Standard cases: BeginTx error, each
statement's error, Commit error, success, each with exact commit/rollback
counts. Exemplar: `internal/assistant/tools/document_test.go`.

**Database-layer tests** (`internal/db/db_test.go`): `TestMain` starts one
gnomock Postgres per package run; `prepTempDB(t)` gives each subtest its own
migrated database, which is what makes `t.Parallel()` safe. Tables are maps
of case constructors `func(*testing.T, *DB) tcase`. Round times to
microseconds before insert; verify by re-querying through the production
select builders.

**HTTP handlers.** Invoke methods directly, no router:
`httptest.NewRequest` against `http://test.com/`, chi params via
`testutil.AddChiCtx` (an `OmitID bool` case field drives the missing-param
path). Assert `rec.Code` and `assert.JSONEq` against verbatim envelopes;
204s assert an empty body. Auth middleware is tested through real components
over mocked stores. WS binder tests stub `OnFirstSubFunc`/`OnLastUnsubFunc`
to invoke immediately, fire the captured callback, assert
`PublishManyCalls()`.

**External HTTP.** Never activate httpmock globally: `client, mt :=
testutil.MockHTTP()`, responders per test (`Resp httpmock.Responder` as a
table field), the client handed to the SUT through its `HTTPClient`
dependency.

**Never sleepy.** No `time.After` polling: drain supervisors
(`supv.Wait()`/`CloseAndWait()`), await a `stopCh` the goroutine closes, or
`cancel()` from inside a mock. `time.Sleep` is a last resort with a comment.
Logging is asserted, not mocked: `slog.NewTextHandler` over a `bytes.Buffer`
(`testutil.NewBuffer()` for concurrent paths); `slog.DiscardHandler` where
logs don't matter. Use `context.Background()`, not `t.Context()`.

**Hygiene.** Golden files in `testdata/`. Package-level fixtures are
`_`-prefixed. `t.Helper()` in named helpers. White-box construction is
normal. Giant test functions are fine; prefer one exhaustive function over
splitting a target's cases.
