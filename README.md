# dbcli

[![CI](https://github.com/diogoaalmeida/dbcli/actions/workflows/ci.yml/badge.svg)](https://github.com/diogoaalmeida/dbcli/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/diogoaalmeida/dbcli.svg)](https://pkg.go.dev/github.com/diogoaalmeida/dbcli)
[![Go Report Card](https://goreportcard.com/badge/github.com/diogoaalmeida/dbcli)](https://goreportcard.com/report/github.com/diogoaalmeida/dbcli)

A small, read-only SQL CLI built for AI agents. It's meant to be a tool call
in an agent's toolbox: every result is a stable JSON envelope on stdout, and
every query is constrained so an agent (or a prompt-injected one) can't
mutate your database, run away with resources, or dump millions of rows into
context.

Currently speaks Postgres. The internal driver interface is engine-neutral
so MySQL/SQLite support can be added later without a rewrite, but only
Postgres ships today.

## Why this is safe to point at a real database

- **Read-only, enforced twice.** Every query is first classified: only
  `SELECT`, `WITH`, `EXPLAIN`, and `TABLE` statements are accepted, and only
  one statement at a time. Then, regardless of what the classifier decided,
  the query still runs inside a Postgres `READ ONLY` transaction, so even a
  gap in the classifier can't result in a write.
- **Statement timeout.** Every query has a timeout (default 5s, `--timeout`
  to override) so a runaway query can't hang the agent or the database.
- **Hard row cap.** Every query is wrapped in an outer `LIMIT` (default 500,
  hard ceiling 5000 even if you ask for more) so an agent can't accidentally
  pull millions of rows into its context window.
- **No raw identifier interpolation.** `dbcli sample <table>` checks the
  table exists via `information_schema` first, then safely quotes it. It
  never string-interpolates a table name into SQL.
- **You should still connect with a least-privilege role.** All of the above
  is defense in depth, not a replacement for using a database user that
  literally cannot write. See below.

## Install

```bash
go install github.com/diogoaalmeida/dbcli@latest
```

Or build from source:

```bash
git clone https://github.com/diogoaalmeida/dbcli.git
cd dbcli
go build -o dbcli .
```

## Set up a least-privilege role

Don't point dbcli at a superuser connection string. Create a role that can
only read:

```sql
CREATE ROLE dbcli_agent WITH LOGIN PASSWORD 'change-me';
GRANT CONNECT ON DATABASE mydb TO dbcli_agent;
GRANT USAGE ON SCHEMA public TO dbcli_agent;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO dbcli_agent;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO dbcli_agent;
```

## Connect

Either set `DATABASE_URL` directly:

```bash
export DATABASE_URL="postgres://dbcli_agent:change-me@localhost:5432/mydb?sslmode=disable"
dbcli query "select 1"
```

...or manage named profiles, stored in `~/.config/dbcli/profiles.env`:

```bash
dbcli profiles add dev "postgres://dbcli_agent:change-me@localhost:5432/mydb?sslmode=disable"
dbcli query "select 1" --profile dev
```

See `.env.example` for both forms.

## Commands

- `query`: run a validated, read-only query
- `explain`: print the query plan (`--analyze` for `EXPLAIN ANALYZE`)
- `schemas`: list non-system schemas with their table counts
- `schema`: list tables in one schema
- `describe`: show a table's columns, indexes, and foreign keys
- `sample`: print up to N rows from a table
- `profiles`: manage named connection profiles (`list` / `add` / `remove`)

Run `dbcli <command> --help` or see the [package documentation](https://pkg.go.dev/github.com/diogoaalmeida/dbcli) for the full flag reference.

Every result, success or failure, is JSON on stdout:

```json
{
  "ok": true,
  "columns": [{"name": "id", "type": "int4"}],
  "rows": [{"id": "1"}],
  "row_count": 1,
  "truncated": false,
  "duration_ms": 2,
  "query": "select id from vehicles limit 500"
}
```

```json
{"ok": false, "error": "query rejected: only SELECT/WITH/EXPLAIN/TABLE statements are allowed", "code": "query_error"}
```

Numbers (`int4`, `int8`, `numeric`, etc.) are serialized as JSON strings to
avoid precision loss for large integers and decimals, so parse them as
needed on the consuming side. `timestamp`/`timestamptz` are RFC3339;
`date` is a plain `"YYYY-MM-DD"` string with no time component. `bytea` is
base64.

## Using it from an agent

Point your agent at the `dbcli` binary as a shell tool. A typical flow:
`dbcli schemas` to see what schemas exist, `dbcli schema --schema X` to see
what tables are in one, `dbcli describe <table> --schema X` to see its
columns/indexes/foreign keys, then `dbcli query "..."` to actually answer a
question. Each step returns the same predictable JSON shape, so parsing
logic doesn't need to special-case each command.

## Development

```bash
go build ./...
go vet ./...
go test ./...
```

Integration tests in `internal/postgres` need a local Postgres. The fixture
connection string only ever points at `localhost` with a throwaway password,
never a real credential:

```bash
docker run -d --name dbcli-test-pg -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:15
go test ./internal/postgres/...
```

## License

MIT, see [LICENSE](LICENSE).
