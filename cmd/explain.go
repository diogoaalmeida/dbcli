package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/diogoaalmeida/dbcli/internal/driver"
)

func Explain(args []string) int {
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	profile := fs.String("profile", "", "named connection profile")
	driverFlag := fs.String("driver", "", "override driver scheme (default: inferred from connection string)")
	analyze := fs.Bool("analyze", false, "run EXPLAIN ANALYZE (executes the query; plain EXPLAIN does not)")
	timeout := fs.Int("timeout", 0, "statement timeout in seconds (default 5)")
	format := fs.String("format", "json", "output format: json|table")
	flagArgs, positional := splitFlagsAndPositional(args, map[string]bool{
		"profile": false, "driver": false, "analyze": true, "timeout": false, "format": false,
	})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, `usage: dbcli explain [flags] "<SQL>"`)
		return 2
	}
	sql := positional[0]

	ctx, cancel := withOperationTimeout(context.Background(), *timeout)
	defer cancel()
	conn, err := connect(ctx, *profile, *driverFlag)
	if err != nil {
		return fail(err, "connection_error")
	}
	defer conn.Close(ctx)

	result, err := conn.Explain(ctx, sql, driver.QueryOptions{Analyze: *analyze, TimeoutSeconds: *timeout})
	if err != nil {
		return fail(err, "query_error")
	}
	return writeResult(result, *format)
}
