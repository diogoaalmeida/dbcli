package cmd

import (
	"encoding/json"
	"testing"
)

func TestExplain_ReturnsQueryEnvelope(t *testing.T) {
	t.Setenv("DATABASE_URL", testDSN(t))

	out := captureStdout(t, func() {
		if code := Explain([]string{"select 1"}); code != 0 {
			t.Fatalf("Explain: exit code %d", code)
		}
	})

	var decoded struct {
		OK   bool             `json:"ok"`
		Rows []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("did not print valid JSON: %v\noutput: %s", err, out)
	}
	if !decoded.OK || len(decoded.Rows) == 0 {
		t.Fatalf("expected ok:true with plan rows, got %s", out)
	}
}

// TestExplain_RejectsWriteQueries is the same classifier guarantee
// query/sample already have tested at the internal/postgres layer,
// exercised here through the actual Explain() CLI entry point.
func TestExplain_RejectsWriteQueries(t *testing.T) {
	t.Setenv("DATABASE_URL", testDSN(t))

	out := captureStdout(t, func() {
		if code := Explain([]string{"delete from pg_catalog.pg_class"}); code == 0 {
			t.Fatalf("expected a non-zero exit code for a write query")
		}
	})
	var decoded struct {
		OK   bool   `json:"ok"`
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("did not print valid JSON even on error: %v\noutput: %s", err, out)
	}
	if decoded.OK || decoded.Code != "query_error" {
		t.Fatalf("expected ok:false, code:\"query_error\", got %+v", decoded)
	}
}

func TestExplain_UnknownDriverFailsWithJSONEnvelope(t *testing.T) {
	t.Setenv("DATABASE_URL", testDSN(t))

	out := captureStdout(t, func() {
		if code := Explain([]string{"select 1", "--driver", "not-a-real-driver"}); code == 0 {
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
