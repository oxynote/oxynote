# Server

To build and run the core and auth-realtime applications simply use the following command:

```
make run
```

By default the following ports are opened:
- `8080` - core application's HTTP and WebSocket server.
- `8081` - auth-realtime's Hocuspocus HTTP and WebSocket server.

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

(You can comment redis, etc. out in the auth.ts file)
