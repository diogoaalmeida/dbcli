# dbcli: LLM-agent discovery features, implementation plan

## Goal

Move database discovery work out of the agent's context and into Go. Today an
agent explores an unfamiliar database through many round trips (`schemas` →
`schema` → `describe` × N → `sample` × N → trial-and-error queries). Each step
costs tokens and latency. The goal is to answer common discovery questions in
**one call, in compact form**, while keeping every existing safety guarantee
(read-only classifier + read-only transaction, timeout, row cap, no raw
identifier interpolation).

## Principles

1. **Safety is non-negotiable.** Every new command runs through
   `beginReadOnlyTimeoutTx` and the hard row cap. Identifiers are never
   string-interpolated; they are checked against the catalog and quoted with
   `pgx.Identifier`.
2. **Backwards compatible.** The existing JSON envelopes (`ok`, `data`,
   `error`, `code`) and field names don't change. New output shapes are opt-in
   (`--format`, `--compact`) or new commands. New fields on existing results
   are additive only.
3. **Token-efficient by default for new commands.** Prefer compact text (DDL)
   or columnar JSON where it helps; truncate large values with explicit
   markers.
4. **Engine-neutral interface.** New capabilities are added to
   `driver.Conn` with engine-neutral types; Postgres is the only
   implementation for now.
5. **Tests for each change**, following the existing pattern (unit tests for
   pure logic; Postgres-backed tests where the repo already uses them).

## Phase 0: Correctness fixes to existing introspection

These are bugs/gaps that later phases build on.

| # | Item | Detail |
|---|------|--------|
| 0.1 | Fix composite/cross-schema FKs | `foreignKeysSQL` joins `information_schema` views on `constraint_name` only, producing a cross product for composite FKs and ambiguity across schemas. Rewrite using `pg_constraint` (`conkey`/`confkey` with `unnest ... WITH ORDINALITY`). Return `ref_schema` in addition to `ref_table`, and group columns per constraint (`columns`/`ref_columns`) while keeping the existing single-column fields populated for compatibility. |
| 0.2 | Primary key and constraints in `describe` | Add `primary_key` (ordered column list), plus `unique` and `check` constraints (name, definition). |
| 0.3 | Comments | Add table comment and per-column comment (`obj_description` / `col_description`). |
| 0.4 | Enum values | For columns of enum type, add `enum_values`. |
| 0.5 | View definitions | For views/matviews, include `definition` (`pg_get_viewdef`). |
| 0.6 | Partitioned & foreign tables | Include `relkind` `'p'` (and `'f'`) in `schema` listings with an accurate `kind`. |
| 0.7 | Reverse FKs | Add `referenced_by` to `describe` (tables whose FKs point here). |
| 0.8 | Stale-stats signal | `estimated_rows` of -1/0 from `reltuples` means "never analyzed"; surface `stats_known: false` rather than a misleading 0. |

Files: `internal/driver/driver.go`, `internal/postgres/schema.go`,
`internal/postgres/schema_test.go`, `cmd/describe.go`, README.

## Phase 1: One-call discovery (biggest token savings)

### 1.1 `dbcli overview [--schema X | --all-schemas] [--format json|ddl]`
- Returns every table/view in scope with columns, PK, FKs, comments, enum
  values, and estimated rows in a single call.
- `--format ddl` emits `CREATE TABLE`-style text (compact and LLM-friendly)
  including FK references and comments as SQL comments.
- Implementation: bulk catalog queries (one per object kind, not one per
  table) to avoid N+1; bounded by a table cap (`--max-tables`, default e.g.
  200) with `truncated` and a hint to narrow by `--schema` / `--tables`.
- New `Conn` method: `Overview(ctx, schema, opts) (*Overview, error)`.

### 1.2 `dbcli relations [--schema X] [--format json|mermaid]`
- FK graph as edges (`from_table.col → to_table.col`); `mermaid` output for
  an ER diagram.
- `dbcli join-path <from> <to> [--max-depth N]`: BFS over the FK graph
  (undirected, shortest path), returning the table chain plus ready-made
  `JOIN ... ON ...` clauses. Pure Go graph logic over the edges from 0.1, so it
  is unit-testable without a database.

### 1.3 `dbcli search <term> [--schema X] [--kind table|column|comment]`
- Case-insensitive substring/trigram-ish matching over table names, column
  names, and comments, ranked (exact > prefix > substring; name > comment).
- Returns `{kind, schema, table, column?, comment?, score}`; capped results.

## Phase 2: Sampling

Statistics-based profiling (`profile`, `stats`) is intentionally **out of
scope**: it depends on `pg_stats` / `pg_stat_*`, whose visibility varies with
role privileges and whose columns vary by Postgres version, and stale or
missing stats look like "empty table". Agents can run their own aggregate
queries through `dbcli query` and get a clear Postgres error if something
fails.

### 2.1 `sample` improvements
- `--random` (TABLESAMPLE), `--columns a,b,c` (validated against catalog,
  quoted), `--max-cell-bytes N` truncating long text/JSON/bytea with a
  `…[+N bytes]` marker.

## Phase 3: Better query feedback

### 3.1 `dbcli validate "<SQL>"`
- Classifier + `PREPARE`/`EXPLAIN` (no execution): reports syntax/semantic
  errors, result column names/types, and the planner's estimated cost/rows.
- `--max-cost N` on `query`/`explain` to refuse plans above a cost
  threshold (opt-in guard against expensive queries).

### 3.2 Structured, actionable errors
- Include Postgres `code` (SQLSTATE), `position`, and `hint`. For
  undefined column/table (42703/42P01), add "did you mean" suggestions using
  edit distance against catalog names.
- Additive fields on the existing error envelope: `pg_code`, `position`,
  `suggestions`.

### 3.3 Pagination and truncation metadata
- When `truncated: true`, include `limit_applied`, `hard_cap`, and a short
  `hint` ("add ORDER BY and keyset condition, or aggregate").

### 3.4 Output shaping
- `--format ndjson|csv` and `--compact` (columnar: `columns` once, `rows`
  as arrays) to eliminate repeated keys on wide results; also applies to
  `sample`.

## Phase 4: Agent integration

### 4.1 `dbcli mcp` (stdio MCP server)
- Exposes `query`, `explain`, `overview`, `describe`, `relations`,
  `join_path`, `search`, `validate` as MCP tools with JSON
  schemas, so agents call them natively with no shell-out. Reuses the same
  command logic through a shared internal service layer (refactor `cmd/*` so
  handlers return data and the CLI/MCP layers do the I/O).
- Profile selection via server flag/env; no DSNs are ever returned to the
  client.

### 4.2 Schema cache + agent docs
- `dbcli cache refresh|show`: stores overview output on disk under
  `~/.cache/dbcli/<profile>/`, keyed by a catalog fingerprint
  (e.g. hash of `pg_class`/`pg_attribute` oids + `pg_stat` DDL counters) so
  unchanged databases skip re-introspection.
- `dbcli agent-doc [--profile NAME]`: generates a `SCHEMA.md`/`AGENTS.md`
  snippet (tables, join paths, enum values, gotchas) for dropping into a
  repo's agent instructions.

## Phase 5 (later): More engines and conveniences
- SQLite driver (local agent workflows), then MySQL, behind the existing
  `driver` interface (needs per-engine classifier + read-only mechanics).
- Named saved queries (`queries.env`-style, parameterized, still routed
  through the read-only validator).

## Suggested delivery order

Each step is an independently reviewable commit/PR.

1. Phase 0 (0.1–0.8): correctness fixes + richer `describe`.
2. 1.1 `overview` (+ `ddl` format).
3. 1.2 `relations` / `join-path`, 1.3 `search`.
4. 2.1 `sample` improvements.
5. 3.1–3.4 (`validate`, structured errors, truncation hints, compact output).
6. 4.1 MCP server (includes the service-layer refactor).
7. 4.2 cache + `agent-doc`.
8. Phase 5 as demand dictates.

## Cross-cutting work for every step
- Update `main.go` `printUsage`, `doc.go`, and the README command list and
  `--help` block; add a worked example with real captured output.
- Tests: unit tests for pure logic (graph, ranking, formatting, edit
  distance); Postgres-backed tests for catalog queries, including edge cases
  (composite FKs, cross-schema FKs, partitioned tables, enums, views, empty
  schemas).
- Safety regression tests: new commands must fail closed against injection
  via table/column/schema names and respect timeout and row caps.
- CI (`.github/workflows/ci.yml`) stays green; no new runtime dependencies
  unless needed (MCP in 4.1 may add one SDK dependency, to be decided then).

## Open decisions
1. **Default output shape:** new commands compact by default, existing
   commands unchanged (proposed). Confirm.
2. **MCP:** official Go SDK vs. a minimal hand-rolled stdio JSON-RPC server
   (fewer dependencies, more maintenance).
3. **Table caps:** default `--max-tables` for `overview` (proposed 200).
