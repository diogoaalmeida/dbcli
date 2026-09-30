// Package cmd implements dbcli's subcommands. Each exported function takes
// the subcommand's raw args (os.Args[2:]) and returns a process exit code.
package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/diogoaalmeida/dbcli/internal/config"
	"github.com/diogoaalmeida/dbcli/internal/driver"
	"github.com/diogoaalmeida/dbcli/internal/output"

	_ "github.com/diogoaalmeida/dbcli/internal/postgres" // registers the postgres:// driver
)

const (
	connectTimeout = 10 * time.Second

	// defaultOperationTimeoutSeconds mirrors postgres.defaultTimeoutSeconds
	// (the server-side statement_timeout default) so a command with no
	// --timeout flag still gets a sane client-side ceiling.
	defaultOperationTimeoutSeconds = 5

	// clientTimeoutBufferSeconds is headroom added on top of the requested
	// (or default) operation timeout, to cover connect() and network
	// latency the server-side statement_timeout alone can't bound.
	clientTimeoutBufferSeconds = 15
)

// withOperationTimeout bounds an entire command — connect plus the actual
// operation — on the client side. statement_timeout alone only bounds
// server-side execution time; if the connection stalls before Postgres
// starts working (a network partition, an unresponsive server), nothing
// server-side ever gives up, so every command needs its own deadline too.
func withOperationTimeout(parent context.Context, timeoutSeconds int) (context.Context, context.CancelFunc) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultOperationTimeoutSeconds
	}
	return context.WithTimeout(parent, time.Duration(timeoutSeconds+clientTimeoutBufferSeconds)*time.Second)
}

// connect resolves --profile (or DATABASE_URL) to a DSN, infers the driver
// from the DSN's URI scheme unless driverOverride is set, and connects.
func connect(ctx context.Context, profile, driverOverride string) (driver.Conn, error) {
	dsn, err := config.Resolve(profile)
	if err != nil {
		return nil, err
	}

	scheme := driverOverride
	if scheme == "" {
		u, err := url.Parse(dsn)
		if err != nil || u.Scheme == "" {
			return nil, fmt.Errorf("cannot determine driver from connection string; pass --driver explicitly")
		}
		scheme = u.Scheme
	}

	d, ok := driver.Lookup(scheme)
	if !ok {
		return nil, driver.ErrUnknownScheme(scheme)
	}

	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	return d.Connect(connectCtx, dsn)
}

// fail writes the JSON error envelope to stdout (so agents parsing stdout
// always get JSON, success or failure) and returns the exit code.
func fail(err error, code string) int {
	if writeErr := output.WriteError(os.Stdout, err, code); writeErr != nil {
		fmt.Fprintln(os.Stderr, writeErr)
	}
	return 1
}

func writeResult(result *driver.QueryResult, format string) int {
	var err error
	switch format {
	case "table":
		err = output.WriteQueryResultTable(os.Stdout, result)
	default:
		err = output.WriteQueryResult(os.Stdout, result)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func writeData(data any) int {
	if err := output.WriteData(os.Stdout, data); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
