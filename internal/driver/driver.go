// Package driver defines the engine-neutral interface every database backend
// implements, plus a scheme registry so the CLI can pick a backend from a
// connection string without knowing about specific engines.
package driver

import (
	"context"
	"fmt"
)

// Column describes one column in a query result.
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// QueryResult is the shape every query-producing command returns.
type QueryResult struct {
	Columns    []Column         `json:"columns"`
	Rows       []map[string]any `json:"rows"`
	RowCount   int              `json:"row_count"`
	Truncated  bool             `json:"truncated"`
	DurationMS int64            `json:"duration_ms"`
	Query      string           `json:"query"`
}

// TableInfo is one row of a schema listing.
type TableInfo struct {
	Schema        string `json:"schema"`
	Name          string `json:"name"`
	Kind          string `json:"kind"` // "table" or "view"
	EstimatedRows int64  `json:"estimated_rows"`
}

// SchemaInfo is one row of a schemas listing.
type SchemaInfo struct {
	Name       string `json:"name"`
	TableCount int    `json:"table_count"`
}

// ColumnInfo describes one column of a described table.
type ColumnInfo struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Nullable bool    `json:"nullable"`
	Default  *string `json:"default,omitempty"`
}

// IndexInfo describes one index of a described table.
type IndexInfo struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
}

// ForeignKeyInfo describes one foreign key of a described table.
type ForeignKeyInfo struct {
	Column    string `json:"column"`
	RefTable  string `json:"ref_table"`
	RefColumn string `json:"ref_column"`
}

// TableDescription is the full result of describing one table.
type TableDescription struct {
	Schema      string           `json:"schema"`
	Table       string           `json:"table"`
	Columns     []ColumnInfo     `json:"columns"`
	Indexes     []IndexInfo      `json:"indexes"`
	ForeignKeys []ForeignKeyInfo `json:"foreign_keys"`
}

// QueryOptions carries the per-invocation safety knobs a Conn must enforce.
type QueryOptions struct {
	Limit          int  // hard cap on rows returned; 0 means use the driver default
	TimeoutSeconds int  // statement timeout; 0 means use the driver default
	Analyze        bool // Explain only: EXPLAIN ANALYZE instead of plain EXPLAIN
}

// Conn is a live, driver-specific connection capable of running validated,
// read-only queries and introspecting schema.
type Conn interface {
	Query(ctx context.Context, sql string, opts QueryOptions) (*QueryResult, error)
	Explain(ctx context.Context, sql string, opts QueryOptions) (*QueryResult, error)
	ListSchemas(ctx context.Context, opts QueryOptions) ([]SchemaInfo, error)
	ListSchema(ctx context.Context, schema string, opts QueryOptions) ([]TableInfo, error)
	DescribeTable(ctx context.Context, schema, table string, opts QueryOptions) (*TableDescription, error)
	Sample(ctx context.Context, schema, table string, opts QueryOptions) (*QueryResult, error)
	Close(ctx context.Context) error
}

// Driver connects a DSN into a live Conn for one specific database engine.
type Driver interface {
	Connect(ctx context.Context, dsn string) (Conn, error)
}

var registry = map[string]Driver{}

// Register associates a connection-string URI scheme with a Driver
// implementation. Called from each driver package's init().
func Register(scheme string, d Driver) {
	registry[scheme] = d
}

// Lookup returns the Driver registered for scheme, if any.
func Lookup(scheme string) (Driver, bool) {
	d, ok := registry[scheme]
	return d, ok
}

// ErrUnknownScheme is returned when no driver is registered for a scheme.
func ErrUnknownScheme(scheme string) error {
	return fmt.Errorf("no driver registered for scheme %q", scheme)
}
