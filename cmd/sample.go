package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/diogoaalmeida/dbcli/internal/driver"
)

func Sample(args []string) int {
	fs := flag.NewFlagSet("sample", flag.ContinueOnError)
	profile := fs.String("profile", "", "named connection profile")
	driverFlag := fs.String("driver", "", "override driver scheme (default: inferred from connection string)")
	schema := fs.String("schema", "public", "schema the table belongs to")
	limit := fs.Int("limit", 20, "rows to sample")
	format := fs.String("format", "json", "output format: json|table")
	flagArgs, positional := splitFlagsAndPositional(args, nil)
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "usage: dbcli sample [flags] <table>")
		return 2
	}
	table := positional[0]

	ctx := context.Background()
	conn, err := connect(ctx, *profile, *driverFlag)
	if err != nil {
		return fail(err, "connection_error")
	}
	defer conn.Close(ctx)

	result, err := conn.Sample(ctx, *schema, table, driver.QueryOptions{Limit: *limit})
	if err != nil {
		return fail(err, "query_error")
	}
	return writeResult(result, *format)
}
