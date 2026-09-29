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
ORDER BY c.relname`

func (c *conn) ListSchema(ctx context.Context, schema string) ([]driver.TableInfo, error) {
	if schema == "" {
		schema = "public"
	}

	rows, err := c.pool.Query(ctx, listSchemaSQL, schema)
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
ORDER BY ordinal_position`

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
ORDER BY i.relname`

const foreignKeysSQL = `
SELECT kcu.column_name, ccu.table_name AS ref_table, ccu.column_name AS ref_column
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
JOIN information_schema.constraint_column_usage ccu
  ON tc.constraint_name = ccu.constraint_name AND tc.table_schema = ccu.table_schema
WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_schema = $1 AND tc.table_name = $2
ORDER BY kcu.column_name`

const tableExistsSQL = `
SELECT EXISTS (
  SELECT 1 FROM information_schema.tables
  WHERE table_schema = $1 AND table_name = $2
)`

func (c *conn) DescribeTable(ctx context.Context, schema, table string) (*driver.TableDescription, error) {
	if schema == "" {
		schema = "public"
	}

	var exists bool
	if err := c.pool.QueryRow(ctx, tableExistsSQL, schema, table).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check table exists: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("table %s.%s not found", schema, table)
	}

	desc := &driver.TableDescription{Schema: schema, Table: table}

	rows, err := c.pool.Query(ctx, columnsSQL, schema, table)
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

	idxRows, err := c.pool.Query(ctx, indexesSQL, schema, table)
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

	fkRows, err := c.pool.Query(ctx, foreignKeysSQL, schema, table)
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

	var exists bool
	if err := c.pool.QueryRow(ctx, tableExistsSQL, schema, table).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check table exists: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("table %s.%s not found", schema, table)
	}

	ident := pgx.Identifier{schema, table}.Sanitize()
	limit := clampLimit(opts.Limit)
	sql := fmt.Sprintf("SELECT * FROM %s LIMIT %d", ident, limit)
	return c.run(ctx, sql, opts, limit, sql)
}
