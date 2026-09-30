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
         ELSE c.relkind::text
       END AS kind,
       GREATEST(c.reltuples, 0)::bigint AS estimated_rows
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r', 'v', 'm') AND n.nspname = $1
ORDER BY c.relname
LIMIT $2`

const listSchemasSQL = `
SELECT n.nspname AS name,
       count(c.oid) FILTER (WHERE c.relkind IN ('r', 'v', 'm')) AS table_count
FROM pg_catalog.pg_namespace n
LEFT JOIN pg_catalog.pg_class c ON c.relnamespace = n.oid
WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
  AND n.nspname NOT LIKE 'pg\_toast%' ESCAPE '\'
  AND n.nspname NOT LIKE 'pg\_temp\_%' ESCAPE '\'
GROUP BY n.nspname
ORDER BY n.nspname
LIMIT $1`

// ListSchemas lists every non-system schema in the database, with a count
// of the tables/views/materialized views each one holds. It's the entry
// point for exploring an unfamiliar database: run this first, then
// ListSchema(name) to see what's inside one of the schemas it reports.
// Like every other query dbcli runs, this goes through a read-only,
// timeout-bounded transaction and a hard row cap — a schema-per-tenant
// database can have as many schemas as a data table has rows.
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
		if err := rows.Scan(&t.Schema, &t.Name, &t.Kind, &t.EstimatedRows); err != nil {
			return nil, fmt.Errorf("scan table row: %w", err)
		}
		tables = append(tables, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list schema iteration: %w", err)
	}
	return tables, nil
}

const columnsSQL = `
SELECT column_name, data_type, (is_nullable = 'YES') AS nullable, column_default
FROM information_schema.columns
WHERE table_schema = $1 AND table_name = $2
ORDER BY ordinal_position
LIMIT $3`

const indexesSQL = `
SELECT i.relname AS index_name,
       array_agg(a.attname ORDER BY array_position(ix.indkey, a.attnum)) AS columns,
       ix.indisunique
FROM pg_catalog.pg_class t
JOIN pg_catalog.pg_namespace n ON n.oid = t.relnamespace
JOIN pg_catalog.pg_index ix ON ix.indrelid = t.oid
JOIN pg_catalog.pg_class i ON i.oid = ix.indexrelid
JOIN pg_catalog.pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
WHERE n.nspname = $1 AND t.relname = $2
GROUP BY i.relname, ix.indisunique
ORDER BY i.relname
LIMIT $3`

const foreignKeysSQL = `
SELECT kcu.column_name, ccu.table_name AS ref_table, ccu.column_name AS ref_column
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
JOIN information_schema.constraint_column_usage ccu
  ON tc.constraint_name = ccu.constraint_name AND tc.table_schema = ccu.table_schema
WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_schema = $1 AND tc.table_name = $2
ORDER BY kcu.column_name
LIMIT $3`

const tableExistsSQL = `
SELECT EXISTS (
  SELECT 1 FROM information_schema.tables
  WHERE table_schema = $1 AND table_name = $2
)`

// DescribeTable runs all four of its queries (existence check, columns,
// indexes, foreign keys) inside one read-only, timeout-bounded transaction,
// so the whole operation shares a single deadline instead of each query
// getting its own.
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

	rows, err := tx.Query(ctx, columnsSQL, schema, table, limit)
	if err != nil {
		return nil, fmt.Errorf("describe columns: %w", err)
	}
	for rows.Next() {
		var col driver.ColumnInfo
		if err := rows.Scan(&col.Name, &col.Type, &col.Nullable, &col.Default); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan column: %w", err)
		}
		desc.Columns = append(desc.Columns, col)
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		return nil, fmt.Errorf("describe columns iteration: %w", rowsErr)
	}

	idxRows, err := tx.Query(ctx, indexesSQL, schema, table, limit)
	if err != nil {
		return nil, fmt.Errorf("describe indexes: %w", err)
	}
	for idxRows.Next() {
		var idx driver.IndexInfo
		if err := idxRows.Scan(&idx.Name, &idx.Columns, &idx.Unique); err != nil {
			idxRows.Close()
			return nil, fmt.Errorf("scan index: %w", err)
		}
		desc.Indexes = append(desc.Indexes, idx)
	}
	idxErr := idxRows.Err()
	idxRows.Close()
	if idxErr != nil {
		return nil, fmt.Errorf("describe indexes iteration: %w", idxErr)
	}

	fkRows, err := tx.Query(ctx, foreignKeysSQL, schema, table, limit)
	if err != nil {
		return nil, fmt.Errorf("describe foreign keys: %w", err)
	}
	for fkRows.Next() {
		var fk driver.ForeignKeyInfo
		if err := fkRows.Scan(&fk.Column, &fk.RefTable, &fk.RefColumn); err != nil {
			fkRows.Close()
			return nil, fmt.Errorf("scan foreign key: %w", err)
		}
		desc.ForeignKeys = append(desc.ForeignKeys, fk)
	}
	fkErr := fkRows.Err()
	fkRows.Close()
	if fkErr != nil {
		return nil, fmt.Errorf("describe foreign keys iteration: %w", fkErr)
	}

	return desc, nil
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
