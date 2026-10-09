package cmd

import (
	"os"
	"testing"
)

// testDSN returns a real Postgres DSN for cmd-level integration tests,
// skipping if DBCLI_TEST_DATABASE_URL isn't set — the same convention
// internal/postgres's tests use, duplicated here since it's a different
// package and the helper isn't exported.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("DBCLI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("DBCLI_TEST_DATABASE_URL not set")
	}
	return dsn
}
