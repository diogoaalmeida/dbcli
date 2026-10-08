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

...or manage named profiles, stored in `~/.config/dbcli/profiles.env`. The
DSN carries a password, so prefer piping it in over passing it as a plain
argument, which shell history and `ps` can both expose:

```bash
echo "postgres://dbcli_agent:change-me@localhost:5432/mydb?sslmode=disable" | dbcli profiles add dev
dbcli query "select 1" --profile dev
```

`dbcli profiles add dev "<dsn>"` still works if you pass the DSN directly,
with that tradeoff.

See `.env.example` for both forms.

## Commands

- `query`: run a validated, read-only query
- `explain`: print the query plan (`--analyze` for `EXPLAIN ANALYZE`)
- `schemas`: list non-system schemas with their table counts
- `schema`: list tables in one schema
- `describe`: show a table's columns, indexes, constraints, comments, and
  relationships (including what references it)
- `sample`: print up to N rows from a table
- `profiles`: manage named connection profiles (`list` / `add` / `remove`)

```
$ dbcli --help
dbcli: read-only SQL for AI agents

Usage:
  dbcli query "<SQL>" [--profile NAME] [--limit N] [--timeout Ns] [--format json|table]
  dbcli explain "<SQL>" [--profile NAME] [--analyze] [--timeout Ns] [--format json|table]
  dbcli schemas [--profile NAME] [--timeout Ns]
  dbcli schema [--profile NAME] [--schema public] [--timeout Ns]
  dbcli describe <table> [--profile NAME] [--schema public] [--timeout Ns]
  dbcli sample <table> [--profile NAME] [--schema public] [--limit 20] [--timeout Ns] [--format json|table]
  dbcli profiles list
  dbcli profiles add <name> [dsn]   (reads dsn from stdin if omitted)
  dbcli profiles remove <name>
  dbcli version

Connection: --profile NAME resolves against ~/.config/dbcli/profiles.env,
or set DATABASE_URL directly. See README.md for setup, including creating
a least-privilege read-only database role.
```

Run `dbcli <command> --help` for that command's individual flags (e.g. `--driver`, present on every command, to override the scheme inferred from the connection string).

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
base64. Range types (`numrange`, `int8range`, `daterange`, etc.) become
`{"lower": ..., "upper": ..., "lower_type": "inclusive"|"exclusive", "upper_type": ..., "valid": true}`,
with bounds following the same rules as their scalar type.

## Example

A typical flow for an agent that has never seen this database before: find
out what schemas exist, list the tables in one, inspect a table's shape,
then answer a real question, against a small `shop` schema (`customers`,
`orders`, `order_items`).

```bash
$ dbcli schemas --profile prod
{
  "ok": true,
  "data": [
    { "name": "public", "table_count": 0 },
    { "name": "shop", "table_count": 3 }
  ]
}
```

```bash
$ dbcli schema --profile prod --schema shop
{
  "ok": true,
  "data": [
    { "schema": "shop", "name": "customers", "kind": "table", "estimated_rows": 2, "stats_known": true },
    { "schema": "shop", "name": "order_items", "kind": "table", "estimated_rows": 3, "stats_known": true },
    { "schema": "shop", "name": "orders", "kind": "table", "estimated_rows": 4, "stats_known": true }
  ]
}
```

`stats_known` is `false` when a table has never been vacuumed or
analyzed, so `estimated_rows` would otherwise look like a misleading 0.

```bash
$ dbcli describe orders --profile prod --schema shop
{
  "ok": true,
  "data": {
    "schema": "shop",
    "table": "orders",
    "columns": [
      { "name": "id", "type": "integer", "nullable": false, "default": "nextval('shop.orders_id_seq'::regclass)" },
      { "name": "customer_id", "type": "integer", "nullable": true },
      { "name": "status", "type": "USER-DEFINED", "nullable": false, "enum_values": ["pending", "shipped", "cancelled"] },
      { "name": "created_at", "type": "timestamp with time zone", "nullable": false, "default": "now()" }
    ],
    "indexes": [
      { "name": "orders_pkey", "columns": ["id"], "unique": true, "primary": true }
    ],
    "foreign_keys": [
      {
        "column": "customer_id", "ref_table": "customers", "ref_column": "id",
        "constraint_name": "orders_customer_id_fkey", "ref_schema": "shop",
        "columns": ["customer_id"], "ref_columns": ["id"]
      }
    ],
    "primary_key": ["id"],
    "referenced_by": [
      {
        "constraint_name": "order_items_order_id_fkey", "schema": "shop", "table": "order_items",
        "columns": ["order_id"], "ref_columns": ["id"]
      }
    ]
  }
}
```

`describe` also reports `unique_constraints`/`check_constraints` (name +
definition), table/column `comment`s, and `view_definition` for a view or
materialized view. These are omitted when they don't apply to the table,
same as `enum_values` above. `foreign_keys` and `referenced_by` are
always present, as `null` when there's nothing to report.

```bash
$ dbcli query "select status::text, count(*) from shop.orders group by status" --profile prod
{
  "ok": true,
  "columns": [
    { "name": "status", "type": "text" },
    { "name": "count", "type": "int8" }
  ],
  "rows": [
    { "status": "cancelled", "count": "1" },
    { "status": "pending", "count": "1" },
    { "status": "shipped", "count": "2" }
  ],
  "row_count": 3,
  "truncated": false,
  "duration_ms": 1,
  "query": "select status::text, count(*) from shop.orders group by status"
}
```

(`status` is cast to `text` here because it's an enum column — see the
`enum_values` in `describe orders` above; grouping by it directly works
too, just reported with its real type name instead of `"text"`.)

And the guarantee this whole tool is built around, in action:

```bash
$ dbcli query "delete from shop.orders" --profile prod
{
  "ok": false,
  "error": "query rejected: only SELECT/WITH/EXPLAIN/TABLE statements are allowed",
  "code": "query_error"
}
```

## License

MIT, see [LICENSE](LICENSE).
