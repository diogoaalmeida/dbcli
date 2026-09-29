package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
)

func Describe(args []string) int {
	fs := flag.NewFlagSet("describe", flag.ContinueOnError)
	profile := fs.String("profile", "", "named connection profile")
	driverFlag := fs.String("driver", "", "override driver scheme (default: inferred from connection string)")
	schema := fs.String("schema", "public", "schema the table belongs to")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: dbcli describe [flags] <table>")
		return 2
	}
	table := fs.Arg(0)

	ctx := context.Background()
	conn, err := connect(ctx, *profile, *driverFlag)
	if err != nil {
		return fail(err, "connection_error")
	}
	defer conn.Close(ctx)

	desc, err := conn.DescribeTable(ctx, *schema, table)
	if err != nil {
		return fail(err, "describe_error")
	}
	return writeData(desc)
}
