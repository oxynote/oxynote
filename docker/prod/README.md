# Oxynote production image

One container running the whole product: Caddy (front door), the web app
(Nuxt SSR), the core API server, and the auth/realtime service, supervised by
a small launcher. PostgreSQL runs outside the image and is the only thing
it requires; Valkey, an S3-compatible object store, Meilisearch, an SMTP
relay and changedetection.io are optional and also external.

## Quick start

```sh
cp docker/prod/docker-compose.example.yaml my-deployment.yaml
# replace every change-me value, set OXYNOTE_PUBLIC_URL
docker compose -f my-deployment.yaml up -d
```

From a repository checkout, `make prod-run` builds the image locally
(goreleaser builds the core binary first — a bare `docker build` is not
supported) and runs this example on `http://localhost:8080`, with
`docker-compose.local.yaml` layered on top so signup mail lands in a mailpit
at `http://localhost:8025` instead of being logged. `make prod-stop` stops
it.

The image serves plain HTTP on port **8080**. For a real domain, put a
TLS-terminating proxy in front and set `OXYNOTE_PUBLIC_URL` to the public
`https://` origin.

## Configuration

Everything is configured through flat `OXYNOTE_*` variables. Setting any
other `OXYNOTE_`-prefixed variable (a typo, or a component-internal name)
fails the boot with an error naming it.

### Required

| Variable | Meaning |
| --- | --- |
| `OXYNOTE_PUBLIC_URL` | the origin users open in the browser, e.g. `https://notes.example.com`. Scheme + host only — every public URL, the cookie domain, and the CORS rules derive from it. |
| `OXYNOTE_DB_DSN` | PostgreSQL DSN, e.g. `postgresql://user:pass@host/db?sslmode=disable`. One database serves the whole product; migrations run automatically at boot. |

### Optional features

Each feature is keyed on one variable; leaving it unset disables the feature
cleanly. An incomplete group fails the boot with the missing name.

| Group | Variables |
| --- | --- |
| Valkey/Redis | `OXYNOTE_VALKEY_DSN` (`redis[s]://[user:pass@]host:port[/db]`, credentials supported). Without it the assistant keeps its conversations in the core process and sessions are not cached outside Postgres. |
| Object storage | `OXYNOTE_OBJECT_STORAGE_DSN` — an S3-compatible store as one URL: `http(s)://ACCESS_KEY:SECRET_KEY@host:port/bucket[?region=...]` (the bucket defaults to `oxynote` and is created if missing). Without it uploaded images are kept on the data volume, which is the right choice for a single-node deployment. |
| Search | `OXYNOTE_MEILISEARCH_URL`, `OXYNOTE_MEILISEARCH_MASTER_KEY` |
| Email | `OXYNOTE_SMTP_DSN` (`smtp[s]://[user:pass@]host:port[?tls=none\|starttls\|tls]`), `OXYNOTE_EMAIL_FROM_ADDRESS`. Without email, verification mails are logged instead of sent. |
| GitHub App | `OXYNOTE_GITHUB_APP_ID`, `OXYNOTE_GITHUB_APP_SLUG`, `OXYNOTE_GITHUB_APP_SIGNATURE_SECRET`; mount the app's private key at `/oxynote/github/private-key.pem` |
| Slack app | `OXYNOTE_SLACK_APP_CLIENT_ID`, `OXYNOTE_SLACK_APP_CLIENT_SECRET`, `OXYNOTE_SLACK_APP_SIGNATURE_SECRET` |
| Social login | `OXYNOTE_SOCIAL_LOGIN_{GITHUB,GOOGLE,SLACK}_CLIENT_ID` + `_CLIENT_SECRET` (both halves per provider) |
| AI assistant | `OXYNOTE_AI_ASSISTANT_PROVIDER` (`anthropic`, `openai`, `google`, `ollama`, `openrouter`) plus the vendor's credentials: `OXYNOTE_AI_ASSISTANT_API_KEY`, `_MODEL`, `_BASE_URL`, `_MAX_TOKENS`, `_REQUEST_TIMEOUT`, `_SUMMARY_MODEL`, `_AZURE_API_VERSION`, `_BEDROCK_{REGION,ACCESS_KEY,SECRET_ACCESS_KEY,SESSION_TOKEN}`, `_VERTEX_{PROJECT_ID,REGION,SERVICE_ACCOUNT_JSON}` |
| URL watching | `OXYNOTE_CHANGE_DETECTION_URL`, `OXYNOTE_CHANGE_DETECTION_API_KEY` (a changedetection.io instance) |

### Tuning

`OXYNOTE_MAX_ORGANIZATIONS`, `OXYNOTE_MAX_ORGANIZATION_MEMBERS`,
`OXYNOTE_RATE_LIMIT_DISABLED` (`true`/`false`; the built-in limiter buckets
by client IP — disable it behind a proxy that hides the original IP and rate
limit there instead), `OXYNOTE_MAX_DOCUMENT_HISTORY_ENTRIES`,
`OXYNOTE_DOCUMENT_HISTORY_RETENTION` (Go duration, e.g. `2160h`),
`OXYNOTE_LOG_LEVEL` (`DEBUG`/`INFO`/`WARN`/`ERROR`; the floor for both core
and auth-realtime, `WARN` by default, so the container logs carry the
exceptions rather than the traffic),
`OXYNOTE_TERMS_OF_SERVICE_URL`, `OXYNOTE_PRIVACY_POLICY_URL`.

## Secrets and the data volume

Internal secrets (session signing, data-source credential encryption, the
GitHub/Slack install-state keys) are generated on first boot and stored under
`/oxynote/data/secrets/` with owner-only permissions — keep that volume.

- Unless an object store is configured, every uploaded image lives there
  too, under `/oxynote/data/object-storage`.
- **Losing the volume signs everyone out, permanently orphans every stored
  data-source credential** — the encryption key cannot be rotated — **and
  takes the uploaded images with it.**
- Two advanced overrides exist for migrating an existing deployment in:
  `OXYNOTE_AUTH_SECRET` (session/token signing) and
  `OXYNOTE_DATA_SOURCE_ENCRYPTION_KEY` (exactly 16, 24, or 32 bytes). An
  override always wins and is never written to disk.

## Security model

- Only port **8080** (Caddy) is reachable. Core, auth-realtime, and the web
  server bind the container's loopback, so their unauthenticated internal
  surfaces cannot be reached from any network — the launcher verifies this
  at boot and refuses to serve otherwise. Caddy additionally blocks the
  internal paths (`/core/api/x/*`, `/auth-realtime/api/internal/*`) at the
  front door.
- Keep the backing services on a private network reachable only by this
  container, and where Valkey is used enable its authentication
  (`requirepass` + a credentialed `OXYNOTE_VALKEY_DSN`).
- The image runs as a non-root user and writes only to `/oxynote/data` and
  `/tmp`, so it supports a read-only root filesystem
  (`docker run --read-only --tmpfs /tmp`, Kubernetes
  `readOnlyRootFilesystem: true`).

## Error reporting

Official images carry Sentry DSNs baked in at build time, so crashes in
self-hosted deployments can reach the Oxynote team. DSNs are write-only
ingest addresses — they expose nothing about your instance. Set
`OXYNOTE_CRASH_REPORTING_DISABLED=true` to switch reporting off entirely.
Locally built images carry no DSNs and report nothing.

## Bundled software

The image bundles [the Caddy web server](https://github.com/caddyserver/caddy)
unmodified (Apache-2.0; its license ships at `/oxynote/licenses/caddy/LICENSE`)
and [tini](https://github.com/krallin/tini) (MIT) as the init process. Both
are downloaded at pinned versions and checksum-verified during the image
build. Node.js comes from the pinned Alpine base (the v24 line, with full
ICU locale data).

Currently the image is built for `linux/amd64`.
