// Package output renders command results into the stable JSON envelope
// dbcli's agent consumers parse, plus a human-readable table format.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/diogoaalmeida/dbcli/internal/driver"
)

type queryEnvelope struct {
	OK         bool             `json:"ok"`
	Columns    []driver.Column  `json:"columns"`
	Rows       []map[string]any `json:"rows"`
	RowCount   int              `json:"row_count"`
	Truncated  bool             `json:"truncated"`
	DurationMS int64            `json:"duration_ms"`
	Query      string           `json:"query"`
}

// WriteQueryResult writes a query/explain/sample result in the stable JSON
// envelope. Rows is always a JSON array, never null, even when empty.
func WriteQueryResult(w io.Writer, r *driver.QueryResult) error {
	rows := r.Rows
	if rows == nil {
		rows = []map[string]any{}
	}
	env := queryEnvelope{
		OK:         true,
		Columns:    r.Columns,
		Rows:       rows,
		RowCount:   r.RowCount,
		Truncated:  r.Truncated,
		DurationMS: r.DurationMS,
		Query:      r.Query,
	}
	return encode(w, env)
}

type dataEnvelope struct {
	OK   bool `json:"ok"`
	Data any  `json:"data"`
}

// WriteData writes any non-query result (schema listings, table
// descriptions, profile lists) under {"ok": true, "data": ...}.
func WriteData(w io.Writer, data any) error {
	return encode(w, dataEnvelope{OK: true, Data: data})
}

type errorEnvelope struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
	Code  string `json:"code"`
}

// WriteError writes {"ok": false, "error": ..., "code": ...}. Callers
// should still exit non-zero — this only shapes the body.
func WriteError(w io.Writer, err error, code string) error {
	return encode(w, errorEnvelope{OK: false, Error: err.Error(), Code: code})
}

func encode(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// WriteQueryResultTable renders a query result as an aligned text table,
// for --format table / human use at a terminal.
func WriteQueryResultTable(w io.Writer, r *driver.QueryResult) error {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)

	for i, col := range r.Columns {
		if i > 0 {
			fmt.Fprint(tw, "\t")
		}
		fmt.Fprint(tw, col.Name)
	}
	fmt.Fprintln(tw)

	for _, row := range r.Rows {
		for i, col := range r.Columns {
			if i > 0 {
				fmt.Fprint(tw, "\t")
			}
			fmt.Fprint(tw, formatCell(row[col.Name]))
		}
		fmt.Fprintln(tw)
	}

	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(w, "(%d row(s), %dms%s)\n", r.RowCount, r.DurationMS, truncatedSuffix(r.Truncated))
	return nil
}

func truncatedSuffix(truncated bool) string {
	if truncated {
		return ", truncated"
	}
	return ""
}

func formatCell(v any) string {
	if v == nil {
		return "NULL"
	}
	return fmt.Sprint(v)
}
