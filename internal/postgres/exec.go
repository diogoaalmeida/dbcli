package postgres

import (
	"context"
	sqldriver "database/sql/driver"
	"encoding/base64"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diogoaalmeida/dbcli/internal/driver"
)

const (
	defaultTimeoutSeconds = 5
	defaultRowLimit       = 500
	maxRowLimit           = 5000
)

type conn struct {
	pool *pgxpool.Pool
}

func (c *conn) Close(ctx context.Context) error {
	c.pool.Close()
	return nil
}

// Query validates sql, wraps it in a hard row-limit ceiling, and runs it
// inside a read-only, timeout-bounded transaction.
func (c *conn) Query(ctx context.Context, sql string, opts driver.QueryOptions) (*driver.QueryResult, error) {
	validated, err := ValidateQuery(sql)
	if err != nil {
		return nil, err
	}
	limit := clampLimit(opts.Limit)
	wrapped := WrapWithLimit(validated, limit)
	return c.run(ctx, wrapped, opts, limit, validated)
}

// Explain validates sql, prefixes it with EXPLAIN (or EXPLAIN ANALYZE), and
// runs it inside the same read-only, timeout-bounded transaction. No row
// limit is applied — EXPLAIN's output rows are plan lines, not data rows.
func (c *conn) Explain(ctx context.Context, sql string, opts driver.QueryOptions) (*driver.QueryResult, error) {
	validated, err := ValidateQuery(sql)
	if err != nil {
		return nil, err
	}

	explainSQL := validated
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(validated)), "explain") {
		if opts.Analyze {
			explainSQL = "EXPLAIN ANALYZE " + validated
		} else {
			explainSQL = "EXPLAIN " + validated
		}
	}

	return c.run(ctx, explainSQL, opts, 0, validated)
}

func (c *conn) run(ctx context.Context, sql string, opts driver.QueryOptions, limitForTruncation int, recordQuery string) (*driver.QueryResult, error) {
	timeoutSeconds := opts.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultTimeoutSeconds
	}

	tx, err := c.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("begin read-only transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", timeoutSeconds*1000)); err != nil {
		return nil, fmt.Errorf("set statement_timeout: %w", err)
	}

	start := time.Now()
	rows, err := tx.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	fieldDescs := rows.FieldDescriptions()
	columns := make([]driver.Column, len(fieldDescs))
	typeMap := tx.Conn().TypeMap()
	for i, fd := range fieldDescs {
		columns[i] = driver.Column{Name: string(fd.Name), Type: typeName(typeMap, fd.DataTypeOID)}
	}

	var resultRows []map[string]any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("read row: %w", err)
		}
		rowMap := make(map[string]any, len(columns))
		for i, col := range columns {
			rowMap[col.Name] = marshalValue(values[i])
		}
		resultRows = append(resultRows, rowMap)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	duration := time.Since(start)

	return &driver.QueryResult{
		Columns:    columns,
		Rows:       resultRows,
		RowCount:   len(resultRows),
		Truncated:  limitForTruncation > 0 && len(resultRows) >= limitForTruncation,
		DurationMS: duration.Milliseconds(),
		Query:      recordQuery,
	}, nil
}

func clampLimit(requested int) int {
	limit := requested
	if limit <= 0 {
		limit = defaultRowLimit
	}
	if limit > maxRowLimit {
		limit = maxRowLimit
	}
	return limit
}

func typeName(tm *pgtype.Map, oid uint32) string {
	if t, ok := tm.TypeForOID(oid); ok {
		return t.Name
	}
	return fmt.Sprintf("oid(%d)", oid)
}

// marshalValue converts a pgx-decoded Go value into a JSON-safe value per
// dbcli's output rules: NULL stays null, all numeric kinds become strings
// (avoids float precision loss on numeric/bigint for JSON consumers),
// timestamps become RFC3339, and byte slices become base64.
func marshalValue(v any) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case []byte:
		return base64.StdEncoding.EncodeToString(val)
	case time.Time:
		return val.UTC().Format(time.RFC3339Nano)
	}

	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return nil
	}

	switch rv.Kind() {
	case reflect.Array:
		if rv.Len() == 16 && rv.Type().Elem().Kind() == reflect.Uint8 {
			// pgx decodes uuid columns into [16]byte, not a string.
			return formatUUID(rv)
		}
		fallthrough
	case reflect.Slice:
		out := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = marshalValue(rv.Index(i).Interface())
		}
		return out
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(rv.Float(), 'f', -1, 64)
	case reflect.Struct, reflect.Ptr:
		if valuer, ok := v.(sqldriver.Valuer); ok {
			if dv, err := valuer.Value(); err == nil {
				return marshalValue(dv)
			}
		}
		return fmt.Sprintf("%v", v)
	default:
		return v
	}
}

// formatUUID renders a [16]byte reflect.Value in canonical
// 8-4-4-4-12 hex form, e.g. "38f9d7d7-d4ed-4a1e-9c1a-2f6b7c8d9e0f".
func formatUUID(rv reflect.Value) string {
	var b [16]byte
	for i := range b {
		b[i] = byte(rv.Index(i).Uint())
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
