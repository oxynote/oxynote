# Server

To build and run the core and auth-realtime applications simply use the following command:

```
make run
```

By default the following ports are opened:
- `8080` - core application's HTTP and WebSocket server.
- `8081` - auth-realtime's Hocuspocus HTTP and WebSocket server.

# Tag API

`PUT /api/tags/{tagId}` renames or recolours an existing tag in the active
organization. The route requires a session and is available through the
front door at `/core/api/tags/{tagId}`.

Send a JSON object with `tagName`, `color`, or both. Omitted or `null` fields
keep their current values; at least one field must be set. A name cannot be
empty, and a colour must be a hex value from the existing tag palette.
Successful updates return `204 No Content` and notify the organization's
`change@tag-tree` subscribers. The tag's ID, display order, and document
branch assignments stay unchanged.

Invalid input returns `400`, duplicate names return `409`, and missing tags
or tags outside the active organization return `404`.

# Better Auth

## Using Migrations

Nothing to do by hand: the Better Auth tables are owned by core's SQL
migrations (`server/core/internal/db/migrations/`), which are embedded in the
binary and applied automatically on startup.

## Generating the Reference Schema

`auth-realtime/sql/better_auth_schema.sql` is a generated reference of the
schema Better Auth expects — regenerate it after changing the Better Auth
config in `src/auth.ts` and diff it against core's migrations to see whether a
new migration is needed.

To generate it, start a postgres container:

```
docker run \
  --name betterauth-db \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=betterauth \
  -p 5432:5432 \
  --rm \
  postgres
```

Then put the DB DSN values into the env var:
```
OXYNOTE_AUTH_REALTIME_DB_DSN=postgresql://postgres:postgres@localhost:5432/betterauth
```

Then, from within the `auth-realtime` directory, run (make sure that other env vars
that are needed by src/auth.ts are available and exported too):

```
source ../../docker/env/auth-realtime.local.env && npx @better-auth/cli generate --output ./sql/better_auth_schema.sql
```
