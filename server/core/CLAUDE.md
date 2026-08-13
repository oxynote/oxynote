# CLAUDE.md — Go Engineering & Testing Standards

These conventions are mandatory for all Go code in `server/core` — production and
test alike. When surrounding code conflicts with this file, this file wins for new
and modified code.

Some rules reference small shared utility packages (`errutil`, `sqlutil`,
`timeutil`, `logutil`, `ioutil`, `testutil`, `metricutil`, `httpserver`, and
friends). They live in this repo under `pkg/`; supervisors come from
`github.com/jellydator/xync`. The in-repo implementations are the source of truth
for their contracts. A helper referenced here but not ported yet (`syncutil`'s
context-aware mutex, `mathutil`) gets ported before its first use — the contracts
are part of the convention.

## Non-negotiables (summary)

1. Every possible code path — success and failure — is unit tested. Untestable
   branches carry a `// NOCOV: <reason>.` comment.
2. Every external dependency of a unit is an interface **declared by the consumer**
   and mocked (via `moq`) in tests. Only the database layer tests hit a real database.
3. One test function per production function/method, named `Test_<Type>_<Method>`.
4. Table-driven tests use `map[string]struct{...}` tables named `cc`, run with
   `t.Run(cn, ...)` + `t.Parallel()`.
5. The database is accessed through the **agent pattern**: consumers own narrow
   `DB`/`DBTx`/`DBAgent` interfaces and run transactions without ever seeing
   `*sql.Tx`/`*sqlx.Tx`.
6. Doc comment on every declaration — exported *and* unexported — ending with a
   period. Interface method docs say what the method *should* do.
7. Package-level variables/constants that are unexported are prefixed with `_`.
   Exported globals are allowed only for `Err*` sentinels.
8. `timeutil.Now()` instead of `time.Now()`. `decimal.Decimal` for money/market
   values. `any` instead of `interface{}`.
9. Goroutines are owned by a supervisor and every package's tests run under
   `goleak.VerifyTestMain`.
10. Lint clean under a strict golangci-lint profile (gofumpt extra rules, wsl, godot,
    revive all-rules, gocritic all tags). `//nolint` always carries a reason.

## Repository layout

- `cmd/<app>/main.go` — the only place that reads the environment, builds the root
  options struct, and wires everything. `log.Fatal()` is allowed only here.
- `internal/` — all business logic. Deep nesting is encouraged and mirrors the domain
  tree. Use a nested `internal/` (e.g. `internal/remote/internal/...`) to hard-lock
  sub-packages (like HTTP handlers) to their parent subtree.
- `pkg/` — the shared utility packages listed above plus tiny, dependency-light,
  domain-agnostic helpers (`errcode`, `ptrutil`, `sliceutil`-style). Business
  logic never goes here.
- Package names: short, lowercase, no underscores. Helper siblings are named
  `<parentabbrev><role>` (`stratutil`, `exchmarket`, `tenantpool`) or a bare role
  name when nesting disambiguates (`intercept`, `execution`, `ratelimit`).
- Files: snake_case, named after the primary type/concern. Recurring names across
  packages: `manager.go`, `options.go`, `metrics.go`, `client.go`, `event.go`,
  `util.go`, `error.go`, `http.go`, `ws.go`.
- Generated mocks: in-package `0mock_<name>_test.go` files (the `0` sorts them to
  the top of the directory) and exported `_mock/` sibling directories (package
  `mock`; the `_` prefix hides them from normal builds).
- SQL migrations live in `<dbpkg>/migrations/NNN_snake_case_description.sql`
  (zero-padded, sequential) and are embedded with `//go:embed migrations`.
- Package doc comment (`// Package x <verb phrase>.`) on the primary file of each
  package.

## Style mechanics

### Comments

- godot is enforced: **every comment ends with a period.**
- Doc comments on everything, including unexported types, funcs, methods, fields,
  constants, and vars. Format: `// <Name> <indicative present verb phrase>.`
  Constructors: `// NewX creates a fresh instance of X.`
- Interface methods use contract language: `// FetchUser should retrieve a user by ID.`
  Implementations use indicative: `// FetchUser retrieves a user by ID.`
- `// NOCOV: <lowercase reason ending with a period.>` marks an intentionally
  uncovered branch. Place it as the first line inside the branch body. Use it only
  when the branch genuinely cannot be reproduced in tests (constant inputs already
  validated, OS-level failures, "error case cannot be simulated in tests."). NOCOV is
  a reviewer covenant, not a tool directive — it never excuses skipping a
  testable path.
- `// NOTE: <why something non-obvious is the way it is.>` for design rationale.
- `//nolint:<linter>[,<linter>] // <lowercase reason>` — reason is mandatory
  (nolintlint enforces it). Recurring accepted reasons:
  `//nolint:errcheck // error provides no meaningful info` (deferred Rollback/Close),
  `//nolint:gocognit // this method is complex, however, it's well-structured`,
  `//nolint:forcetypeassert // the type is static`,
  `//nolint:gochecknoglobals // used as a constant`.
- Never write comments that narrate the next line or justify a diff; comments state
  constraints and reasons the code cannot express.
- Comments stand alone: don't reference design docs by section number ("§5.4",
  "M1") or assume the reader has another file open.

### Globals

- `gochecknoglobals` is enabled with a single lint exclusion for names matching
  `^_.+` — therefore every unexported package-level `var`/`const` is `_`-prefixed
  and doc-commented: `_updateInterval`, `_maxRetries`, `_typeWebhook`, `_ctxKeyUserID`.
- Use `var` (not `const`) for values tests need to shorten (timeouts, intervals).
- Exported globals: only `Err*` sentinels. Anything else needs
  `//nolint:gochecknoglobals // used as a constant`.
- `init()` is banned (`gochecknoinits`) except for build-info parsing, with a nolint.
- Compile-time interface assertions (`var _ Iface = &Impl{}`) are rare: because
  interfaces are consumer-declared, the assertion forces the implementing package
  to import its consumer and can create an import cycle. Never add one on your
  own initiative — only when a human explicitly asks for it (placed directly
  above the implementing type).

### Naming

- Receivers: 1–3 lowercase letters from the type's initials (`m` Manager, `t` Tenant,
  `el` EventLogger, `so` SequenceOperator), consistent across all methods of a type.
- Short local vocabulary is idiomatic: `cfg`, `opts`, `inp` (input), `rep` (report),
  `exec`, `supv` (supervisor), `res`, `evt`, `tnt`. Secondary error variables get a
  prefix letter: `cerr` (close), `rerr` (recover/retry), `perr` (parse).
- Acronyms stay uppercase everywhere, including at the start of unexported
  identifiers' interior: `httpClient`, `BaseHTTPURL`, `individualETFChecker`, `ID`,
  `DSN`, `TTL`. Exception: `Ws` in compounds (`BaseWsURL`, `setupWs`).
- `any`, never `interface{}`.
- Unused parameters are `_`.
- When restructuring, audit every exported identifier with `grep -rn "pkg\.Name"`.
  Anything used only inside its own package becomes lowercase (functions) or
  `_camelCase` (constants/vars). Doubly-unused things get deleted.

### Function shape & whitespace (wsl rhythm)

- Guard clauses and early returns; no `else` after a returning `if`.
- `if err := f(); err != nil {` when `err` is not reused; otherwise assign then check.
- Blank line before every `return` (unless it's the only statement), before every
  block that starts a new logical step, and after a `Lock/defer Unlock` pair:

  ```go
  t.mu.Lock()
  defer t.mu.Unlock()

  // work...
  ```

- Multi-argument constructors and calls are exploded one parameter per line.
- Named returns exist for exactly one purpose: letting a `defer` observe/replace the
  result to make a function panic-safe (pre-seed a default error result, then defer
  the finalizer). Naked returns are effectively banned.
- `defer cancel()` immediately after `context.With*`; `defer mu.Unlock()` immediately
  after `Lock()`; `defer resp.Body.Close() //nolint:errcheck // ...` after HTTP calls.
- Import grouping: one stdlib block, then one block of everything else sorted by
  path. Alias collisions with lowercase concatenations
  (`marketExchange "example.com/lib/market/exchange"`) and mock imports as
  `<pkg>Mock` (see Testing).

## API design

### Interfaces — always consumer-side

- **A package declares an interface for every dependency it consumes**, listing only
  the methods it calls. Providers satisfy them structurally; consumers never import
  the provider just for its type.
- Interfaces are small (1–4 methods). Bigger ones exist only as facades composed by
  embedding smaller interfaces — never by re-listing methods:

  ```go
  // DB is an interface that contains all database interfaces
  // required by the engine.
  //
  //go:generate ../../scripts/codegen/mock -t internal DB db
  type DB interface {
      eventlog.DB
      statistics.DB
      strategy.DB
  }
  ```

- Every interface method has a `should`-phrased doc comment.
- The `//go:generate` mock directive lives inside the interface's doc comment block,
  after the prose, separated by a bare `//` line (see Mock generation).
- Interfaces are declared **at the bottom of the file that uses them**.
- Naming: `-er` agent nouns where natural (`EventPublisher`, `SafeExecutor`,
  `Accessor`), otherwise role nouns (`DB`, `DBTx`, `Components`, `Input`, `Cache`).

### Constructors

- `New` when the type is the package namesake, otherwise `New<Type>`. Unexported
  types get `new<Type>`.
- Parameter order: `ctx` (only if the constructor performs I/O) → `logger` →
  dependencies → `opts Options` **last**. Return a pointer; return `(T, error)` only
  if construction can fail.
- First statement: `if err := opts.validate(); err != nil { return nil, err }`
  when Options exist.
- Complex wiring uses construct-then-mutate: build the literal, assign nested
  maps/fields, start background work, return.
- Multi-stage constructors accumulate cleanup:

  ```go
  var closers []io.Closer

  sub, err := newSubsystem(...)
  if err != nil {
      return nil, ioutil.AppendCloseErr(ioutil.MultiCloser(true, closers...), err)
  }

  closers = append([]io.Closer{sub}, closers...) // prepend: close in reverse order
  ```

  Adapt non-standard cleanups with `ioutil.CloserFunc`. Every resource-owning type
  has `Close() error`.

### Options — plain structs, never functional options

- One `Options` struct per configurable package, named exactly `Options`, defined
  next to its constructor (own `options.go` file when large).
- Every field carries a doc comment stating semantics and units; zero-value behavior
  is documented (`// Zero value disables X.`).
- Options **nest to mirror the dependency tree**: the root package's Options embeds
  child packages' Options, and `Validate()`/`validate()` cascades down. Unexported
  `validate()` when only the local constructor calls it; exported when a parent
  cascades into it.
- Config comes from environment variables only, read once in `main`, prefixed with
  the uppercased app name (`<APP>_DB_DSN`), and assembled into the root Options
  literal. No config files, no env access outside `main`/buildinfo.

### Structs — two genres

**Service/manager structs** (unexported fields, constructor-built):

```go
type Manager struct {
    log *slog.Logger     // always first
    db  DB               // dependencies next

    mu     sync.RWMutex  // a mutex sits directly above the fields it guards,
    active map[xid.ID]*item // separated from other fields by a blank line

    events struct {       // anonymous nested structs namespace related state;
        update struct {   // the mutex is the first field of the struct it guards
            mu  sync.RWMutex
            fns map[uint64]func(context.Context, Item)
        }
    }

    supv *xync.Supervisor
    opts Options          // last
}
```

**Data/DTO structs** (exported fields): blank line between fields, doc comment on
every field (`// X specifies ...`; `// X indicates ...` for booleans), tags
`json:"camelCase" db:"snake_case"`, foreign keys `db:"fk_<table>_id"`.

- Embedded fields come first, with no blank line between them, before named fields.
- Embedding is the mixin mechanism: tiny stateless structs exist solely to satisfy
  an interface method so implementations don't repeat it
  (`PlainBinder`, `SubscriptionPro`, `AccessLevelNormal`-style traits). Document
  them as such: `// PlainX implements the X interface to avoid Y re-implementation.`

## Errors

Two tiers, kept in the same `var (...)` block when convenient:

- **User-facing** errors are built with `errutil.New(httpStatus, errcode.Constant,
  "lowercase message, no punctuation")`. The `errutil` contract: an error type
  carrying an HTTP status code, a machine-readable internal code, and a message;
  helpers `errutil.Wrap(err, status, code, msg)`, `errutil.NewWithData(status, code,
  map[string]any{...}, msg)`, `errutil.StatusCode(err, ...)`,
  `errutil.InternalCode(err, ...)`, sentinel `errutil.ErrNotFound` (404).
- **Internal** errors are plain `errors.New("lowercase message")`.

Rules:

- Machine-readable codes live in a central `pkg/errcode` package: untyped string
  constants, dotted-namespace values (`"config.duplicate_name"`,
  `"capabilities.webhook_limit_exceeded"`), grouped in per-domain `const` blocks,
  each doc-commented.
- Sentinels are package-level `Err*` vars, doc-commented
  `// ErrX is returned when ...`.
- Sentinel handling uses a **single outer `if err != nil`** with the sentinel
  branch nested inside — never two sibling ifs; `errors.Is` must not run on the
  success path:

  ```go
  existing, err := store.FetchX(ctx, id)
  if err != nil {
      if errors.Is(err, errutil.ErrNotFound) {
          return nil
      }

      return fmt.Errorf("fetching existing: %w", err)
  }
  ```

- Don't discard `json.Unmarshal` errors. If a parse failure is genuinely
  best-effort (e.g. building a label), log a Warn with context and continue with
  the zero-valued target; never `_ = json.Unmarshal(...)`.
- Capability/permission checks are methods returning `error`
  (`AllowsX() error`), building the user error inline at the check site.
- `fmt.Errorf("%w")` wrapping is the default when passing errors up the stack —
  add context at every hop that has any to give, using gerund phrases:
  `fmt.Errorf("fetching portfolio: %w", err)`. Return bare only when the caller
  would add nothing beyond what the callee already said.
- At trust boundaries (background reports, job results), classify errors once in a
  dedicated `error.go`: 4xx user errors pass through with their code; 5xx/unknown
  errors are reported (Sentry/critical log) and replaced with an opaque
  `ErrInternal` so internals never leak.
- **No `panic` in production code.** Goroutine panics are contained by supervisor
  recovery plans (`logutil.Recover`/recovery options). The single allowed panic
  category is programmer-error guards in reflection-based plumbing
  (`panic("dest is not a pointer")`).
- Multierror only for parallel fan-out (`multierror.Group`) and close paths.

## Logging (slog)

- Inject `log *slog.Logger`, first field of the struct, first constructor
  parameter (named `log` in both).
- Child loggers are created at wiring time, not in methods:
  `log.With("component", "notifications-manager")`, or
  `log.With(slog.String("org_id", orgID))` for per-entity children.
- Event style — message first, then typed `slog.Attr`s, one per line when ≥2
  fields:

  ```go
  m.log.Error(
      "cannot update item",
      slog.String("error", err.Error()),
      slog.String("item_id", id.String()),
  )
  ```

- Messages: lowercase, no trailing period, no interpolated errors. Prefer
  `"cannot <verb> <object>"`; `"failed to <verb>"` is acceptable.
- Field keys snake_case; typed constructors (`slog.String`, `slog.Int`), never
  bare key-value pairs in call sites with ≥2 fields. Errors are always
  `slog.String("error", err.Error())`.
- `logutil.Critical(m.log, err).Error("cannot ...")` (reports to Sentry, a level
  above Error) for anything a human must act on.
- Levels: Error/Critical = actionable failures; Warn = malformed external input
  that is skipped; Info = lifecycle only ("ignited", "shut down"); Debug = teardown
  details; fatal exits (`log.Fatal`/`os.Exit`) = `main` only.
- **Log or return, never both.** A function returning an error does not also log
  it. When a failure is deliberately swallowed (best-effort cleanup), log it and
  comment why the error is not returned.

## Concurrency

- **Every background goroutine is owned by a supervisor** (`xync.Supervisor`
  contract: `Go(func(ctx context.Context))`, `Wait()`, `Close()`, `CloseAndWait()`,
  options for recovery and max-active). Raw `go` statements are reserved for
  starting a periodic executor or in `main`. `sync.WaitGroup` is never used
  directly.
- Mutexes: `sync.Mutex`/`RWMutex` for pure in-memory state, named `mu` (single
  concern) or `<thing>Mu` (several). For operations a caller may abandon, use a
  context-aware mutex (`syncutil.Mutex` contract, not ported yet — port before
  first use: `Lock(<-chan struct{}) bool` returns true when interrupted):

  ```go
  if m.mu.Lock(ctx.Done()) {
      return ctx.Err()
  }

  defer m.mu.Unlock()
  ```

- Lock-then-defer-unlock is the invariant; manual early unlock only when the
  critical section must end mid-function.
- **Callback registries return an unsubscribe closure.** IDs come from an
  `atomic.Uint64` (or a counter under the same mutex); dispatch reads the registry
  under `RLock` and fans out through the supervisor — never invoke callbacks
  synchronously while holding the write lock.

  ```go
  func (t *Thing) OnUpdate(fn func(context.Context, Item)) func() {
      t.fns.mu.Lock()
      defer t.fns.mu.Unlock()

      id := t.fns.id.Add(1)
      t.fns.m[id] = fn

      return func() {
          t.fns.mu.Lock()
          defer t.fns.mu.Unlock()

          delete(t.fns.m, id)
      }
  }
  ```

- Periodic work: a periodic executor (`timeutil.NewPeriodicExec(interval, offset,
  fn, recoveryValue, immediate)`) or cron with 6-field specs; never hand-rolled
  ticker loops.
- Shutdown coordination: a `chan struct{}` closed once, exposed as
  `ShutDownCh() <-chan struct{}`; loops `select` on it.
- Ctx-bound sleeps use `select { case <-ctx.Done(): ...; case <-time.After(d): }`,
  never bare `time.Sleep`.
- Retries: exponential backoff (`cenkalti/backoff`) wrapped with context and a max
  retry count; a retry failure that is not `context.Canceled` is logged critical.
- Bounded parallel work: `errgroup`/`multierror.Group` with an extra mutex for
  shared map writes.

## Time, IDs, numbers

- `timeutil.Now()` (a mockable `time.Now` wrapper) — **never** `time.Now()` in
  production code. Stdlib is fine for `time.Since`, timers, and arithmetic.
- Durations are named `_`-prefixed package constants/vars with doc comments, never
  magic values at call sites.
- IDs: `rs/xid` (`xid.New()`, `xid.ID` fields, `.IsZero()`/`.IsNil()` checks).
- Money and market values: `shopspring/decimal`, never floats. Zero is
  `decimal.Zero`. All non-trivial decimal math lives in `pkg/mathutil` (not
  ported yet — port before first use: `SafeDiv` guarding division by zero,
  `PercentChange`, `CmpDecimal` for validator plumbing, shared `Hundred`); call
  sites never inline formulas.
- Nullable **database** columns: `github.com/guregu/null/v5` (`null.Time`,
  `null.String`), set via `null.TimeFrom(...)`, cleared with the zero literal.
  Prefer nullable columns + null types over `NOT NULL DEFAULT ''` + empty-string
  sentinels for genuinely optional fields. Optional config values use plain
  types + `omitempty` or pointers when nil is meaningful — not null types.

## Enums & serialization

- Typed string enums by default. The `const` block comes **before** the type
  declaration; values are kebab-case; every variant doc-commented (or one header
  comment — `// All available X constants.` — when self-evident):

  ```go
  const (
      // StatusActive specifies the active status.
      StatusActive Status = "active"

      // StatusInactive specifies the inactive status.
      StatusInactive Status = "inactive"
  )

  // Status specifies the state of an item.
  type Status string
  ```

- `iota` only for context keys (`type ctxKey int` + `_ctxKeyX ctxKey = iota`),
  bitmasks, and `iota + 1` when zero must mean "unset".
- Enum methods: `Validate() error` (switch over variants, returning a package
  `ErrInvalidX` sentinel) — this makes the enum usable directly as a validation
  rule — plus `MarshalText`/`UnmarshalText` (not MarshalJSON) with input
  normalization (trim + lowercase) in `UnmarshalText`.
- Every JSON-in-SQL type implements `driver.Valuer`/`sql.Scanner` with this exact
  template (copy it; deviating is wrong):

  ```go
  // Value transforms the metadata type into a database entry.
  func (md Metadata) Value() (driver.Value, error) {
      // NOCOV: error case cannot happen since the data
      // is already validated.
      return json.Marshal(md)
  }

  // Scan transforms a database entry into a metadata type.
  func (md *Metadata) Scan(src any) error {
      var pv []byte

      switch v := src.(type) {
      case []byte:
          pv = v
      case string:
          pv = []byte(v)
      default:
          return errors.New("invalid metadata type")
      }

      data := &Metadata{}
      if err := json.Unmarshal(pv, data); err != nil {
          return err
      }

      *md = *data

      return nil
  }
  ```

- Polymorphic JSON uses the `type sub X` alias trick plus a `_type<Name>` string
  discriminator constant:

  ```go
  // MarshalJSON converts the step into raw JSON.
  func (w webhook) MarshalJSON() ([]byte, error) {
      type sub webhook

      return json.Marshal(struct {
          Type string `json:"type"`
          sub
      }{Type: _typeWebhook, sub: sub(w)})
  }
  ```

  `UnmarshalJSON` unmarshals into `sub`, converts back, then runs post-decode
  initialization.

## Validation

- User-input domain types implement `ValidateWithContext(ctx) error` using
  `jellydator/validation` (ozzo-style): value receiver, pass `&v`, one
  `validation.ValidateStructWithContext(ctx, &v, validation.Field(...)...)` return,
  fields in declaration order, embedded configs validated first. Doc comment is
  literally `// ValidateWithContext checks whether the options are valid.`
- Decimal bounds plug in via `validation.Min(decimal.Zero).CmpFunc(mathutil.CmpDecimal)`.
- Options structs use hand-rolled `validate()` with `errors.New` instead of ozzo.
- Aggregate mutations go through `Set*` methods returning `(changed bool, err error)`
  with the doc note `// The first return value determines whether X was set or not.`;
  an `ApplyInput(inp)` method orchestrates them and returns a summary struct of
  booleans.

## Database layer — the agent pattern

The whole point: **consumers get transactional and non-transactional access through
their own interfaces, and `*sql.Tx`/`*sqlx.Tx` never leaks out of the db package.**

### Core (implement once per project, in `internal/db`)

```go
// agent holds everything needed to run queries in both
// transaction and non-transaction modes.
type agent struct {
    sql     sqlx.ExtContext            // satisfied by *sqlx.DB and *sqlx.Tx
    builder sq.StatementBuilderType    // squirrel, dollar placeholders
    opts    Options
}

// DB contains dependencies needed for direct communication
// with the database.
type DB struct {
    agent

    log *slog.Logger
    sql *sqlx.DB
    // migrations, metrics factory, closer...
}

// Tx contains dependencies needed for direct communication
// with the database in a transaction mode.
type Tx struct {
    agent

    sql *sqlx.Tx
}

// Commit commits a transaction.
func (tx *Tx) Commit() error { return tx.sql.Commit() }

// Rollback rollbacks a transaction.
func (tx *Tx) Rollback() error { return tx.sql.Rollback() }

// BeginTx begins a transaction and tries to apply its object to the one
// given in dest parameter.
func (db *DB) BeginTx(ctx context.Context, dest any) error {
    value := reflect.ValueOf(dest)
    if value.Kind() != reflect.Ptr || value.IsNil() {
        panic("dest is not a pointer")
    }

    tx, err := db.sql.BeginTxx(ctx, nil)
    if err != nil {
        return err
    }

    defer func() {
        if rerr := recover(); rerr != nil {
            tx.Rollback() //nolint:errcheck // rollback errors provide no meaningful info
            panic(rerr)
        }
    }()

    value.Elem().Set(reflect.ValueOf(&Tx{
        agent: agent{sql: tx, builder: db.builder, opts: db.opts},
        sql:   tx,
    }))

    return nil
}
```

**Every query method is defined on `*agent`** (`func (a *agent) CreateItem(...)`), so
it is automatically available on both `*DB` and `*Tx`.

The shared contracts (in `sqlutil` or equivalent):

```go
// DB is implemented by all database managers.
type DB interface {
    // BeginTx should begin a transaction and set the active Tx
    // object to the dest parameter.
    BeginTx(ctx context.Context, dest any) error
}

// Tx is implemented by all transactions.
type Tx interface {
    Commit() error
    Rollback() error
}
```

`sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error)` runs a multi-statement write
atomically from inside an agent method: it opens+commits a transaction when given a
`*sqlx.DB` and transparently reuses the transaction when the agent is already
running in one.

### Consumer side

Each consumer package declares a triple at the bottom of its main file:

```go
// DBTx is an interface that handles communication with the item
// database in a transaction.
//
//go:generate ../../scripts/codegen/mock -t internal DBTx db_tx
type DBTx interface {
    sqlutil.Tx
    subpkg.DBTx // compose child packages' transaction needs
    DBAgent
}

// DB is an interface that handles communication with the item database.
//
//go:generate ../../scripts/codegen/mock -t both DB db
type DB interface {
    sqlutil.DB
    subpkg.DB
    DBAgent
}

// DBAgent is an interface that handles communication with the item database
// in both modes.
type DBAgent interface {
    // CreateItem should insert a new item into the database.
    CreateItem(ctx context.Context, it *Item) error

    // FetchAllItems should retrieve all items by the owner ID.
    FetchAllItems(ctx context.Context, ownerID xid.ID) ([]*Item, error)
}
```

`DBAgent` holds the shared methods; `DB` adds `BeginTx`; `DBTx` adds
`Commit`/`Rollback`. Parent packages compose child packages' `DB`/`DBTx` interfaces
by embedding, so a single concrete `*db.DB` satisfies the entire tree. Leaf
consumers that never transact declare a flat `DB` without the `sqlutil.DB` embed.

### The transaction idiom

```go
var tx DBTx

err := m.db.BeginTx(ctx, &tx)
if err != nil {
    return err
}

defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

if err = tx.CreateItem(ctx, it); err != nil {
    return err
}

if err = tx.CreateAuditEvent(ctx, evt); err != nil {
    return err
}

err = tx.Commit()
if err != nil {
    return err
}

// mutate in-memory state only after a successful commit.
m.items[it.ID] = it
```

The deferred rollback after a successful commit is a harmless no-op. In-memory
caches/maps are updated strictly after `Commit()` succeeds.

### Entity files

One file per entity in the db package; **row structs live in the domain packages**,
the db package contains only queries. Section order within a file:

1. `Create*` (inserts)
2. `Fetch*` (single → many → counts)
3. `Update*` / `MarkX*`
4. `Delete*` / `Cleanup*`
5. `select<Entity>` — a lowercase builder helper that appends the full aliased
   column list, `FROM`, and the mandatory owner/tenant scope so it cannot be
   forgotten by any query.
6. `apply<Entity>Filter`, `apply<Entity>Sort` — whitelist free functions.

Method naming: `<Verb><Entity>[By<Key>]` (`FetchOrderByID`, `DeleteItemsByOwnerID`).

### Query idioms

- squirrel with `$` placeholders; every query ends with `.MustSql()` → hand the
  `(q, args)` to sqlx. Never `RunWith`, never `ToSql` + error handling.
- Reads: `sqlx.GetContext(ctx, a.sql, &dst, q, args...)` for one row/scalar,
  `sqlx.SelectContext` for many. Writes: `a.sql.ExecContext(ctx, q, args...)`.
- Open the pool with sqlx `Unsafe()` (extra columns like `page_count` must not
  error) and comment why.
- Conditions with `sq.Eq`/`sq.NotEq`/`sq.Gt`/`sq.And` maps; `SetMap` for updates;
  `sq.Expr` for raw fragments (`"count + 1"`); subqueries by nesting a builder with
  `Prefix("x NOT IN (")`/`Suffix(")")`.
- NULL checks compare against Go nil: `sq.Eq{"deleted_at": nil}`.
- **Pagination**: parse the HTTP query into a `Query{Limit, Page, Filters, Sorts}`
  value; a shared `sqlutil.Select(qr, applyFilter, applySort, sqlutil.PageCount)`
  applies limit/offset and calls the whitelists (invoking them at least once with
  zero values so defaults apply); the page count rides along as a window-function
  column and is scanned via an anonymous struct embedding the entity pointer:

  ```go
  var data []struct {
      *Item
      PageCount uint64 `db:"page_count"`
  }
  ```

  Empty results return a non-nil empty slice and count 0. Always append a
  deterministic tiebreaker after the user sort (`b.OrderBy("items.id ASC")`).
- Filter/sort whitelists return typed 400 errors (`ErrInvalidFilterKey`,
  `ErrInvalidFilterValue`, `ErrInvalidSortKey`) for anything unknown; the `case "":`
  branch sets defaults then `fallthrough`s.
- Joins alias to the nested-struct prefix so dotted `db` tags scan directly:
  `` Join(`accounts AS "account" ON account.id = items.fk_account_id`) `` with
  columns like `items.started_at AS "report.started_at"`. Null-out empty embedded
  structs after scanning (`if it.Account.ID.IsNil() { it.Account = nil }`).
- IDs are generated in the domain layer (`xid.New()`) before insert — no
  `RETURNING`.
- Soft deletes are UPDATEs setting `deleted_at`; hard deletes happen in `Cleanup*`
  sweeps when nothing references the row. Retention trimming (keep last N per
  owner) happens inside the entity's `Create*` under `WrapTx`, driven by Options.
- Encrypted columns cross the boundary through `Raw*` twin structs with
  `To<Entity>`/`ToRaw<Entity>` converters taking the `Encrypt`/`Decrypt` function;
  the `Cryptor` interface (`Encrypt`/`Decrypt`) is declared by the db package.
- **Driver-level error mapping**: install an error hook at connection setup
  (sqlhooks) with a `DetectError(err) error` function that converts
  `sql.ErrNoRows` → `errutil.ErrNotFound` and known constraint violations
  (pg code `23505` + constraint name) → user errors. Callers then branch with
  `errors.Is(err, errutil.ErrNotFound)` and handlers forward errors untouched.
  The same hooks emit per-query metrics with a slow-query threshold.
- Migrations run automatically in `New()` from the embedded FS.

## HTTP & WebSocket layer

- One `server` package owns cross-cutting concerns (server, router, auth, metrics);
  **all handlers live in nested packages under `server/internal/...` mirroring the
  domain package tree**, each leaf being exactly `http.go` + `ws.go`.
- Handler packages do not register routes. The router file owns the entire URL tree
  centrally in a fixed set of `http<Area>Router() chi.Router` functions. Trust
  zones are explicit mounts: authenticated private API, public API, and an
  unauthenticated admin subtree expected to be protected upstream (reverse proxy) —
  commented as such.
- Middleware baseline: CORS (only when origins are configured), request timeout,
  recoverer, method-not-allowed/not-found handlers, then the metrics wrapper.
- Handler shape: a `Handler` struct holding only `log` + process-wide deps, with
  `NewHandler(log, deps...)`. Methods are **not** `http.HandlerFunc` — they take
  extra arguments for per-request/per-tenant services, wired by route-level
  closures: `func (h *Handler) HandleCreateItem(w http.ResponseWriter,
  r *http.Request, ownerID xid.ID, exec SafeExecutor)`.
- Handler method names are `Handle<DomainVerb><Entity>` — the domain verb, never
  the HTTP verb (the router file already says `GET`/`PUT`/`POST`):
  `HandleFetchX` for GET of a single resource, `HandleFetchXs` for a collection
  (not `HandleListX`), `HandleExtractX`/`HandleReextractX` for action endpoints
  (not `HandlePutX`), `HandleCreateX`/`HandleUpdateX`/`HandleDeleteX` for the
  obvious CRUD verbs.
- Identity is resolved once in middleware into a private context key
  (`type ctxKey int` + `_ctxKeyUserID`), then passed to handlers as a plain
  argument — handlers never read the context for it.
- Decoding: shared helpers only — `DecodeJSON(r, &v)` (collapses failures into one
  invalid-JSON user error), `DecodeForm` (gorilla/schema tags) for query params,
  `ParseQuery(r)` for list endpoints, `ExtractTargetID(r)` for chi `{id}` params
  (bad ID → 404).
- Responding: exactly two calls — `Respond(log, w, data, status, headers...)` and
  `RespondError(log, w, err)` — after every failure, respond and `return`.
  Mutations return 204 with nil body; creates return 201 + Location header filled
  by route middleware. List responses and decorated entities use inline anonymous
  structs (embed the entity pointer, add `PageCount`/extra fields).
- Error mapping is centralized in the respond helper: user errors pass through with
  status + code + message; ≥500 is logged critical and rewritten to a generic body.
  Handlers never build HTTP error payloads.
- Each handler package declares its own narrow `DB` interface at the bottom of
  `http.go`, composed from domain interfaces.
- `ws.go` contains only `Bind*` methods on the same `Handler`: each subscribes to a
  domain callback and republishes to a topic. Use lazy subscribe on first
  subscriber / unsubscribe on last, guarding the unsubscribe closure with a mutex.
  Payloads are inline anonymous structs; deletion events publish just the ID.
  Topics are registered centrally next to the router, named `<event>@<domain.path>`
  (`update@exchange.orders`), with per-topic metrics injected by wrapping the
  binder function.
- HTTP metrics label by chi `RoutePattern()` (bounded cardinality), method, and
  identity; measured around `next.ServeHTTP` so post-auth context values are
  available.

## Metrics

- A metrics factory (registry wrapper) is threaded through constructors as `fc` —
  never a global registry; `prometheus.NewRegistry()` is created once in `main`.
- Each instrumented package has `metrics.go` with an unexported `metrics` struct +
  `newMetrics(fc)`; small packages inline the struct on the manager.
- `_subsystem<Name>` constant per package; `_<x>Label` constants for label keys.
- Names snake_case; counters end `_total`; histograms carry their unit as a suffix
  (`_duration_seconds`); gauges unsuffixed. Stay consistent with the project's
  existing scheme.
- `Help` is a complete capitalized sentence ending with a period, conventionally
  `"Tracks the number of ..."`.
- Cross-package label constants are exported from a dedicated `<domain>metrics`
  package.

## Testing standards

Tests are first-class: they live in the same package as the code (white-box), cover
**every** success and failure path, and mock **every** external dependency. Only the
db package touches a real database.

Most of the suite hasn't been written yet — it is being added soon. Absent tests in
a package are not license to skip them: new and modified code follows these
standards from day one.

### Naming & layout

- One test function per production function/method. Do not merge; do not split.
- Test files pair 1:1 with source files: `expand.go` → `expand_test.go`, never a
  bundled `package_test.go`. When tests span two files, they live in the one
  that pairs with the *primary* file driving the test (round-trip tests land
  with the file that initiates the round-trip).
- `Test_<Type>_<Method>` for methods (`Test_Manager_SafeUpdate`), `Test_<Func>`
  for functions (`Test_New`, `Test_applyItemFilter`). Unexported names keep their
  casing: `Test_agent_CreateEvent`, `Test_webhook_Exec`,
  `Test_Tenant_maybeUpdateCapabilities`. The only `Test<X>` (no underscore) forms
  are `TestMain` and `TestIntegration_*`.
- Every package with tests has:

  ```go
  func TestMain(m *testing.M) {
      goleak.VerifyTestMain(m)
  }
  ```

  Add `goleak.IgnoreCurrent()`/`IgnoreTopFunction(...)` options only for known
  third-party leaks, in the package that needs them.
- Serialization-heavy types get the full round-trip suite:
  `Test_X_UnmarshalJSON`, `Test_X_MarshalJSON` (raw JSON literal + `assert.JSONEq`),
  and `Test_X_Value` / `Test_X_Scan`.

### Table-driven tests (map form)

The default shape for any function with more than one path:

```go
func Test_Manager_Create(t *testing.T) {
    cc := map[string]struct {
        DB     *DBMock
        Item   *Item
        Result *Item
        Err    error
    }{
        "Error returned by db.CreateItem": {
            DB: &DBMock{
                CreateItemFunc: func(_ context.Context, _ *Item) error {
                    return assert.AnError
                },
            },
            Item: stubItem(),
            Err:  assert.AnError,
        },
        "Successful creation": {
            DB:     &DBMock{},
            Item:   stubItem(),
            Result: stubItem(),
        },
    }

    for cn, c := range cc {
        t.Run(cn, func(t *testing.T) {
            t.Parallel()

            m := &Manager{log: slog.New(slog.DiscardHandler), db: c.DB}

            res, err := m.Create(context.Background(), c.Item)
            testutil.AssertEqualError(t, c.Err, err)

            if err != nil {
                return
            }

            assert.Equal(t, c.Result, res)
        })
    }
}
```

Rules:

- The table is a **map keyed by a human-readable case name**, named `cc`; the loop
  is `for cn, c := range cc`; there is no `name` field.
- Case names are sentence case, no punctuation: `"Successful creation"`,
  `"Malformed JSON"`, `"Not permitted"`, `"Context cancelled"`. Error paths name
  the failing collaborator verbatim: `"Error returned by db.CreateItem"`,
  `"Error returned by Tx.Commit"`.
- Case-struct fields are exported PascalCase: inputs/collaborators first,
  expectations (`Result`, `Err`, or `Checks`) last. Standard vocabulary: `DB`,
  `Tx`, `Inp`, `JSON`, `ID`, `Context` (nil → `context.Background()` default in
  the loop body), `Result`, `Err`, `Checks`, `RespCode`, `RespJSON`.
- `t.Parallel()` is the first statement of the subtest closure (~everywhere; omit
  only for genuinely shared mutable state). No `c := c` rebinding.
- When a case needs setup or must keep a mock reference for later assertions,
  hoist a `type tcase struct {...}` above the table and build cases with
  immediately-invoked closures:

  ```go
  cc := map[string]tcase{
      "Error returned by Tx.Commit": func() tcase {
          tx := stubTx(nil, assert.AnError)

          return tcase{
              DB:     stubDB(tx, nil),
              Tx:     tx,
              Checks: checks(hasError(assert.AnError), wasTxRollbackCalled(1)),
          }
      }(),
  }
  ```

- Simple single-path functions (constructors, getters) may use a linear test with
  comment-delimited phases instead of a table:

  ```go
  // error
  ...
  // success
  ...
  ```

- Constructor tests (`Test_New*`) exercise the error path, then `require.NotNil`,
  then assert **every** struct field.

### Error expectations — `assert.AnError` three-state protocol

The `Err error` field has exactly three states, asserted with
`testutil.AssertEqualError(t, c.Err, err)` (or `RequireEqualError`):

- `nil` (omitted) → asserts no error;
- `assert.AnError` → asserts *some* error occurred (the generic failure);
- a concrete sentinel/constructed error → asserts deep equality with exactly it.

Mocks made to fail return `assert.AnError`. After the error assertion, bail with
`if err != nil { return }`. Never use `assert.ErrorIs`/`errors.Is` in tests — the
equality protocol above replaces it.

The helper (port it if absent):

```go
// AssertEqualError checks if errors are equal or, if assert.AnError is
// expected, whether an error exists or not.
func AssertEqualError(t *testing.T, exp, err error) {
    t.Helper()

    if exp != nil {
        if exp == assert.AnError { //nolint:err113,errorlint // direct check is needed
            assert.Error(t, err)
            return
        }

        assert.Equal(t, exp, err)

        return
    }

    assert.NoError(t, err)
}
```

### require vs assert

- `require` when the test cannot meaningfully continue: setup errors, `require.Len`
  before indexing, `require.NotNil` before dereferencing, `require.NoError` on
  fixture creation.
- `assert` for terminal outcome checks (values, final errors, call params).

### Structural comparisons

- `testutil.AssertFilterEqual(t, exp, act, ignoreTypes...)` (go-cmp with exported
  unexported access + `IgnoreTypes`) to skip nondeterministic values wholesale:
  `testutil.AssertFilterEqual(t, &exp, &act, xid.ID{}, time.Time{})`.
- `assert.JSONEq` for JSON bodies and MarshalJSON output; regex
  (`assert.Regexp`) for loose matching on large payloads.
- `assert.WithinDuration(t, timeutil.Now(), rec.CreatedAt, time.Second)` for fresh
  timestamps; fixed `time.Date(2021, 1, 2, 0, 0, 0, 0, time.UTC)` literals for
  deterministic values; or assert against the object's own field to sidestep
  nondeterminism.
- Decimals compare as strings: `assert.Equal(t, exp.String(), res.String())`.
- `assert.Same` for pointer identity.

### Mock generation (moq)

A `scripts/codegen/mock` wrapper drives `matryer/moq`; directives sit in the
interface doc block:

```go
//go:generate ../../scripts/codegen/mock -t internal DBTx db_tx
//go:generate ../../scripts/codegen/mock -t both DB db
//go:generate ../../scripts/codegen/mock -t external EventPublisher
```

- `-t internal` → `moq -fmt goimports -out 0mock_<name>_test.go -stub . <Iface>` —
  an in-package, test-only mock named `<Iface>Mock` (`DBMock`, `DBTxMock`).
- `-t external` (default) → `moq -fmt goimports -out ./_mock/<name>.go -stub
  -pkg mock . "<Iface>:<Iface>"` — an importable mock package for *other*
  packages' tests, type keeps the bare interface name (`mock.DB`).
- `-t both` → both outputs. The optional second argument overrides the snake_case
  file name.
- **`-stub` matters**: unset `XxxFunc` fields return zero values instead of
  panicking, so `&DBMock{}` is a valid don't-care collaborator that still records
  calls.
- `_mock/` directories contain only generated code (plus, rarely, a tiny
  hand-written adapter helper). Mocks for third-party interfaces go in a shared
  `internal/_mock` package.
- Cross-package mock imports are always aliased `<pkg>Mock`:
  `eventlogMock "example.com/app/internal/eventlog/_mock"`.

### Using mocks

- Configure by struct literal with only the needed funcs; unused params are `_`:

  ```go
  db := &DBMock{
      FetchItemFunc: func(_ context.Context, _ xid.ID) (*Item, error) {
          return nil, assert.AnError
      },
  }
  ```

- Reassign `XxxFunc` mid-test to move from the error phase to the success phase.
- Factories for repeated shapes are local closures named `stub*`, parameterized by
  the errors each method should return:
  `stubTx := func(createErr, commitErr, rollbackErr error) *DBTxMock {...}`.
- Assert calls exclusively through the generated recorders:

  ```go
  ff := db.CreateItemCalls()
  require.Len(t, ff, 1)
  assert.NotNil(t, ff[0].Ctx)
  assert.Equal(t, id, ff[0].ID)
  ```

- Test `On*` subscription APIs by capturing the registered callback from the
  recorder and invoking it: `se.OnUpdateCalls()[0].Fn(context.Background(), item)`.

### The check/checks combinator (for multi-collaborator functions)

When a function touches several mocks, replace per-field expectations with a
`Checks []check` field. Define locally, per test function:

```go
type check func(*testing.T, *DBMock, *DBTxMock, error)

checks := func(cc ...check) []check { return cc }

hasError := func(exp error) check {
    return func(t *testing.T, _ *DBMock, _ *DBTxMock, err error) {
        testutil.AssertEqualError(t, exp, err)
    }
}

wasTxCommitCalled := func(count int) check {
    return func(t *testing.T, _ *DBMock, tx *DBTxMock, _ error) {
        require.Len(t, tx.CommitCalls(), count)
    }
}

wasDBCreateItemCalled := func(count int) check {
    return func(t *testing.T, db *DBMock, _ *DBTxMock, _ error) {
        ff := db.CreateItemCalls()
        require.Len(t, ff, count)

        if count == 0 {
            return
        }

        assert.NotNil(t, ff[0].Ctx)
        assert.NotNil(t, ff[0].It)
    }
}
```

Naming is strict: **`has*` asserts state/outputs, `was*Called` asserts call
counts/params.** Each case composes exactly the checks that matter — including
zero-count checks proving what must *not* have happened:

```go
Checks: checks(
    hasError(assert.AnError),
    wasDBBeginTxCalled(1),
    wasDBCreateItemCalled(0),
    wasTxCommitCalled(0),
    wasTxRollbackCalled(1),
),
```

The subtest epilogue runs them: `for _, ch := range c.Checks { ch(t, c.DB, c.Tx, err) }`.

### Testing transactions (BeginTx mock wiring)

The DB mock injects the Tx mock through the `dest any` pointer exactly the way the
real implementation does:

```go
stubDB := func(tx *DBTxMock, beginErr error) *DBMock {
    return &DBMock{
        BeginTxFunc: func(_ context.Context, dest any) error {
            if beginErr != nil {
                return beginErr
            }

            reflect.ValueOf(dest).
                Elem().
                Set(reflect.ValueOf(tx))

            return nil
        },
    }
}
```

Store the `*DBTxMock` on the case struct so `was*` checks can inspect
`CommitCalls()`/`RollbackCalls()` afterwards. Standard cases for every
transactional function: BeginTx error, each statement's error, Commit error, and
success — each asserting the exact commit/rollback counts.

### Database-layer tests (real database)

- The db package's `TestMain` starts one throwaway Postgres container (gnomock)
  per package run, exports its DSN into a `_`-prefixed package var, and still runs
  goleak (`goleak.IgnoreCurrent()` for the container client). These are unit
  tests — Docker is a unit-test dependency for the db package only.
- `prepTempDB(t)` gives every test/subtest **its own randomly named database**:
  connect to the admin DB, `CREATE DATABASE <random>`, run `New()` (which
  migrates), register `t.Cleanup(func() { db.Close() })`. This is what makes
  `t.Parallel()` safe against a single container.
- Fixtures are per-entity helpers with a uniform signature:

  ```go
  func prepItems(t *testing.T, db *DB, count int, fn func(int, *Item)) []*Item {
      t.Helper()
      // build records, let fn mutate the i-th one, insert via the
      // package's own builder, require.NoError on every step.
  }
  ```

- Because each case needs a fresh schema, db tables are maps of **case constructor
  functions**: `cc := map[string]func(*testing.T, *DB) tcase{...}` iterated as
  `for cn, cfn := range cc`, calling `db := prepTempDB(t); c := cfn(t, db)` inside
  the subtest.
- Round times before insertion (`.Round(time.Microsecond)`) so values survive the
  database round-trip; verify outcomes by re-querying through the production
  select builders and comparing with `AssertFilterEqual`; assert deleted rows with
  `sql.ErrNoRows`.

### HTTP handler tests

- Invoke handler methods **directly** — no router:

  ```go
  hdl := Handler{log: slog.New(slog.DiscardHandler), db: c.DB}

  req := httptest.NewRequest("PUT", "http://test.com/", strings.NewReader(c.Body))
  req = req.WithContext(testutil.AddChiCtx(req.Context(), "id", id.String()))
  rec := httptest.NewRecorder()

  hdl.HandleUpdateItem(rec, req, ownerID, c.Exec)

  assert.Equal(t, c.RespCode, rec.Code)
  assert.JSONEq(t, c.RespJSON, rec.Body.String())
  ```

- URL is the throwaway `"http://test.com/"`; empty bodies are `http.NoBody`; chi
  URL params are injected with an `AddChiCtx(ctx, key, value)` helper, never a real
  router; an `OmitID bool` case field drives the missing-param path.
- Expected bodies are verbatim JSON literals, including the error envelope
  (`{"code":"request.invalid_json","message":"invalid JSON body"}`); 204s assert
  `assert.Zero(t, rec.Body.Len(), rec.Body.String())`; Location headers via
  `assert.Regexp`.
- Query strings are built by encoding the request struct with a package-level
  gorilla/schema encoder (`_formEnc`), not by hand-concatenating strings.
- Auth middleware is tested through real components over mocked stores (a real
  session manager backed by a moq'd store), not by stubbing the middleware.
- WebSocket binder tests use a mocked topic: stub `OnFirstSubFunc`/`OnLastUnsubFunc`
  to invoke their callbacks immediately, fire the captured domain callback, then
  assert `PublishManyCalls()`. Reserve a real socket server + client dial helper
  for the one test that exercises the actual transport.

### External HTTP dependencies (httpmock)

Never activate httpmock globally. Use an isolated transport per test:

```go
client, mt := testutil.MockHTTP() // -> &http.Client{Transport: httpmock.NewMockTransport()}
mt.RegisterResponder(http.MethodPost, "https://test.test/hook", c.Resp)
```

The responder is a table field (`Resp httpmock.Responder`); request-validating
responders inspect headers/body inline and return
`httpmock.NewStringResponse(http.StatusBadRequest, "")` on mismatch. The client is
handed to the SUT through its `HTTPClient` dependency (mock func or field).

### Concurrency in tests — deterministic, never sleepy

- No `select` + `time.After` polling. Synchronize on real completion signals:
  - inject an `xync.Supervisor` and drain with `supv.Wait()` /
    `supv.CloseAndWait()` before asserting;
  - `stopCh := make(chan struct{})`, closed by the goroutine, awaited `<-stopCh`;
  - drain crons with `<-cron.Stop().Done()`;
  - break consumer loops by calling `cancel()` from *inside* a mock's func.
- `time.Sleep` is a last resort and always carries an explanatory comment.
- Logging is asserted, not mocked: `slog.New(slog.NewTextHandler(&buf, nil))`
  with a plain `bytes.Buffer` for synchronous paths, a mutex-guarded buffered
  writer (`testutil.NewBuffer()` + `Flush()`) for concurrent paths, then
  `assert.Contains(t, b.String(), "cannot save on completion")` /
  `assert.NotContains`. Use `slog.New(slog.DiscardHandler)` everywhere logs
  don't matter.
- Use `context.Background()` in tests (not `t.Context()`); build cancelled
  contexts inline in the table via an immediately-invoked closure.

### Integration & golden tests

- Files end `_integration_test.go` with `//go:build integration` on line 1;
  functions are `TestIntegration_<Name>`; run with
  `go test -run '^TestIntegration' -race --tags=integration` (a make target will
  gate this once the QA harness lands).
- Golden JSON round-trips guard serialization compatibility: `.golden` files in
  `testdata/`, processed by a helper that enumerates them, unmarshals into a fresh
  instance, re-marshals, and `assert.JSONEq`s against the file (subtest per file,
  failing if the directory is empty).

### Test hygiene

- Package-level test fixtures are `_`-prefixed (`_pgDSN`, `_stratJSON`, `_formEnc`).
- `t.Helper()` in named helper functions (`prep*`, `dial`); closures don't need it.
- White-box construction is normal: build the SUT as a struct literal with
  unexported fields when the constructor's side effects are unwanted.
- Time/ID randomness: `xid.New()` freely (only uniqueness matters); random unique
  names via a short-ID generator for parallel-safe resources.
- Giant test functions are acceptable and linted for
  (`cyclop`/`goconst`/`noctx`/`funlen` relaxed for `_test.go`); prefer one huge
  exhaustive test func over splitting a target function's cases across several.
