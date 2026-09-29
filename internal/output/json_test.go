package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/diogoaalmeida/dbcli/internal/driver"
)

func TestWriteQueryResult_RowsNeverNull(t *testing.T) {
	var buf bytes.Buffer
	result := &driver.QueryResult{
		Columns: []driver.Column{{Name: "id", Type: "int4"}},
		Rows:    nil, // zero-row result
		Query:   "select 1 where false",
	}
	if err := WriteQueryResult(&buf, result); err != nil {
		t.Fatalf("WriteQueryResult: %v", err)
	}

	if strings.Contains(buf.String(), "\"rows\": null") {
		t.Fatalf("rows must never be null, even when empty; got: %s", buf.String())
	}

	var decoded struct {
		OK   bool             `json:"ok"`
		Rows []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !decoded.OK {
		t.Fatalf("expected ok:true")
	}
	if decoded.Rows == nil || len(decoded.Rows) != 0 {
		t.Fatalf("expected an empty (non-nil) rows array, got %#v", decoded.Rows)
	}
}

func TestWriteQueryResult_Shape(t *testing.T) {
	var buf bytes.Buffer
	result := &driver.QueryResult{
		Columns:    []driver.Column{{Name: "id", Type: "int4"}},
		Rows:       []map[string]any{{"id": "1"}},
		RowCount:   1,
		Truncated:  true,
		DurationMS: 5,
		Query:      "select id from t limit 1",
	}
	if err := WriteQueryResult(&buf, result); err != nil {
		t.Fatalf("WriteQueryResult: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"ok", "columns", "rows", "row_count", "truncated", "duration_ms", "query"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("expected key %q in envelope, got %#v", key, decoded)
		}
	}
	if decoded["truncated"] != true {
		t.Fatalf("expected truncated:true, got %#v", decoded["truncated"])
	}
}

func TestWriteData(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteData(&buf, []string{"public", "core"}); err != nil {
		t.Fatalf("WriteData: %v", err)
	}

	var decoded struct {
		OK   bool     `json:"ok"`
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !decoded.OK {
		t.Fatalf("expected ok:true")
	}
	if len(decoded.Data) != 2 || decoded.Data[0] != "public" {
		t.Fatalf("got %#v", decoded.Data)
	}
}

func TestWriteError(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteError(&buf, errors.New("query rejected"), "query_error"); err != nil {
		t.Fatalf("WriteError: %v", err)
	}

	var decoded struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.OK {
		t.Fatalf("expected ok:false")
	}
	if decoded.Error != "query rejected" || decoded.Code != "query_error" {
		t.Fatalf("got %#v", decoded)
	}
}

func TestWriteQueryResultTable_RendersNullAndRowCount(t *testing.T) {
	var buf bytes.Buffer
	result := &driver.QueryResult{
		Columns:    []driver.Column{{Name: "id"}, {Name: "name"}},
		Rows:       []map[string]any{{"id": "1", "name": nil}},
		RowCount:   1,
		DurationMS: 3,
	}
	if err := WriteQueryResultTable(&buf, result); err != nil {
		t.Fatalf("WriteQueryResultTable: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "NULL") {
		t.Fatalf("expected NULL to be rendered for a nil cell, got: %s", out)
	}
	if !strings.Contains(out, "(1 row(s), 3ms)") {
		t.Fatalf("expected row count summary line, got: %s", out)
	}
}
