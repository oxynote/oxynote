# AGENTS.md

Guidance for `docker/prod/`, the production all-in-one image. Shared
principles and TS/JS style live in the root [AGENTS.md](../../AGENTS.md).

## What this is

One public image (`ghcr.io/oxynote/oxynote`) running Caddy, web (Nuxt SSR),
core and auth-realtime under `launcher/` (`@oxynote/launcher`, pnpm), a
TypeScript supervisor run by node under tini. Postgres is the only required
external service; valkey, an S3-compatible store, meilisearch, SMTP and
changedetection.io are optional (without a store, objects live on the data
volume). `docker-compose.example.yaml` is the canonical deployment and
`README.md` the operator reference. `docker-compose.local.yaml` is the
override `make prod-run` layers on it to build the image locally and add a
mailpit, since signup needs a verified address.

- **`make prod-build`** (repo root) builds it. Core's binary must come from
  goreleaser, never `go build`; the target stages it into `.build/` for the
  Dockerfile's COPY, so a bare `docker build` fails on that COPY by design.
- **`make e2e-prod`** is the only thing that exercises the image as an
  operator installs it, and CI runs it only as a release gate. Run it
  locally for any change to the Dockerfile, the Caddyfile or the launcher.
  See [e2e/AGENTS.md](../../e2e/AGENTS.md).
- **`make prod-publish`** is the release build, run only by
  [release.yml](../../.github/workflows/release.yml) on a tag. It uses
  `goreleaser build` (not `release`, which would also publish
  `ghcr.io/oxynote/core`), so core reports the tag as its version and
  `production` as its environment, and pushes exactly two tags: `latest`
  and the bare semver (`v1.2.3` → `:1.2.3`).

## Invariants

- **The trust boundary is the loopback bind.** Core's `/api/x/*` and
  auth-realtime's `/api/internal/*` have no auth. Inside the container core
  (8180), auth-realtime (8181) and web (3000) bind `127.0.0.1`; only caddy
  listens on the wildcard (8080). Ports are constants in
  `launcher/src/mapping.ts`, the internal address variables are rejected by
  the unknown-variable guard, and the supervisor dials the container's own
  external addresses at boot and refuses to serve if anything answers.
  [e2e/tests/prod-trust-boundary.test.ts](../../e2e/tests/prod-trust-boundary.test.ts)
  asserts the same from outside. Changing any of this is a security
  decision.
- **`Caddyfile` is a sibling of [docker/Caddyfile](../Caddyfile)**: same
  routes, same 403 blocks on `/core/api/x/*` and
  `/auth-realtime/api/internal/*`; only the upstreams differ. Change both
  together.
- **The public namespace is minimal and vendor-neutral.** Operators see only
  the flat `OXYNOTE_*` variables declared in `launcher/src/env.ts`. Derive
  from shared values (one `OXYNOTE_PUBLIC_URL`, DSNs carrying credentials)
  instead of adding variables, generate secrets instead of exposing them,
  and never leak an internal dependency's name into a public variable. An
  undeclared `OXYNOTE_*` variable is a boot error.
- **Secrets precedence** (`launcher/src/secrets.ts`): explicit env (never
  persisted) → existing volume file → generate and persist 0600. The
  data-source encryption key cannot be rotated; never weaken this path.
- **Sentry DSNs are build-time only** (esbuild `--define` from Dockerfile
  ARGs); `OXYNOTE_CRASH_REPORTING_DISABLED` is the only runtime switch. The
  launcher keeps its own DSN (`bakedLauncherSentryDsn`) to report a child
  killed outright or a boot that fails before any child exists, and reads
  the disable switch from `process.env` because an invalid environment is
  one of the failures worth reporting.
- **Crash policy: die, don't restart.** Any child exit tears the rest down
  and exits nonzero; restarts belong to the container runtime.
- **Every emitted line is a JSON record with a `service` field**
  (`launcher`, `core`, `auth-realtime`, `web`, `caddy`). `childRecord`
  (`launcher/src/logging.ts`) adds `service` to the child's own object and
  changes nothing else; a non-JSON line becomes `{msg, service}`. The
  launcher's own events go through pino at a fixed info level
  (`OXYNOTE_LOG_LEVEL` governs the services only). Each stream has its own
  line splitter; `mute` drops output a child cannot switch off (nitro's
  listen banner).
- **Supply chain**: every `FROM` is pinned by digest; caddy and tini are
  downloaded at pinned versions and checksum-verified. Caddy is Apache-2.0
  (LICENSE ships at `/oxynote/licenses/caddy/`) and trademarked: bundle it
  unmodified and unrebranded.

## launcher/

Follows [server/auth-realtime](../../server/auth-realtime/AGENTS.md)'s
conventions: tabs-at-8 prettier, no semicolons, explicit `.js` import
extensions, type-aware eslint, knip, vitest with concurrent tests and the
context-local `expect`. `src/index.ts` is the only module with side effects
and is excluded from coverage; everything else is a factory taking its
dependencies and is fully tested.

```sh
cd docker/prod/launcher
pnpm run qa           # check-lint + test
pnpm run build:bundle # esbuild -> dist/launcher.mjs; SENTRY_*_DSN env become baked defines
```

The runtime is pinned alpine with alpine's nodejs package plus
`icu-data-full` (so `Intl` formats every locale during SSR). The image ships
no node_modules: auth-realtime and the launcher are single esbuild bundles
with sentry folded in, and web is nitro's minified self-contained output
(`NITRO_MINIFY=true`). The web builder fails if a `*.node` file lands in the
output.

**Both bundles carry a `createRequire` banner.** Sentry's opentelemetry
dependencies `require()` node builtins at runtime and an `--format=esm`
bundle has no `require` to give them; without the banner the container
fails at startup with `Dynamic require of "util" is not supported`, and no
unit test catches it because the bundle is only built in the image.
