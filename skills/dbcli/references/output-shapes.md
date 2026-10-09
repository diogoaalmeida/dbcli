# dbcli output shapes

Every dbcli command prints exactly one JSON object to stdout, except
`version` and `help`/`--help`/`-h`, which print plain text. Parse
stdout as JSON for every other command, success or failure.

## Envelopes

`query`, `explain`, and `sample` return a query envelope:

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

`schemas`, `schema`, and `describe` return a data envelope, where
`data` is the actual result (an array or an object depending on the
command):

```json
{"ok": true, "data": [{"name": "public", "table_count": 0}]}
```

Any command, on failure, returns an error envelope instead:

```json
{"ok": false, "error": "query rejected: only SELECT/WITH/EXPLAIN/TABLE statements are allowed", "code": "query_error"}
```

Check `code` before deciding how to react. `query_error` usually means
the classifier rejected a non-SELECT statement, or the query hit the
statement timeout — rephrase rather than retry unchanged.
`connection_error` means the DSN, profile, or driver couldn't be
resolved. `schema_error`/`describe_error` mean the schema or table
doesn't exist, or isn't visible to the connected role.

## Value conventions

- Numbers (`int4`, `int8`, `numeric`, etc.) are JSON strings, not JSON
  numbers — avoids precision loss on large integers and decimals.
  Parse them on the consuming side as needed.
- `timestamp`/`timestamptz` are RFC3339. `date` is a plain
  `"YYYY-MM-DD"` string with no time component — pgx decodes both into
  the same Go type, so dbcli has to know which column type it came
  from to format it correctly.
- `bytea` is base64.
- `uuid` is the canonical hyphenated string form, not a byte array.
- `macaddr` is a colon-separated string (`"08:00:2b:01:02:03"`), not a
  byte array.
- Range types (`numrange`, `int8range`, `daterange`, etc.) become
  `{"lower": ..., "upper": ..., "lower_type": "inclusive"|"exclusive"|"unbounded"|"empty", "upper_type": ..., "valid": true}`,
  with bounds following the same rules as their scalar type (so an
  `int8range`'s bounds are strings, a `daterange`'s bounds are
  date-only strings, etc).

## `schema`/`schemas` fields

Each table row includes `kind` (`"table"`, `"view"`,
`"materialized_view"`, `"partitioned_table"`, or `"foreign_table"`),
`estimated_rows`, and `stats_known`.

`stats_known` is `false` when a table has never been vacuumed or
analyzed — `estimated_rows` would otherwise look like a misleading 0.

**Partitioned-table caveat:** a partitioned table's parent row is
almost always `stats_known: false` with `estimated_rows: 0`, even when
its partitions hold real data. Autovacuum only analyzes leaf
partitions, never the parent relation itself (it has no storage of its
own). Sum the partitions' own `estimated_rows` (listed alongside the
parent in the same `schema` output) to get the real count — don't
report the parent's `estimated_rows: 0` as the table's actual size.

## `describe` fields

Always present (`null` when there's nothing to report, never an
absent key): `columns`, `indexes`, `foreign_keys`, `referenced_by`
(reverse foreign keys — other tables that reference this one).

Present only when applicable to the table (omitted otherwise, same
rule as a column's `enum_values`): `primary_key`,
`unique_constraints`/`check_constraints` (each `{"name": ..., "definition": ...}`),
table/column `comment`, and `view_definition` (for a view or
materialized view).

A column's `enum_values` is present only when that column is an enum
type (`"type": "USER-DEFINED"` in that case, with the enum's labels in
definition order).
