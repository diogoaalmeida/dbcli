package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/diogoaalmeida/dbcli/internal/driver"
)

// TestQuery_RespectsCallerContextDeadline proves the actual mechanism behind
// the client-side timeout fix: a connection can succeed and the query can
// still hang server-side with no response (a stalled server, a network
// partition after the handshake) — statement_timeout alone doesn't help if
// the client never hears back at all. Before cmd/*.go started passing a
// bounded context (see cmd.withOperationTimeout), every command used
// context.Background() for the operation, which never expires on its own.
// Here we set opts.TimeoutSeconds high (so statement_timeout would NOT be
// what cuts this off) but give the caller's own ctx a short deadline, and
// confirm that's what actually bounds a slow query.
func TestQuery_RespectsCallerContextDeadline(t *testing.T) {
	c := testConn(t)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	start := time.Now()
	_, err := c.Query(ctx, "select pg_sleep(10)", driver.QueryOptions{TimeoutSeconds: 100})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected the caller's context deadline to cut off a 10s query")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("query took %s to return; the caller's 1s context deadline should have bounded it, not the 100s statement_timeout", elapsed)
	}
}
