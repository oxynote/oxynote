# CLAUDE.md

Guidance for `docker/prod/` — the production all-in-one Docker image.
Shared working principles and the TS/JS comment/whitespace rules live in the
root [CLAUDE.md](../../CLAUDE.md).

## What this is

One public image (`ghcr.io/oxynote/oxynote`) containing Caddy, the web app
(Nuxt SSR), core, and auth-realtime, supervised by `launcher/`
(`@oxynote/launcher`, **pnpm**) — a TypeScript program run by node under
tini. Postgres is the only required external service; valkey, an
S3-compatible store, meilisearch, SMTP and changedetection.io are optional
and external too — without a store, objects are kept on the data volume;
`docker-compose.example.yaml` is the canonical deployment and `README.md`
the operator-facing reference. `docker-compose.local.yaml` is not part of
either: it is the override **`make prod-run`** layers on the example to
build the image and drive it from a browser here, adding the mailpit the
example only describes in a comment — signup requires a verified address, so
without a relay the verification link is never more than a log line.

Build with **`make prod-build`** from the repository root: core's
binary must come from goreleaser (never a plain `go build`), so the target
builds it first and stages it into `.build/` where the Dockerfile's COPY
expects it. A bare `docker build` fails on that COPY by design.

**`make e2e-prod`** runs the whole e2e suite against the built image, plus
the trust-boundary tests that only mean something here. It is the only thing
that exercises this image as an operator installs it — the dev stack shares
none of its assembly — so run it locally for a change to the Dockerfile, the
Caddyfile or the launcher. CI runs it only as a release gate, so a change
here gets no pre-merge signal from it. Details in
[e2e/CLAUDE.md](../../e2e/CLAUDE.md).

**`make prod-publish` is the release build**, run by
[release.yml](../../.github/workflows/release.yml) on a tag and by nothing
else. It differs from `prod-build` in two ways that matter: core's binary
comes from `goreleaser build` rather than a snapshot, so it reports the tag
as its version and `production` as its environment instead of `dev`; and the
result is pushed to `ghcr.io/oxynote/oxynote` as `latest` and the bare
semver — `v1.2.3` publishes `:1.2.3`. Those two tags are the only things the
registry gets. `goreleaser build` is deliberate: `goreleaser release` would
also publish `ghcr.io/oxynote/core` from the `dockers_v2` block.

## Invariants

- **The trust boundary is the loopback bind.** Core's `/api/x/*` and
  auth-realtime's `/api/internal/*` carry no authentication. Inside the
  container core (8180), auth-realtime (8181), and web (3000) bind
  `127.0.0.1`; only caddy listens on the wildcard (8080). The ports are
  constants in `launcher/src/mapping.ts`, the internal address variables are
  rejected by the launcher's unknown-variable guard, and the supervisor
  dials the container's own external addresses at boot and refuses to serve
  if anything accepts. Changing any of this is a security decision.
  [e2e/tests/prod-trust-boundary.test.ts](../../e2e/tests/prod-trust-boundary.test.ts)
  asserts it from the outside — a sibling container on the private network
  dialling each internal port, and the front door refusing every spelling of
  the blocked prefixes — so the boot gate is not the only thing standing
  between a regression and a release.
- **`Caddyfile` is a sibling of [docker/Caddyfile](../Caddyfile)**: same
  routes, same `/core/api/x/*` and `/auth-realtime/api/internal/*` 403
  blocks — only the upstreams differ. Change the two files together.
- **The external namespace is minimal and vendor-neutral.** Operators see
  only the flat `OXYNOTE_*` variables declared in `launcher/src/env.ts`:
  derive from shared values (one `OXYNOTE_PUBLIC_URL`, DSN-style URLs
  carrying credentials) instead of adding variables, generate secrets
  instead of exposing them, and never leak an internal dependency's name
  into a public variable. Any undeclared `OXYNOTE_*` variable is a boot
  error.
- **Secrets precedence** (`launcher/src/secrets.ts`): explicit env override
  wins and is never persisted → existing volume file → generate
  (`crypto.randomBytes`) + persist 0600. The data-source encryption key is
  unrotatable — never weaken this path.
- **Sentry DSNs are build-time only** (esbuild `--define` from Dockerfile
  ARGs). No runtime DSN variables; `OXYNOTE_CRASH_REPORTING_DISABLED` is the
  only runtime switch. There are four: web, core and auth-realtime get
  theirs passed down as child env, and the launcher keeps its own
  (`bakedLauncherSentryDsn`) because it reports for itself. That fourth one
  is not redundant — it covers what no child can report, a child killed
  outright and a boot that fails before any child exists. The launcher reads
  the disable switch straight from `process.env` rather than from `Config`,
  since an invalid environment is one of the failures worth reporting and
  `loadConfig` throws on it.
- **Crash policy: die, don't restart.** Any child exit tears the rest down
  and exits the container nonzero; restarts belong to the container runtime.
- **Every line the container emits is a JSON record with a `service`
  field** naming the process that wrote it: `launcher`, `core`,
  `auth-realtime`, `web`, `caddy`.
- **`service` is the only thing the launcher adds to a child's line.**
  `childRecord` ([logging.ts](launcher/src/logging.ts)) merges it into the
  object the child logged and changes nothing else, so core's `level` and
  caddy's `ts` survive as their own keys; the launcher's own level and
  timestamp would describe the relay rather than the event. A line that is
  not a JSON object becomes `{msg, service}`. `service` is written last, so
  it wins over a child that claims a different one.
- **The launcher's own events go through pino** at a fixed info level:
  `OXYNOTE_LOG_LEVEL` governs the services, not the container's account of
  its own boot. Both paths share one open stdout, so records reach the
  stream in write order.
- **Each stream gets its own splitter**, since a line is only whole within
  the stream it arrived on; `mute` drops what a child prints with no way to
  switch it off (nitro's loopback listen banner).
- **Supply chain**: every `FROM` is pinned by digest; caddy and tini are
  downloaded at pinned versions and checksum-verified. Caddy is Apache-2.0
  (its LICENSE ships in the image at `/oxynote/licenses/caddy/`); the name
  "Caddy" is trademarked — bundle it unmodified and unrebranded.

## launcher/

Follows [server/auth-realtime](../../server/auth-realtime/CLAUDE.md)'s
conventions: tabs-at-8 prettier, no semicolons, explicit `.js` import
extensions, type-aware eslint, knip, vitest with concurrent tests and the
context-local `expect`. `src/index.ts` is the only module with side effects
(env, filesystem, spawning, signals) and is excluded from coverage;
everything else is a factory taking its dependencies and is fully tested.

```sh
cd docker/prod/launcher
pnpm run qa           # check-lint + test
pnpm run build:bundle # esbuild bundle -> dist/launcher.mjs; the SENTRY_*_DSN
                      # env vars become baked defines
```

The runtime base is pinned alpine with alpine's own nodejs package — the
v24 line built against shared system libraries, about half the size of
the official node binary — plus `icu-data-full` so `Intl` formats every
locale during SSR. The image ships **no node_modules**:
auth-realtime and the launcher are single esbuild bundles (sentry is
folded into both — require-hook auto-instrumentation is lost, error
capture kept) and the web server is nitro's terser-minified
self-contained output (`NITRO_MINIFY=true`). With no native addons the
musl runtime cannot clash with anything, and the web builder fails the
build if a `*.node` file ever lands in the output.

**Both bundles carry a `createRequire` banner**, and it is what makes
folding sentry in possible at all: its opentelemetry dependencies still
`require()` node builtins at runtime, and an `--format=esm` bundle has no
`require` to give them — esbuild's shim throws `Dynamic require of "util"
is not supported` on the first import, before any of this code runs. The
banner hands the shim a real `require` built from `import.meta.url`.
Removing it from either package breaks that package's container at
startup, and no unit test will tell you: the bundle is only built in the
image.

## References

Established practice this design follows:

- Hoppscotch AIO — tini + node launcher spawning caddy/backend/frontend,
  checksum-verified caddy: <https://github.com/hoppscotch/hoppscotch>
  (`aio_run.mjs`, `healthcheck.sh`, `prod.Dockerfile`)
- Baserow all-in-one — bundled Caddy, single public-URL variable, secret
  files on the data volume: <https://gitlab.com/baserow/baserow>
  (`deploy/all-in-one/`)
- Appsmith fat container — layered healthcheck, generated secrets on the
  volume: <https://github.com/appsmithorg/appsmith> (`deploy/docker/`)
- grist-omnibus — the anti-pattern reference (no tini, no SIGTERM handling):
  <https://github.com/gristlabs/grist-omnibus>
- tini: <https://github.com/krallin/tini> · Docker multi-process guidance:
  <https://docs.docker.com/engine/containers/multi-service_container/>
- Caddy (Apache-2.0, trademarked name): <https://github.com/caddyserver/caddy>
