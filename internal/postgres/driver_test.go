package postgres

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestConnect_ErrorNeverLeaksThePassword verifies, empirically rather than
// by inspection, that a failed connection's error string never contains the
// DSN's password. This was flagged as "worth confirming" during a code
// review: the caller's designed consumer is an LLM agent whose transcripts
// often get logged or shipped elsewhere, so a credential leaking into "the
// thing the agent reads" is a bigger deal here than in a normal CLI.
//
// Covers three failure shapes: a malformed DSN (fails before any network
// call), a connection actively refused, and a live server rejecting a wrong
// password.
func TestConnect_ErrorNeverLeaksThePassword(t *testing.T) {
	const password = "SUPERSECRETPASSWORD123"

	assertNoLeak := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("expected a connection error")
		}
		if strings.Contains(err.Error(), password) {
			t.Fatalf("error message leaked the password: %v", err)
		}
	}

	t.Run("malformed DSN, fails before any network call", func(t *testing.T) {
		// An invalid sslmode fails pgx's own DSN parsing/config step,
		// before it ever dials anything.
		dsn := "postgres://user:" + password + "@localhost:5432/db?sslmode=not-a-real-mode"
		_, err := (pgDriver{}).Connect(context.Background(), dsn)
		assertNoLeak(t, err)
	})

	t.Run("connection refused", func(t *testing.T) {
		// A closed local TCP port: open and immediately close a listener to
		// get a port nothing is listening on.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		addr := ln.Addr().String()
		ln.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		dsn := "postgres://user:" + password + "@" + addr + "/db?sslmode=disable"
		_, err = (pgDriver{}).Connect(ctx, dsn)
		assertNoLeak(t, err)
	})

	t.Run("wrong password against a real server", func(t *testing.T) {
		dsn := "postgres://baduser:" + password + "@" + testDBHost(t) + "/postgres?sslmode=disable"
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := (pgDriver{}).Connect(ctx, dsn)
		assertNoLeak(t, err)
	})
}

// testDBHost extracts host:port from DBCLI_TEST_DATABASE_URL, skipping the
// test if it isn't set (same convention as testConn).
func testDBHost(t *testing.T) string {
	t.Helper()
	c := testConn(t)
	t.Cleanup(func() { c.Close(context.Background()) })
	// c is already connected; we just need a real, reachable host:port to
	// point a *bad-credential* connection at, so reuse whatever testConn
	// resolved rather than re-parsing the DSN here.
	return c.pgxConn.Config().Host + ":" + strconv.Itoa(int(c.pgxConn.Config().Port))
}
