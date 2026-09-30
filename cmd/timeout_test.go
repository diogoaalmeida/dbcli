package cmd

import (
	"fmt"
	"net"
	"testing"
	"time"
)

// TestQuery_ClientSideTimeoutBoundsAHungConnection simulates the exact gap
// statement_timeout alone can't cover: a connection that succeeds at the TCP
// level but then never responds (a stalled server, a network partition after
// the handshake). Before withOperationTimeout existed, every cmd/*.go passed
// context.Background() to the actual operation, so nothing on the client
// side would ever give up here.
func TestQuery_ClientSideTimeoutBoundsAHungConnection(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			// Accept the TCP connection and then do nothing — never send
			// the Postgres startup response, never close it.
			t.Cleanup(func() { c.Close() })
		}
	}()

	dsn := fmt.Sprintf("postgres://user:pass@%s/db?sslmode=disable", ln.Addr().String())
	t.Setenv("DATABASE_URL", dsn)

	done := make(chan int, 1)
	go func() { done <- Query([]string{"select 1", "--timeout", "1"}) }()

	select {
	case code := <-done:
		if code == 0 {
			t.Fatalf("expected a non-zero exit code for a connection that never responds")
		}
	case <-time.After(25 * time.Second):
		t.Fatalf("Query did not return within the client-side timeout; a hung connection would block forever")
	}
}
