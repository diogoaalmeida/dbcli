---
name: dbcli
description: |
  This skill should be used when the user asks to explore, query, inspect,
  or sample a Postgres database, asks what schemas or tables exist, wants
  to check a table's structure or a column's type, or wants to run a SQL
  query against Postgres, and the dbcli CLI tool is available or should be
  installed. dbcli enforces read-only access itself (SQL classifier plus
  a READ ONLY transaction), with a row cap and statement timeout. Covers
  the install check, the safe discovery workflow, and how to read
  dbcli's JSON output correctly.
license: MIT
metadata:
  version: "1.1.0"
---

# dbcli: read-only Postgres queries

dbcli is a read-only SQL CLI for Postgres built for AI agents. Every
query runs inside a read-only transaction with a hard row cap and a
statement timeout. Every success or failure prints a JSON envelope on
stdout (except `version`/`help`, which print plain text).

## Check availability first

Run `dbcli version`. If the command isn't found, tell the user dbcli
isn't installed and ask before installing it; don't install software
on their machine without that confirmation. Once they agree, install a
pinned release. With Go available:

    go install github.com/diogoaalmeida/dbcli@v1.1.0

The binary lands in `$(go env GOPATH)/bin`, which may not be on `PATH`
yet. If `dbcli version` still isn't found right after installing,
check that directory is on `PATH` before assuming the install failed.

Without Go, download the prebuilt archive for the user's OS and
architecture from the v1.1.0 release
(https://github.com/diogoaalmeida/dbcli/releases/tag/v1.1.0, named
`dbcli_<os>_<arch>.tar.gz`, or `.zip` on Windows), verify it against
`checksums.txt` from the same release, and put the `dbcli` binary on
`PATH`.

Use the connection the user has authorized: `DATABASE_URL`, a named
profile (`dbcli profiles list`), or credentials they provide for this
task. If they ask for a local database and no connection is configured,
check running containers (`docker ps`) or the default port
(`pg_isready -h localhost -p 5432`). These checks identify possible
servers; they do not authorize access. Ask which database to use when
the target or credentials are unclear. Do not search unrelated files,
inspect container secrets, guess credentials, or reuse a stale DSN.
Add an authorized connection as a profile through stdin:

    echo "postgres://user:pass@host:5432/db" | dbcli profiles add <name>

Never pass a DSN as a plain command-line argument when stdin is an
option: shell history and `ps` can both expose it.

## Explore before querying

Follow this order for an unfamiliar database rather than guessing table
names or writing SQL blind:

1. `dbcli schemas --profile <name>`: list non-system schemas.
2. `dbcli schema --profile <name> --schema <schema>`: list tables in
   one schema.
3. `dbcli describe <table> --profile <name> --schema <schema>`:
   columns, indexes, constraints, comments, enum values, and what
   references the table.
4. `dbcli sample <table> --profile <name> --schema <schema>` or
   `dbcli query "<SQL>" --profile <name>`: look at real rows, or
   answer a specific question.

Each step narrows the search. Skipping ahead to `query` on an
unfamiliar table risks a wrong assumption about a column's name or type
that `describe` would have caught immediately.

## Trust the safety guarantees already built in

dbcli enforces read-only access two ways (a SQL classifier, then a
Postgres `READ ONLY` transaction regardless of what the classifier
decided), a hard row cap (default 500 rows, ceiling 5000), and a
statement timeout (default 5s). Within the user's authorized database
and task, run read queries without asking for confirmation each time.
Follow the host agent's permission controls and any limits the user sets.

Still point dbcli at a least-privilege database role when one exists,
rather than treating dbcli's own safeguards as a substitute for one. If
a DSN being added via `profiles add` (or an existing `DATABASE_URL`)
looks like a superuser or admin connection string, say so to the user
rather than proceeding silently.

## Read the output correctly

Parse stdout as JSON for every command except `version` and `help`.
Before interpreting an unexpected `describe` or `query` result, read
`references/output-shapes.md`. It covers numbers as strings, date and
timestamp formats, range types, and the `stats_known` caveat for
partitioned tables.

On a query error (`{"ok": false, "error": "...", "code": "..."}`),
check `code` before retrying. `query_error` usually means the
classifier rejected a non-SELECT statement or the query hit the
statement timeout: rephrase the query rather than retrying it
unchanged.
