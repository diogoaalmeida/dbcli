package postgres

import (
	"context"
	"testing"

	"github.com/diogoaalmeida/dbcli/internal/driver"
)

// TestQuery_MacaddrRendersAsCanonicalString covers the same bug pattern as
// the original uuid fix: pgx decodes macaddr into net.HardwareAddr, a named
// []byte type that the plain []byte case doesn't match, so without an
// explicit case it fell through to the generic slice branch and rendered as
// an array of raw byte numbers instead of "08:00:2b:01:02:03".
func TestQuery_MacaddrRendersAsCanonicalString(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	if _, err := c.pgxConn.Exec(ctx, "create table if not exists dbcli_macaddr_test (mac macaddr)"); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_macaddr_test")
	})
	if _, err := c.pgxConn.Exec(ctx, "insert into dbcli_macaddr_test values ('08:00:2b:01:02:03')"); err != nil {
		t.Fatalf("test fixture insert: %v", err)
	}

	result, err := c.Query(ctx, "select mac from dbcli_macaddr_test", driver.QueryOptions{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	got, ok := result.Rows[0]["mac"].(string)
	if !ok || got != "08:00:2b:01:02:03" {
		t.Fatalf("got %#v, want the canonical macaddr string", result.Rows[0]["mac"])
	}
}

// TestQuery_RangeTypesRenderBoundsCorrectly covers pgtype.Range[T] (int4range,
// numrange, int8range, daterange, ...), which has no database/sql/driver.Valuer
// implementation.
//
// This subsumes an earlier, weaker version of this test that only checked a
// "Lower" key existed on an int4range result. An earlier fix (returning the
// raw struct so json.Marshal renders it instead of fmt.Sprintf-ing a Go %v
// dump) passed that check while reintroducing two of the exact bugs
// marshalValue exists to prevent, because raw struct fields bypass its
// numeric-as-string and date-vs-timestamp rules entirely:
//   - numrange/int8range bounds serialized as bare JSON numbers, losing
//     precision for large int8 bounds and violating the numeric-as-string
//     convention every other numeric type in this file follows.
//   - daterange bounds serialized as fake-midnight timestamps
//     ("2026-01-01T00:00:00Z") instead of date-only strings, since bounds
//     never reached the pgType == "date" branch that scalar date columns do.
//
// This version checks actual bound values and types, not just field presence,
// so it would have caught both regressions.
func TestQuery_RangeTypesRenderBoundsCorrectly(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	cases := []struct {
		name      string
		sqlValue  string
		wantLower string
		wantUpper string
	}{
		{"int4range", "int4range(1, 10)", "1", "10"},
		{"numrange bounds preserve precision as strings", "numrange(1.5, 9.75)", "1.5", "9.75"},
		{"int8range bounds beyond float64 precision stay exact strings", "int8range(9223372036854775800, 9223372036854775807)", "9223372036854775800", "9223372036854775807"},
		{"daterange bounds are date-only, not fake-midnight timestamps", "daterange('2026-01-01', '2026-02-01')", "2026-01-01", "2026-02-01"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := c.Query(ctx, "select "+tc.sqlValue+" as r", driver.QueryOptions{})
			if err != nil {
				t.Fatalf("query: %v", err)
			}

			r, ok := result.Rows[0]["r"].(map[string]any)
			if !ok {
				t.Fatalf("expected the range to render as a JSON object, got %#v", result.Rows[0]["r"])
			}

			lower, ok := r["lower"].(string)
			if !ok || lower != tc.wantLower {
				t.Fatalf("lower: got %#v, want %q as a string", r["lower"], tc.wantLower)
			}
			upper, ok := r["upper"].(string)
			if !ok || upper != tc.wantUpper {
				t.Fatalf("upper: got %#v, want %q as a string", r["upper"], tc.wantUpper)
			}
			if r["lower_type"] != "inclusive" {
				t.Fatalf("lower_type: got %#v, want \"inclusive\"", r["lower_type"])
			}
			if r["upper_type"] != "exclusive" {
				t.Fatalf("upper_type: got %#v, want \"exclusive\"", r["upper_type"])
			}
			if r["valid"] != true {
				t.Fatalf("valid: got %#v, want true", r["valid"])
			}
		})
	}
}
