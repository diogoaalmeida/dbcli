package postgres

import (
	"context"
	"encoding/json"
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

// TestQuery_RangeTypeRendersAsStructuredJSONNotGoSyntaxDump covers
// pgtype.Range[T] (int4range, tsrange, numrange, ...), which has no
// database/sql/driver.Valuer implementation. Before this test, the fallback
// used fmt.Sprintf("%v", v), producing something like "{1 10 i e true}" —
// not valid structured JSON, and unreadable. It should now come through as
// a real JSON object using the range's own exported field names.
func TestQuery_RangeTypeRendersAsStructuredJSONNotGoSyntaxDump(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	if _, err := c.pgxConn.Exec(ctx, "create table if not exists dbcli_range_test (r int4range)"); err != nil {
		t.Fatalf("test fixture setup: %v", err)
	}
	t.Cleanup(func() {
		c.pgxConn.Exec(context.Background(), "drop table if exists dbcli_range_test")
	})
	if _, err := c.pgxConn.Exec(ctx, "insert into dbcli_range_test values (int4range(1, 10))"); err != nil {
		t.Fatalf("test fixture insert: %v", err)
	}

	result, err := c.Query(ctx, "select r from dbcli_range_test", driver.QueryOptions{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	// The whole point: this value must survive a real JSON round trip as an
	// object, not a bare string containing Go's %v syntax.
	encoded, err := json.Marshal(result.Rows[0]["r"])
	if err != nil {
		t.Fatalf("marshal range value: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("range value is not a JSON object (got %s): %v", encoded, err)
	}
	if _, ok := decoded["Lower"]; !ok {
		t.Fatalf("expected a \"Lower\" field in the range's JSON object, got %s", encoded)
	}
}
