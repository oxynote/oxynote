# Oxynote production image

One container running the whole product: Caddy (front door), the web app
(Nuxt SSR), the core API server, the auth/realtime service and a PostgreSQL
database, supervised by a small launcher. Nothing outside the image is
required: an external PostgreSQL can take the embedded one's place, and
Valkey, an S3-compatible object store, an SMTP relay and changedetection.io
are optional and external. Full-text search is built in: core keeps its
index on the data volume and rebuilds it from PostgreSQL when it is missing,
so the image runs as a single instance.

## Quick start

```sh
cp docker/prod/docker-compose.example.yaml my-deployment.yaml
# replace every change-me value; for a real domain set OXYNOTE_PUBLIC_URL
docker compose -f my-deployment.yaml up -d
```

From a repository checkout, `make prod-run` builds the image locally
(goreleaser builds the core binary first — a bare `docker build` is not
supported) and runs this example on `http://localhost:8080`, with
`docker-compose.local.yaml` layered on top to add an email sender: a mailpit
that shows the delivered mail at `http://localhost:8025`. `make
prod-run-no-email` runs the example without that override, as a deployment
without an email sender. `make prod-stop` stops either.

The image serves plain HTTP on port **8080** and assumes it is reached at
`http://localhost:8080` unless told otherwise, so publish the container as
`8080:8080` for a local run. For a real domain, put a TLS-terminating proxy
in front and set `OXYNOTE_PUBLIC_URL` to the public `https://` origin.

## Configuration

Everything is configured through flat `OXYNOTE_*` variables. Setting any
other `OXYNOTE_`-prefixed variable (a typo, or a component-internal name)
fails the boot with an error naming it.

### Database

| Variable | Meaning |
| --- | --- |
| `OXYNOTE_DB_DSN` | an external PostgreSQL, e.g. `postgresql://user:pass@host/db?sslmode=require`. Unset, the image runs its own PostgreSQL 18 with its data under `/oxynote/data/postgres`; set, the embedded one never starts. One database serves the whole product; migrations run automatically at boot. |

### Public address

| Variable | Meaning |
| --- | --- |
| `OXYNOTE_PUBLIC_URL` | the origin users open in the browser, e.g. `https://notes.example.com`. Scheme + host only — every public URL, the cookie domain, and the CORS rules derive from it. Defaults to `http://localhost:8080`, which is right only while the container's port 8080 is published as host port 8080; set it for a domain or any other host port. |

### Optional features

Each feature is keyed on one variable; leaving it unset disables the feature
cleanly. An incomplete group fails the boot with the missing name.

| Group | Variables |
| --- | --- |
| Valkey/Redis | `OXYNOTE_VALKEY_DSN` (`redis[s]://[user:pass@]host:port[/db]`, credentials supported). Without it the assistant keeps its conversations in the core process and sessions are not cached outside Postgres. |
| Object storage | `OXYNOTE_OBJECT_STORAGE_DSN` — an S3-compatible store as one URL: `http(s)://ACCESS_KEY:SECRET_KEY@host:port/bucket[?region=...]` (the bucket defaults to `oxynote` and is created if missing). Without it uploaded images are kept on the data volume, which is the right choice for a single-node deployment. |
| Email | `OXYNOTE_SMTP_DSN` (`smtp[s]://[user:pass@]host:port[?tls=none\|starttls\|tls]`), `OXYNOTE_EMAIL_FROM_ADDRESS`. Without it the product runs without email, as described below. |
| GitHub App | `OXYNOTE_GITHUB_APP_ID`, `OXYNOTE_GITHUB_APP_SLUG`, `OXYNOTE_GITHUB_APP_SIGNATURE_SECRET`; mount the app's private key at `/oxynote/github/private-key.pem` |
| Slack app | `OXYNOTE_SLACK_APP_CLIENT_ID`, `OXYNOTE_SLACK_APP_CLIENT_SECRET`, `OXYNOTE_SLACK_APP_SIGNATURE_SECRET` |
| Social login | `OXYNOTE_SOCIAL_LOGIN_{GITHUB,GOOGLE,SLACK}_CLIENT_ID` + `_CLIENT_SECRET` (both halves per provider) |
| AI assistant | `OXYNOTE_AI_ASSISTANT_PROVIDER` (`anthropic`, `openai`, `google`, `ollama`, `openrouter`) plus the vendor's credentials, detailed in [docs/ai.md](../../docs/ai.md): `OXYNOTE_AI_ASSISTANT_API_KEY`, `_MODEL`, `_BASE_URL`, `_MAX_TOKENS`, `_REQUEST_TIMEOUT`, `_SUMMARY_MODEL`, `_AZURE_API_VERSION`, `_BEDROCK_{REGION,ACCESS_KEY,SECRET_ACCESS_KEY,SESSION_TOKEN}`, `_VERTEX_{PROJECT_ID,REGION,SERVICE_ACCOUNT_JSON}` |
| URL watching | `OXYNOTE_CHANGE_DETECTION_URL`, `OXYNOTE_CHANGE_DETECTION_API_KEY` (a changedetection.io instance) |

### Without an email sender

Without `OXYNOTE_SMTP_DSN` nothing is emailed, and the flows that would wait
for a link work without one:

- Signup skips email verification and goes straight to workspace creation.
- Invitations are still created, but their links have to be passed on by
  hand. Workspace settings offer **Copy invitation link** on each pending
  invitation, and show a notice about it.
- Changing the email address or deleting the account asks for the current
  password. Accounts that sign in only through a social login have no
  password, so they can do neither.
- Password reset is switched off, and there is no other way to recover a
  forgotten password. The login page tells users to contact their
  administrator.

Setting `OXYNOTE_SMTP_DSN` later brings the email flows back after a
restart. Accounts created in the meantime have unverified addresses, so
their next login sends a verification link first.

### Workspaces

`OXYNOTE_MAX_ORGANIZATIONS` caps the number of workspaces and
`OXYNOTE_MAX_ORGANIZATION_MEMBERS` the members of each. `-1` removes a
limit. Members are unlimited by default.

The workspace limit defaults to `1`, which runs the image as a single
workspace:

- The first boot creates a workspace named Oxynote and its admin. Sign in as
  `admin@example.com` with the password `oxynote-admin-1234`, then change
  the email address, the password and the workspace name in the settings.
  The admin's address receives no email, so a new address is confirmed by a
  link sent to the new address only.
- Nobody can sign up without an invitation. The signup page sends other
  visitors to the login page, and an invited address can create its account
  from the invitation link.
- The last member cannot delete their account.

The login page shows the default email and password until the admin
changes the password. Anyone who can reach the instance can sign in with
them until then, so change the password before exposing the instance.

Any other value lets people sign up and create their own workspace until
the limit is reached. After that they can still sign up, but only to join a
workspace through an invitation.

### Tuning

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
GitHub/Slack install-state keys, the embedded database's password) are
generated on first boot and stored under `/oxynote/data/secrets/` with
owner-only permissions — keep that volume.

- Unless `OXYNOTE_DB_DSN` is set, the database lives there too, under
  `/oxynote/data/postgres`, and so does every document. Back it up with the
  `pg_dump` the image ships:

  ```sh
  docker compose exec -T oxynote sh -c 'PGPASSWORD="$(cat /oxynote/data/secrets/database-password)" \
    /usr/libexec/postgresql18/pg_dump -h /tmp/postgresql -U oxynote oxynote' > oxynote.sql
  ```

- Unless an object store is configured, every uploaded image lives there
  too, under `/oxynote/data/object-storage`.
- **Losing the volume signs everyone out, permanently orphans every stored
  data-source credential, and takes the uploaded images, and the embedded
  database, with it.**
- Two advanced overrides exist for migrating an existing deployment in:
  `OXYNOTE_AUTH_SECRET` (session/token signing) and
  `OXYNOTE_DATA_SOURCE_ENCRYPTION_KEYS` (comma-separated, newest first,
  each the standard base64 of exactly 32 bytes). An override always wins
  and is never written to disk.
- To rotate the data-source encryption key, read the current key from
  `secrets/data-source-encryption-key` on the volume (or from your
  existing override), set `OXYNOTE_DATA_SOURCE_ENCRYPTION_KEYS` to
  `<new key>,<current key>` and restart: core re-encrypts every stored
  credential under the new key at boot and logs how many it moved. On a
  later restart set the override to `<new key>` alone. The override is
  never written back, so from then on it stays set; to return to the
  generated file instead, write `<new key>` into
  `secrets/data-source-encryption-key` before dropping the override, or
  core boots with only the retired key and reads no credential.

## Security model

- Only port **8080** (Caddy) is reachable. Core, auth-realtime, and the web
  server bind the container's loopback, so their unauthenticated internal
  surfaces cannot be reached from any network — the launcher verifies this
  at boot and refuses to serve otherwise. Caddy additionally blocks the
  internal paths (`/core/api/x/*`, `/auth-realtime/api/internal/*`) at the
  front door.
- The embedded PostgreSQL listens on a unix socket inside the container and
  on no TCP port. Every connection needs the app role's generated password,
  and that role owns the product's database and nothing else, so a data
  source pointed at the socket gets nowhere.
- Keep the backing services on a private network reachable only by this
  container, and where Valkey is used enable its authentication
  (`requirepass` + a credentialed `OXYNOTE_VALKEY_DSN`).
- The image runs as a non-root user and writes only to `/oxynote/data` and
  `/tmp`, so it supports a read-only root filesystem
  (`docker run --read-only --tmpfs /tmp`, Kubernetes
  `readOnlyRootFilesystem: true`).

## Error reporting

Official images carry Sentry DSNs baked in at build time, so crashes in
self-hosted deployments can reach the Oxynote team. A DSN is a write-only
ingest address: it lets the image send events, never read them, and nothing
can be queried through it.

What an event carries is the crash itself — the error, its stack trace, the
component and release, the container's hostname, and the request path that
failed. Every component runs with personal-data collection off, so no
cookies, request bodies or client IPs are attached, and no document content
is sent. An error message can still quote what it was working on, so treat the
reports as diagnostic data leaving your deployment, and switch them off with
`OXYNOTE_CRASH_REPORTING_DISABLED=true` if that is not acceptable. Locally
built images carry no DSNs and report nothing.

## Bundled software

The image bundles [the Caddy web server](https://github.com/caddyserver/caddy)
unmodified (Apache-2.0) and [tini](https://github.com/krallin/tini) (MIT) as
the init process. Both are downloaded at pinned versions and
checksum-verified during the image build, and both licenses ship under
`/oxynote/licenses/`. Node.js comes from the pinned Alpine base (the v24 line, with full
ICU locale data), and so does PostgreSQL 18.

The image is published for `linux/amd64` and `linux/arm64`; a pull fetches
the variant for the machine's architecture.
