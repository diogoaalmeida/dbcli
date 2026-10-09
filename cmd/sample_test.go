package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSample_ReturnsRowsAndRespectsLimit(t *testing.T) {
	dsn := testDSN(t)
	t.Setenv("DATABASE_URL", dsn)
	ctx := context.Background()
	ddl := ddlConn(t, dsn)

	if _, err := ddl.Exec(ctx, "create table if not exists cmd_sample_test (id serial primary key)"); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		ddl.Exec(context.Background(), "drop table if exists cmd_sample_test")
	})
	for i := 0; i < 3; i++ {
		if _, err := ddl.Exec(ctx, "insert into cmd_sample_test default values"); err != nil {
			t.Fatalf("test fixture insert: %v", err)
		}
	}

	out := captureStdout(t, func() {
		if code := Sample([]string{"cmd_sample_test", "--limit", "2"}); code != 0 {
			t.Fatalf("Sample: exit code %d", code)
		}
	})

	var decoded struct {
		OK       bool `json:"ok"`
		RowCount int  `json:"row_count"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("did not print valid JSON: %v\noutput: %s", err, out)
	}
	if !decoded.OK || decoded.RowCount != 2 {
		t.Fatalf("got row_count %d, want 2 (output: %s)", decoded.RowCount, out)
	}
}

// TestSample_FormatTableRendersPlainTextNotJSON is the one place this
// package tests --format table end to end through a real CLI entry
// point: before this, it was only tested by handing a struct directly
// to internal/output's WriteQueryResultTable, never through an actual
// command's flag parsing.
func TestSample_FormatTableRendersPlainTextNotJSON(t *testing.T) {
	dsn := testDSN(t)
	t.Setenv("DATABASE_URL", dsn)
	ctx := context.Background()
	ddl := ddlConn(t, dsn)

	if _, err := ddl.Exec(ctx, "create table if not exists cmd_sample_table_fmt_test (id serial primary key)"); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		ddl.Exec(context.Background(), "drop table if exists cmd_sample_table_fmt_test")
	})
	if _, err := ddl.Exec(ctx, "insert into cmd_sample_table_fmt_test default values"); err != nil {
		t.Fatalf("test fixture insert: %v", err)
	}

	out := captureStdout(t, func() {
		if code := Sample([]string{"cmd_sample_table_fmt_test", "--format", "table"}); code != 0 {
			t.Fatalf("Sample --format table: exit code %d", code)
		}
	})

	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("got JSON, want tabwriter plain text for --format table: %s", out)
	}
	if !strings.Contains(out, "id") || !strings.Contains(out, "row(s)") {
		t.Fatalf("expected a table with an \"id\" column header and a row-count summary line, got %q", out)
	}
}

func TestSample_UnknownDriverFailsWithJSONEnvelope(t *testing.T) {
	t.Setenv("DATABASE_URL", testDSN(t))

	out := captureStdout(t, func() {
		if code := Sample([]string{"anytable", "--driver", "not-a-real-driver"}); code == 0 {
			t.Fatalf("expected a non-zero exit code")
		}
	})
	var decoded struct {
		OK   bool   `json:"ok"`
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("did not print valid JSON even on error: %v\noutput: %s", err, out)
	}
	if decoded.OK || decoded.Code == "" {
		t.Fatalf("expected ok:false with a non-empty code, got %+v", decoded)
	}
}
