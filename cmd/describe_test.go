package cmd

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"
)

// ddlConn opens a raw pgx connection for test-fixture DDL, since
// dbcli's own read-only Query path can't CREATE/DROP TABLE. Mirrors the
// pattern internal/postgres's tests already use via testConn's
// underlying pgxConn.
func ddlConn(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("pgx.Connect for test fixture setup: %v", err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	return conn
}

func TestDescribe_ReturnsDataEnvelopeForARealTable(t *testing.T) {
	dsn := testDSN(t)
	t.Setenv("DATABASE_URL", dsn)
	ctx := context.Background()
	ddl := ddlConn(t, dsn)

	if _, err := ddl.Exec(ctx, "create table if not exists cmd_describe_test (id serial primary key, name text)"); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		ddl.Exec(context.Background(), "drop table if exists cmd_describe_test")
	})

	out := captureStdout(t, func() {
		if code := Describe([]string{"cmd_describe_test"}); code != 0 {
			t.Fatalf("Describe: exit code %d", code)
		}
	})

	var decoded struct {
		OK   bool `json:"ok"`
		Data struct {
			Table   string `json:"table"`
			Columns []struct {
				Name string `json:"name"`
			} `json:"columns"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("did not print valid JSON: %v\noutput: %s", err, out)
	}
	if !decoded.OK || decoded.Data.Table != "cmd_describe_test" || len(decoded.Data.Columns) != 2 {
		t.Fatalf("unexpected describe envelope: %+v (output: %s)", decoded, out)
	}
}

func TestDescribe_UnknownTableFailsWithJSONEnvelope(t *testing.T) {
	t.Setenv("DATABASE_URL", testDSN(t))

	out := captureStdout(t, func() {
		if code := Describe([]string{"cmd_describe_does_not_exist"}); code == 0 {
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
	if decoded.OK || decoded.Code != "describe_error" {
		t.Fatalf("expected ok:false, code:\"describe_error\", got %+v", decoded)
	}
}

func TestDescribe_UnknownDriverFailsWithJSONEnvelope(t *testing.T) {
	t.Setenv("DATABASE_URL", testDSN(t))

	out := captureStdout(t, func() {
		if code := Describe([]string{"cmd_describe_test", "--driver", "not-a-real-driver"}); code == 0 {
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
