# dbcli roadmap: LLM-agent discovery features

## Goal

Move database discovery work out of the agent's context and into Go.
Today an agent explores an unfamiliar database through many round trips
(`schemas` → `schema` → `describe` × N → `sample` × N → trial-and-error
queries). Each step costs tokens and latency. The goal is to answer
common discovery questions in one call, in compact form, while keeping
every existing safety guarantee (read-only classifier + read-only
transaction, timeout, row cap, no raw identifier interpolation).

## Principles

1. **Safety is non-negotiable.** Every new command runs through
   `beginReadOnlyTimeoutTx` and the hard row cap. Identifiers are never
   string-interpolated; they're checked against the catalog and quoted
   with `pgx.Identifier`.
2. **Backwards compatible.** The existing JSON envelopes (`ok`, `data`,
   `error`, `code`) and field names don't change. New output shapes are
   opt-in (`--format`, `--compact`) or new commands. New fields on
   existing results are additive only.
3. **Token-efficient by default for new commands.** Prefer compact text
   (DDL) or columnar JSON where it helps; truncate large values with
   explicit markers.
4. **Engine-neutral interface.** New capabilities go on `driver.Conn`
   with engine-neutral types; Postgres is the only implementation for
   now.
5. **Tests for each change**, following the existing pattern: unit tests
   for pure logic, Postgres-backed tests where the repo already uses
   them. Every claim gets reproduced against a real Postgres before it's
   accepted as fixed or working, including claims from code review.

## Phase 0: correctness fixes to existing introspection (done)

Merged 2026-10-08 via [PR #1](https://github.com/diogoaalmeida/dbcli/pull/1).

Fixed a composite/cross-schema foreign-key cross-join bug in
`describe`, and added primary key, unique/check constraints, table and
column comments, enum values, view definitions, reverse foreign keys
(`referenced_by`), and partitioned/foreign tables in `schema`/`schemas`
listings, plus a `stats_known` signal. CI now matrixes Postgres 13
through 18 (`lint` gates a `test` matrix).

Known, documented limitation: a partitioned table's parent relation can
never show `stats_known: true` via autovacuum, since autovacuum only
analyzes leaf partitions, never the parent. A real fix needs aggregating
`reltuples` and analyze-status across `pg_inherits` children; judged
disproportionate for a correctness-fix phase, so it's documented on
`TableInfo.StatsKnown` and locked in with a regression test instead.

## Phase 1: one-call discovery (biggest token savings)

### 1.1 `dbcli overview [--schema X | --all-schemas] [--format json|ddl]`

Every table/view in scope, with columns, PK, FKs, comments, enum values,
and estimated rows, in one call. `--format ddl` emits `CREATE TABLE`-
style text (compact and LLM-friendly), including FK references and
comments as SQL comments.

Implementation: bulk catalog queries (one per object kind, not one per
table) to avoid N+1; bounded by a table cap (`--max-tables`, default
proposed 200) with `truncated` and a hint to narrow by `--schema` /
`--tables`. New `Conn` method: `Overview(ctx, schema, opts)
(*Overview, error)`.

### 1.2 `dbcli relations [--schema X] [--format json|mermaid]`

FK graph as edges (`from_table.col → to_table.col`); `mermaid` output for
an ER diagram.

`dbcli join-path <from> <to> [--max-depth N]`: BFS over the FK graph
(undirected, shortest path), returning the table chain plus ready-made
`JOIN ... ON ...` clauses. Pure Go graph logic over the edges from Phase
0's FK queries, so it's unit-testable without a database.

### 1.3 `dbcli search <term> [--schema X] [--kind table|column|comment]`

Case-insensitive substring/trigram-ish matching over table names, column
names, and comments, ranked (exact > prefix > substring; name >
comment). Returns `{kind, schema, table, column?, comment?, score}`,
capped results.

## Phase 2: sampling

Statistics-based profiling (`profile`, `stats`) is intentionally out of
scope: it depends on `pg_stats` / `pg_stat_*`, whose visibility varies
with role privileges and whose columns vary by Postgres version, and
stale or missing stats look like "empty table." Agents can run their own
aggregate queries through `dbcli query` and get a clear Postgres error if
something fails.

### 2.1 `sample` improvements

`--random` (TABLESAMPLE), `--columns a,b,c` (validated against catalog,
quoted), `--max-cell-bytes N` truncating long text/JSON/bytea with a
`…[+N bytes]` marker.

## Phase 3: better query feedback

### 3.1 `dbcli validate "<SQL>"`

Classifier + `PREPARE`/`EXPLAIN` (no execution): reports syntax/semantic
errors, result column names/types, and the planner's estimated
cost/rows.

`--max-cost N` on `query`/`explain` to refuse plans above a cost
threshold (opt-in guard against expensive queries).

### 3.2 Structured, actionable errors

Include Postgres `code` (SQLSTATE), `position`, and `hint`. For
undefined column/table (42703/42P01), add "did you mean" suggestions
using edit distance against catalog names. Additive fields on the
existing error envelope: `pg_code`, `position`, `suggestions`.

### 3.3 Pagination and truncation metadata

When `truncated: true`, include `limit_applied`, `hard_cap`, and a short
`hint` ("add ORDER BY and keyset condition, or aggregate").

### 3.4 Output shaping

`--format ndjson|csv` and `--compact` (columnar: `columns` once, `rows`
as arrays) to eliminate repeated keys on wide results; also applies to
`sample`.

## Phase 4: agent integration

### 4.1 `dbcli mcp` (stdio MCP server)

Exposes `query`, `explain`, `overview`, `describe`, `relations`,
`join_path`, `search`, `validate` as MCP tools with JSON schemas, so
agents call them natively with no shell-out. Reuses the same command
logic through a shared internal service layer (refactor `cmd/*` so
handlers return data and the CLI/MCP layers do the I/O).

Profile selection via server flag/env; no DSNs are ever returned to the
client.

### 4.2 Schema cache + agent docs

`dbcli cache refresh|show`: stores overview output on disk under
`~/.cache/dbcli/<profile>/`, keyed by a catalog fingerprint (e.g. hash of
`pg_class`/`pg_attribute` oids + `pg_stat` DDL counters) so unchanged
databases skip re-introspection.

`dbcli agent-doc [--profile NAME]`: generates a `SCHEMA.md`/`AGENTS.md`
snippet (tables, join paths, enum values, gotchas) for dropping into a
repo's agent instructions.

## Phase 5 (later): more engines and conveniences

SQLite driver (local agent workflows), then MySQL, behind the existing
`driver` interface (needs per-engine classifier + read-only mechanics).

Named saved queries (`queries.env`-style, parameterized, still routed
through the read-only validator).

## Suggested delivery order

Each step ships as its own independently reviewable branch/PR.

1. Phase 0: correctness fixes + richer `describe`. **Done.**
2. 1.1 `overview` (+ `ddl` format).
3. 1.2 `relations` / `join-path`, 1.3 `search`.
4. 2.1 `sample` improvements.
5. 3.1-3.4 (`validate`, structured errors, truncation hints, compact
   output).
6. 4.1 MCP server (includes the service-layer refactor).
7. 4.2 cache + `agent-doc`.
8. Phase 5 as demand dictates.

## Cross-cutting work for every step

- Update `main.go`'s `printUsage`, `doc.go`, and the README command list
  and `--help` block; add a worked example with real captured output.
- Tests: unit tests for pure logic (graph, ranking, formatting, edit
  distance); Postgres-backed tests for catalog queries, including edge
  cases (composite FKs, cross-schema FKs, partitioned tables, enums,
  views, empty schemas).
- Safety regression tests: new commands must fail closed against
  injection via table/column/schema names and respect timeout and row
  caps.
- CI (`.github/workflows/ci.yml`) stays green across the Postgres
  matrix; no new runtime dependencies unless needed (MCP in 4.1 may add
  one SDK dependency, to be decided then).

## Open decisions

1. **Default output shape:** new commands compact by default, existing
   commands unchanged (proposed). Confirm when Phase 1 starts.
2. **MCP:** official Go SDK vs. a minimal hand-rolled stdio JSON-RPC
   server (fewer dependencies, more maintenance).
3. **Table caps:** default `--max-tables` for `overview` (proposed 200).
