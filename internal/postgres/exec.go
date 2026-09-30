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

	"github.com/diogoaalmeida/dbcli/internal/driver"
)

const (
	defaultTimeoutSeconds = 5
	defaultRowLimit       = 500
	maxRowLimit           = 5000
)

type conn struct {
	pgxConn *pgx.Conn
}

func (c *conn) Close(ctx context.Context) error {
	return c.pgxConn.Close(ctx)
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

// beginReadOnlyTimeoutTx starts a read-only transaction with a per-session
// statement_timeout. Every query dbcli runs, including schema introspection,
// goes through this: it's both the second layer of the read-only defense
// (behind the SQL classifier) and the server-side timeout backstop.
func (c *conn) beginReadOnlyTimeoutTx(ctx context.Context, timeoutSeconds int) (pgx.Tx, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultTimeoutSeconds
	}

	tx, err := c.pgxConn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("begin read-only transaction: %w", err)
	}

	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", timeoutSeconds*1000)); err != nil {
		tx.Rollback(ctx)
		return nil, fmt.Errorf("set statement_timeout: %w", err)
	}
	return tx, nil
}

func (c *conn) run(ctx context.Context, sql string, opts driver.QueryOptions, limitForTruncation int, recordQuery string) (*driver.QueryResult, error) {
	tx, err := c.beginReadOnlyTimeoutTx(ctx, opts.TimeoutSeconds)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	return c.runInTx(ctx, tx, sql, limitForTruncation, recordQuery)
}

// runInTx runs sql on an already-open transaction. Callers that need to run
// a check (like Sample's table-existence lookup) before the real query, in
// the same transaction and under the same timeout, use this directly instead
// of run().
func (c *conn) runInTx(ctx context.Context, tx pgx.Tx, sql string, limitForTruncation int, recordQuery string) (*driver.QueryResult, error) {
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
			rowMap[col.Name] = marshalValue(values[i], col.Type)
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
// (avoids float precision loss on numeric/bigint for JSON consumers), byte
// slices become base64, and time.Time becomes either a date-only string
// ("2006-01-02") or a full RFC3339 timestamp depending on pgType — pgx
// decodes both `date` and `timestamp(tz)` into the same Go type, so this is
// the only place that still knows which one a given value came from.
// pgType is the source column's Postgres type name (e.g. "date", "_uuid").
func marshalValue(v any, pgType string) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case []byte:
		return base64.StdEncoding.EncodeToString(val)
	case time.Time:
		if pgType == "date" {
			return val.Format("2006-01-02")
		}
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
		// Postgres array type names are the element's name prefixed with
		// "_" (e.g. "_date", "_uuid"); strip it so elements format the same
		// way the scalar column type would.
		elemType := strings.TrimPrefix(pgType, "_")
		out := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = marshalValue(rv.Index(i).Interface(), elemType)
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
				return marshalValue(dv, pgType)
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
