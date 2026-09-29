package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/diogoaalmeida/dbcli/internal/driver"
)

// testConn connects to DBCLI_TEST_DATABASE_URL, skipping the test if it
// isn't set. Point it at a scratch local Postgres, e.g.:
//
//	docker run -d --name dbcli-test-pg -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:15
//	export DBCLI_TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
func testConn(t *testing.T) *conn {
	t.Helper()
	dsn := os.Getenv("DBCLI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("DBCLI_TEST_DATABASE_URL not set; skipping integration test")
	}

	c, err := (pgDriver{}).Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { c.Close(context.Background()) })
	return c.(*conn)
}

func TestQuery_ReadOnlyTransactionBlocksWrite(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	if _, err := c.Query(ctx, "select 1", driver.QueryOptions{}); err != nil {
		t.Fatalf("select 1 should succeed: %v", err)
	}

	// Bypass ValidateQuery entirely and call run() directly with a write
	// statement, to prove the READ ONLY transaction itself is the backstop
	// — not just the classifier.
	stmt := "create table dbcli_should_not_exist (id int)"
	if _, err := c.run(ctx, stmt, driver.QueryOptions{}, 0, stmt); err == nil {
		t.Fatalf("expected write to be rejected by the read-only transaction")
	}
}

func TestQuery_RowCapTruncates(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	result, err := c.Query(ctx, "select * from generate_series(1, 10) as g(n)", driver.QueryOptions{Limit: 3})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if result.RowCount != 3 {
		t.Fatalf("got %d rows, want 3", result.RowCount)
	}
	if !result.Truncated {
		t.Fatalf("expected Truncated=true")
	}
}

func TestQuery_LimitClampedToCeiling(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	result, err := c.Query(ctx, "select * from generate_series(1, 5) as g(n)", driver.QueryOptions{Limit: maxRowLimit + 1000})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if result.RowCount != 5 {
		t.Fatalf("got %d rows, want 5 (requested limit should clamp to ceiling, not error)", result.RowCount)
	}
}

func TestQuery_StatementTimeoutCancelsSlowQuery(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	if _, err := c.Query(ctx, "select pg_sleep(2)", driver.QueryOptions{TimeoutSeconds: 1}); err == nil {
		t.Fatalf("expected statement_timeout to cancel a query slower than the timeout")
	}
}

func TestExplain_PlainDoesNotRequireAnalyze(t *testing.T) {
	c := testConn(t)
	ctx := context.Background()

	result, err := c.Explain(ctx, "select 1", driver.QueryOptions{})
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	if len(result.Rows) == 0 {
		t.Fatalf("expected at least one plan row")
	}
}
