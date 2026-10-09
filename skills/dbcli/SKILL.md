---
name: dbcli
description: |
  This skill should be used when the user asks to explore, query, inspect,
  or sample a Postgres database, asks what schemas or tables exist, wants
  to check a table's structure or a column's type, or wants to run a SQL
  query against Postgres, and the dbcli CLI tool is available or should be
  installed. Covers the install check, the safe discovery workflow, and
  how to read dbcli's JSON output correctly.
license: MIT
metadata:
  version: "1.1.0"
---

# dbcli: read-only Postgres queries

dbcli is a read-only SQL CLI for Postgres built for AI agents. Every
query runs inside a read-only transaction with a hard row cap and a
statement timeout, and every result — success or failure — is a JSON
envelope on stdout (except `version`/`help`, which print plain text).

## Check availability first

Run `dbcli version`. If the command isn't found, install it:

    go install github.com/diogoaalmeida/dbcli@latest

The binary lands in `$(go env GOPATH)/bin`, which may not be on `PATH`
yet — if `dbcli version` still isn't found right after installing,
check that directory is on `PATH` before assuming the install failed.

Then confirm a connection is configured: either `DATABASE_URL` is set,
or a profile exists (`dbcli profiles list`). If neither is set and the
user implies a local database ("my local postgres"), check for one
before asking — a running Postgres container (`docker ps`) or the
default port (`pg_isready -h localhost -p 5432`) is often enough to
find it and its credentials without making the user repeat information
they expect you to discover yourself. Only ask the user for a
connection string when nothing local turns up, or it's ambiguous which
database they mean — never fabricate or reuse a stale DSN. Add it as a
profile by piping it through stdin, not as a plain argument:

    echo "postgres://user:pass@host:5432/db" | dbcli profiles add <name>

Never pass a DSN as a plain command-line argument when stdin is an
option — shell history and `ps` can both expose it.

## Explore before querying

Follow this order for an unfamiliar database rather than guessing table
names or writing SQL blind:

1. `dbcli schemas --profile <name>` — list non-system schemas.
2. `dbcli schema --profile <name> --schema <schema>` — list tables in
   one schema.
3. `dbcli describe <table> --profile <name> --schema <schema>` —
   columns, indexes, constraints, comments, enum values, and what
   references the table.
4. `dbcli sample <table> --profile <name> --schema <schema>` or
   `dbcli query "<SQL>" --profile <name>` — look at real rows, or
   answer a specific question.

Each step narrows the search. Skipping ahead to `query` on an
unfamiliar table risks a wrong assumption about a column's name or type
that `describe` would have caught immediately.

## Trust the safety guarantees already built in

dbcli enforces read-only access two ways (a SQL classifier, then a
Postgres `READ ONLY` transaction regardless of what the classifier
decided), a hard row cap (default 500 rows, ceiling 5000), and a
statement timeout (default 5s). Don't add redundant confirmation
prompts or extra caution around read queries run through dbcli — that
safety is already enforced server-side, not just by convention.

Still point dbcli at a least-privilege database role when one exists,
rather than treating dbcli's own safeguards as a substitute for one. If
a DSN being added via `profiles add` (or an existing `DATABASE_URL`)
looks like a superuser or admin connection string, say so to the user
rather than proceeding silently.

## Read the output correctly

Parse stdout as JSON for every command except `version` and `help`.
See `references/output-shapes.md` for field conventions — numbers as
strings, timestamp/date formats, range types, and the `stats_known`
partitioned-table caveat — before interpreting a `describe` or `query`
result that looks unexpected.

On a query error (`{"ok": false, "error": "...", "code": "..."}`),
check `code` before retrying. `query_error` usually means the
classifier rejected a non-SELECT statement or the query hit the
statement timeout — rephrase the query rather than retrying it
unchanged.
