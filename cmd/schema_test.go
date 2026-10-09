package cmd

import (
	"encoding/json"
	"testing"
)

func TestSchema_ReturnsDataEnvelopeForPublicByDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", testDSN(t))

	out := captureStdout(t, func() {
		if code := Schema(nil); code != 0 {
			t.Fatalf("Schema: exit code %d", code)
		}
	})

	var decoded struct {
		OK   bool  `json:"ok"`
		Data []any `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("did not print valid JSON: %v\noutput: %s", err, out)
	}
	if !decoded.OK {
		t.Fatalf("expected ok:true, got %s", out)
	}
}

func TestSchema_RespectsSchemaFlag(t *testing.T) {
	t.Setenv("DATABASE_URL", testDSN(t))

	out := captureStdout(t, func() {
		if code := Schema([]string{"--schema", "pg_catalog"}); code != 0 {
			t.Fatalf("Schema --schema pg_catalog: exit code %d", code)
		}
	})

	var decoded struct {
		OK   bool  `json:"ok"`
		Data []any `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("did not print valid JSON: %v\noutput: %s", err, out)
	}
	if !decoded.OK || len(decoded.Data) == 0 {
		t.Fatalf("expected ok:true with a non-empty pg_catalog listing, got %s", out)
	}
}

func TestSchema_UnknownDriverFailsWithJSONEnvelope(t *testing.T) {
	t.Setenv("DATABASE_URL", testDSN(t))

	out := captureStdout(t, func() {
		if code := Schema([]string{"--driver", "not-a-real-driver"}); code == 0 {
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
