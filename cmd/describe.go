package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/diogoaalmeida/dbcli/internal/driver"
)

func Describe(args []string) int {
	fs := flag.NewFlagSet("describe", flag.ContinueOnError)
	profile := fs.String("profile", "", "named connection profile")
	driverFlag := fs.String("driver", "", "override driver scheme (default: inferred from connection string)")
	schema := fs.String("schema", "public", "schema the table belongs to")
	timeout := fs.Int("timeout", 0, "statement timeout in seconds (default 5)")
	flagArgs, positional := splitFlagsAndPositional(args, nil)
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "usage: dbcli describe [flags] <table>")
		return 2
	}
	table := positional[0]

	ctx, cancel := withOperationTimeout(context.Background(), *timeout)
	defer cancel()
	conn, err := connect(ctx, *profile, *driverFlag)
	if err != nil {
		return fail(err, "connection_error")
	}
	defer conn.Close(ctx)

	desc, err := conn.DescribeTable(ctx, *schema, table, driver.QueryOptions{TimeoutSeconds: *timeout})
	if err != nil {
		return fail(err, "describe_error")
	}
	return writeData(desc)
}
