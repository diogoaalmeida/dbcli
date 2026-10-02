package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/diogoaalmeida/dbcli/internal/driver"
)

const listSchemaSQL = `
SELECT n.nspname AS schema,
       c.relname AS name,
       CASE c.relkind
         WHEN 'r' THEN 'table'
         WHEN 'v' THEN 'view'
         WHEN 'm' THEN 'materialized_view'
         WHEN 'p' THEN 'partitioned_table'
         WHEN 'f' THEN 'foreign_table'
         ELSE c.relkind::text
       END AS kind,
       GREATEST(c.reltuples, 0)::bigint AS estimated_rows,
       (s.last_vacuum IS NOT NULL OR s.last_autovacuum IS NOT NULL
        OR s.last_analyze IS NOT NULL OR s.last_autoanalyze IS NOT NULL) AS stats_known
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
LEFT JOIN pg_catalog.pg_stat_user_tables s ON s.relid = c.oid
WHERE c.relkind IN ('r', 'v', 'm', 'p', 'f') AND n.nspname = $1
ORDER BY c.relname
LIMIT $2`

const listSchemasSQL = `
SELECT n.nspname AS name,
       count(c.oid) FILTER (WHERE c.relkind IN ('r', 'v', 'm', 'p', 'f')) AS table_count
FROM pg_catalog.pg_namespace n
LEFT JOIN pg_catalog.pg_class c ON c.relnamespace = n.oid
WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
  AND n.nspname NOT LIKE 'pg\_toast%' ESCAPE '\'
  AND n.nspname NOT LIKE 'pg\_temp\_%' ESCAPE '\'
GROUP BY n.nspname
ORDER BY n.nspname
LIMIT $1`

// ListSchemas lists every non-system schema in the database, with a count
// of the tables/views/materialized views/partitioned/foreign tables each
// one holds. It's the entry point for exploring an unfamiliar database:
// run this first, then ListSchema(name) to see what's inside one of the
// schemas it reports. Like every other query dbcli runs, this goes
// through a read-only, timeout-bounded transaction and a hard row cap — a
// schema-per-tenant database can have as many schemas as a data table has
// rows.
func (c *conn) ListSchemas(ctx context.Context, opts driver.QueryOptions) ([]driver.SchemaInfo, error) {
	tx, err := c.beginReadOnlyTimeoutTx(ctx, opts.TimeoutSeconds)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, listSchemasSQL, clampLimit(opts.Limit))
	if err != nil {
		return nil, fmt.Errorf("list schemas: %w", err)
	}
	defer rows.Close()

	var schemas []driver.SchemaInfo
	for rows.Next() {
		var s driver.SchemaInfo
		if err := rows.Scan(&s.Name, &s.TableCount); err != nil {
			return nil, fmt.Errorf("scan schema row: %w", err)
		}
		schemas = append(schemas, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list schemas iteration: %w", err)
	}
	return schemas, nil
}

func (c *conn) ListSchema(ctx context.Context, schema string, opts driver.QueryOptions) ([]driver.TableInfo, error) {
	if schema == "" {
		schema = "public"
	}

	tx, err := c.beginReadOnlyTimeoutTx(ctx, opts.TimeoutSeconds)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, listSchemaSQL, schema, clampLimit(opts.Limit))
	if err != nil {
		return nil, fmt.Errorf("list schema: %w", err)
	}
	defer rows.Close()

	var tables []driver.TableInfo
	for rows.Next() {
		var t driver.TableInfo
		if err := rows.Scan(&t.Schema, &t.Name, &t.Kind, &t.EstimatedRows, &t.StatsKnown); err != nil {
			return nil, fmt.Errorf("scan table row: %w", err)
		}
		tables = append(tables, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list schema iteration: %w", err)
	}
	return tables, nil
}

// columnsSQL joins to pg_attribute by column name (not
// information_schema.columns' ordinal_position) to get the real attnum
// for col_description: ordinal_position is a logical, dropped-columns-
// excluded position that can diverge from attnum on a table that has
// ever had a column dropped, which would point the comment lookup at the
// wrong column.
const columnsSQL = `
SELECT c.column_name,
       c.data_type,
       c.udt_schema,
       c.udt_name,
       (c.is_nullable = 'YES') AS nullable,
       c.column_default,
       col_description(pgc.oid, pga.attnum) AS comment
FROM information_schema.columns c
JOIN pg_catalog.pg_class pgc ON pgc.relname = c.table_name
JOIN pg_catalog.pg_namespace pgn ON pgn.oid = pgc.relnamespace AND pgn.nspname = c.table_schema
JOIN pg_catalog.pg_attribute pga ON pga.attrelid = pgc.oid AND pga.attname = c.column_name
WHERE c.table_schema = $1 AND c.table_name = $2
ORDER BY c.ordinal_position
LIMIT $3`

const enumValuesSQL = `
SELECT e.enumlabel
FROM pg_catalog.pg_type t
JOIN pg_catalog.pg_enum e ON e.enumtypid = t.oid
JOIN pg_catalog.pg_namespace n ON n.oid = t.typnamespace
WHERE n.nspname = $1 AND t.typname = $2
ORDER BY e.enumsortorder`

const indexesSQL = `
SELECT i.relname AS index_name,
       array_agg(a.attname ORDER BY array_position(ix.indkey, a.attnum)) AS columns,
       ix.indisunique,
       ix.indisprimary
FROM pg_catalog.pg_class t
JOIN pg_catalog.pg_namespace n ON n.oid = t.relnamespace
JOIN pg_catalog.pg_index ix ON ix.indrelid = t.oid
JOIN pg_catalog.pg_class i ON i.oid = ix.indexrelid
JOIN pg_catalog.pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
WHERE n.nspname = $1 AND t.relname = $2
GROUP BY i.relname, ix.indisunique, ix.indisprimary
ORDER BY i.relname
LIMIT $3`

// foreignKeysSQL finds the foreign keys owned by one table, pairing each
// constraint's local and referenced columns by ordinal position
// (conkey/confkey, unnested WITH ORDINALITY and joined on that ordinal) —
// not by an incidental join on constraint_name alone, which silently
// cross-joins a composite FK's columns. One row per constraint, with the
// full ordered column lists; DescribeTable expands each into one
// driver.ForeignKeyInfo per column pair.
const foreignKeysSQL = `
SELECT con.conname AS constraint_name,
       rn.nspname AS ref_schema,
       rc.relname AS ref_table,
       array_agg(att_child.attname ORDER BY ck.ord) AS columns,
       array_agg(att_parent.attname ORDER BY ck.ord) AS ref_columns
FROM pg_catalog.pg_constraint con
JOIN pg_catalog.pg_class c ON c.oid = con.conrelid
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
JOIN pg_catalog.pg_class rc ON rc.oid = con.confrelid
JOIN pg_catalog.pg_namespace rn ON rn.oid = rc.relnamespace
JOIN LATERAL unnest(con.conkey) WITH ORDINALITY AS ck(attnum, ord) ON true
JOIN LATERAL unnest(con.confkey) WITH ORDINALITY AS cfk(attnum, ord) ON cfk.ord = ck.ord
JOIN pg_catalog.pg_attribute att_child ON att_child.attrelid = con.conrelid AND att_child.attnum = ck.attnum
JOIN pg_catalog.pg_attribute att_parent ON att_parent.attrelid = con.confrelid AND att_parent.attnum = cfk.attnum
WHERE con.contype = 'f' AND n.nspname = $1 AND c.relname = $2
GROUP BY con.conname, rn.nspname, rc.relname
ORDER BY con.conname
LIMIT $3`

// reverseForeignKeysSQL is foreignKeysSQL with the direction flipped: it
// finds foreign keys owned by *other* tables that reference this one.
const reverseForeignKeysSQL = `
SELECT con.conname AS constraint_name,
       n.nspname AS schema,
       c.relname AS "table",
       array_agg(att_child.attname ORDER BY ck.ord) AS columns,
       array_agg(att_parent.attname ORDER BY ck.ord) AS ref_columns
FROM pg_catalog.pg_constraint con
JOIN pg_catalog.pg_class c ON c.oid = con.conrelid
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
JOIN pg_catalog.pg_class rc ON rc.oid = con.confrelid
JOIN pg_catalog.pg_namespace rn ON rn.oid = rc.relnamespace
JOIN LATERAL unnest(con.conkey) WITH ORDINALITY AS ck(attnum, ord) ON true
JOIN LATERAL unnest(con.confkey) WITH ORDINALITY AS cfk(attnum, ord) ON cfk.ord = ck.ord
JOIN pg_catalog.pg_attribute att_child ON att_child.attrelid = con.conrelid AND att_child.attnum = ck.attnum
JOIN pg_catalog.pg_attribute att_parent ON att_parent.attrelid = con.confrelid AND att_parent.attnum = cfk.attnum
WHERE con.contype = 'f' AND rn.nspname = $1 AND rc.relname = $2
GROUP BY con.conname, n.nspname, c.relname
ORDER BY con.conname
LIMIT $3`

const primaryKeySQL = `
SELECT a.attname
FROM pg_catalog.pg_constraint con
JOIN pg_catalog.pg_class c ON c.oid = con.conrelid
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
JOIN LATERAL unnest(con.conkey) WITH ORDINALITY AS ck(attnum, ord) ON true
JOIN pg_catalog.pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = ck.attnum
WHERE con.contype = 'p' AND n.nspname = $1 AND c.relname = $2
ORDER BY ck.ord
LIMIT $3`

// uniqueAndCheckConstraintsSQL covers unique ('u') and check ('c')
// constraints in one query; DescribeTable splits the rows by contype.
const uniqueAndCheckConstraintsSQL = `
SELECT con.conname, pg_get_constraintdef(con.oid) AS definition, con.contype
FROM pg_catalog.pg_constraint con
JOIN pg_catalog.pg_class c ON c.oid = con.conrelid
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE con.contype IN ('u', 'c') AND n.nspname = $1 AND c.relname = $2
ORDER BY con.conname
LIMIT $3`

const tableCommentSQL = `
SELECT obj_description(c.oid, 'pg_class')
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = $1 AND c.relname = $2`

// viewDefinitionSQL filters to relkind IN ('v', 'm') itself, so it's
// always safe to run: it returns no row for an ordinary table.
const viewDefinitionSQL = `
SELECT pg_get_viewdef(c.oid, true)
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = $1 AND c.relname = $2 AND c.relkind IN ('v', 'm')`

const tableExistsSQL = `
SELECT EXISTS (
  SELECT 1 FROM information_schema.tables
  WHERE table_schema = $1 AND table_name = $2
)`

// DescribeTable runs all of its queries (existence check, columns,
// indexes, foreign keys, reverse foreign keys, primary key, unique/check
// constraints, comments, view definition) inside one read-only,
// timeout-bounded transaction, so the whole operation shares a single
// deadline instead of each query getting its own.
func (c *conn) DescribeTable(ctx context.Context, schema, table string, opts driver.QueryOptions) (*driver.TableDescription, error) {
	if schema == "" {
		schema = "public"
	}

	tx, err := c.beginReadOnlyTimeoutTx(ctx, opts.TimeoutSeconds)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, tableExistsSQL, schema, table).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check table exists: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("table %s.%s not found", schema, table)
	}

	limit := clampLimit(opts.Limit)
	desc := &driver.TableDescription{Schema: schema, Table: table}

	if desc.Columns, err = describeColumns(ctx, tx, schema, table, limit); err != nil {
		return nil, err
	}
	if desc.Indexes, err = describeIndexes(ctx, tx, schema, table, limit); err != nil {
		return nil, err
	}
	if desc.ForeignKeys, err = describeForeignKeys(ctx, tx, schema, table, limit); err != nil {
		return nil, err
	}
	if desc.ReferencedBy, err = describeReverseForeignKeys(ctx, tx, schema, table, limit); err != nil {
		return nil, err
	}
	if desc.PrimaryKey, err = describePrimaryKey(ctx, tx, schema, table, limit); err != nil {
		return nil, err
	}
	if desc.UniqueConstraints, desc.CheckConstraints, err = describeConstraints(ctx, tx, schema, table, limit); err != nil {
		return nil, err
	}
	if desc.Comment, err = describeTableComment(ctx, tx, schema, table); err != nil {
		return nil, err
	}
	if desc.ViewDefinition, err = describeViewDefinition(ctx, tx, schema, table); err != nil {
		return nil, err
	}

	return desc, nil
}

func describeColumns(ctx context.Context, tx pgx.Tx, schema, table string, limit int) ([]driver.ColumnInfo, error) {
	rows, err := tx.Query(ctx, columnsSQL, schema, table, limit)
	if err != nil {
		return nil, fmt.Errorf("describe columns: %w", err)
	}
	defer rows.Close()

	type rawColumn struct {
		col       driver.ColumnInfo
		udtSchema string
		udtName   string
	}
	var raw []rawColumn
	for rows.Next() {
		var rc rawColumn
		if err := rows.Scan(&rc.col.Name, &rc.col.Type, &rc.udtSchema, &rc.udtName, &rc.col.Nullable, &rc.col.Default, &rc.col.Comment); err != nil {
			return nil, fmt.Errorf("scan column: %w", err)
		}
		raw = append(raw, rc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("describe columns iteration: %w", err)
	}

	// Enum labels are looked up once per distinct (schema, type), not once
	// per column, since several columns can share an enum type.
	type enumKey struct{ schema, name string }
	cache := map[enumKey][]string{}
	columns := make([]driver.ColumnInfo, 0, len(raw))
	for _, rc := range raw {
		if rc.col.Type == "USER-DEFINED" {
			key := enumKey{rc.udtSchema, rc.udtName}
			values, cached := cache[key]
			if !cached {
				var err error
				values, err = describeEnumValues(ctx, tx, rc.udtSchema, rc.udtName)
				if err != nil {
					return nil, err
				}
				cache[key] = values
			}
			rc.col.EnumValues = values
		}
		columns = append(columns, rc.col)
	}
	return columns, nil
}

func describeEnumValues(ctx context.Context, tx pgx.Tx, typeSchema, typeName string) ([]string, error) {
	rows, err := tx.Query(ctx, enumValuesSQL, typeSchema, typeName)
	if err != nil {
		return nil, fmt.Errorf("describe enum values: %w", err)
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, fmt.Errorf("scan enum label: %w", err)
		}
		values = append(values, label)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("describe enum values iteration: %w", err)
	}
	return values, nil
}

func describeIndexes(ctx context.Context, tx pgx.Tx, schema, table string, limit int) ([]driver.IndexInfo, error) {
	rows, err := tx.Query(ctx, indexesSQL, schema, table, limit)
	if err != nil {
		return nil, fmt.Errorf("describe indexes: %w", err)
	}
	defer rows.Close()

	var indexes []driver.IndexInfo
	for rows.Next() {
		var idx driver.IndexInfo
		if err := rows.Scan(&idx.Name, &idx.Columns, &idx.Unique, &idx.Primary); err != nil {
			return nil, fmt.Errorf("scan index: %w", err)
		}
		indexes = append(indexes, idx)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("describe indexes iteration: %w", err)
	}
	return indexes, nil
}

// expandForeignKeyRow turns one foreignKeysSQL/reverseForeignKeysSQL row
// (one constraint, with its full ordered column lists) into one
// driver.ForeignKeyInfo per column pair, so a single-column FK's shape
// matches exactly what every version of this field has always returned,
// while a composite FK now produces correctly-paired rows instead of a
// cross join.
func describeForeignKeys(ctx context.Context, tx pgx.Tx, schema, table string, limit int) ([]driver.ForeignKeyInfo, error) {
	rows, err := tx.Query(ctx, foreignKeysSQL, schema, table, limit)
	if err != nil {
		return nil, fmt.Errorf("describe foreign keys: %w", err)
	}
	defer rows.Close()

	var fks []driver.ForeignKeyInfo
	for rows.Next() {
		var constraintName, refSchema, refTable string
		var columns, refColumns []string
		if err := rows.Scan(&constraintName, &refSchema, &refTable, &columns, &refColumns); err != nil {
			return nil, fmt.Errorf("scan foreign key: %w", err)
		}
		for i := range columns {
			fks = append(fks, driver.ForeignKeyInfo{
				Column:         columns[i],
				RefTable:       refTable,
				RefColumn:      refColumns[i],
				ConstraintName: constraintName,
				RefSchema:      refSchema,
				Columns:        columns,
				RefColumns:     refColumns,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("describe foreign keys iteration: %w", err)
	}
	return fks, nil
}

func describeReverseForeignKeys(ctx context.Context, tx pgx.Tx, schema, table string, limit int) ([]driver.ReferencingForeignKey, error) {
	rows, err := tx.Query(ctx, reverseForeignKeysSQL, schema, table, limit)
	if err != nil {
		return nil, fmt.Errorf("describe reverse foreign keys: %w", err)
	}
	defer rows.Close()

	var refs []driver.ReferencingForeignKey
	for rows.Next() {
		var r driver.ReferencingForeignKey
		if err := rows.Scan(&r.ConstraintName, &r.Schema, &r.Table, &r.Columns, &r.RefColumns); err != nil {
			return nil, fmt.Errorf("scan reverse foreign key: %w", err)
		}
		refs = append(refs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("describe reverse foreign keys iteration: %w", err)
	}
	return refs, nil
}

func describePrimaryKey(ctx context.Context, tx pgx.Tx, schema, table string, limit int) ([]string, error) {
	rows, err := tx.Query(ctx, primaryKeySQL, schema, table, limit)
	if err != nil {
		return nil, fmt.Errorf("describe primary key: %w", err)
	}
	defer rows.Close()

	var pk []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, fmt.Errorf("scan primary key column: %w", err)
		}
		pk = append(pk, col)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("describe primary key iteration: %w", err)
	}
	return pk, nil
}

func describeConstraints(ctx context.Context, tx pgx.Tx, schema, table string, limit int) (unique, check []driver.ConstraintInfo, err error) {
	rows, err := tx.Query(ctx, uniqueAndCheckConstraintsSQL, schema, table, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("describe constraints: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ci driver.ConstraintInfo
		var contype string
		if err := rows.Scan(&ci.Name, &ci.Definition, &contype); err != nil {
			return nil, nil, fmt.Errorf("scan constraint: %w", err)
		}
		switch contype {
		case "u":
			unique = append(unique, ci)
		case "c":
			check = append(check, ci)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("describe constraints iteration: %w", err)
	}
	return unique, check, nil
}

func describeTableComment(ctx context.Context, tx pgx.Tx, schema, table string) (*string, error) {
	var comment *string
	if err := tx.QueryRow(ctx, tableCommentSQL, schema, table).Scan(&comment); err != nil {
		return nil, fmt.Errorf("describe table comment: %w", err)
	}
	return comment, nil
}

func describeViewDefinition(ctx context.Context, tx pgx.Tx, schema, table string) (*string, error) {
	var def string
	err := tx.QueryRow(ctx, viewDefinitionSQL, schema, table).Scan(&def)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("describe view definition: %w", err)
	}
	return &def, nil
}

// Sample validates that schema.table exists, then runs a safely quoted
// "SELECT * FROM <table> LIMIT N" — the table name is checked against
// information_schema first and identifier-quoted via pgx.Identifier, never
// string-interpolated raw.
func (c *conn) Sample(ctx context.Context, schema, table string, opts driver.QueryOptions) (*driver.QueryResult, error) {
	if schema == "" {
		schema = "public"
	}

	tx, err := c.beginReadOnlyTimeoutTx(ctx, opts.TimeoutSeconds)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, tableExistsSQL, schema, table).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check table exists: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("table %s.%s not found", schema, table)
	}

	ident := pgx.Identifier{schema, table}.Sanitize()
	limit := clampLimit(opts.Limit)
	sql := fmt.Sprintf("SELECT * FROM %s LIMIT %d", ident, limit)
	return c.runInTx(ctx, tx, sql, limit, sql)
}
