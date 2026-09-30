package cmd

import (
	"context"
	"flag"

	"github.com/diogoaalmeida/dbcli/internal/driver"
)

func Schema(args []string) int {
	fs := flag.NewFlagSet("schema", flag.ContinueOnError)
	profile := fs.String("profile", "", "named connection profile")
	driverFlag := fs.String("driver", "", "override driver scheme (default: inferred from connection string)")
	schema := fs.String("schema", "public", "schema to list")
	timeout := fs.Int("timeout", 0, "statement timeout in seconds (default 5)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, cancel := withOperationTimeout(context.Background(), *timeout)
	defer cancel()
	conn, err := connect(ctx, *profile, *driverFlag)
	if err != nil {
		return fail(err, "connection_error")
	}
	defer conn.Close(ctx)

	tables, err := conn.ListSchema(ctx, *schema, driver.QueryOptions{TimeoutSeconds: *timeout})
	if err != nil {
		return fail(err, "schema_error")
	}
	return writeData(tables)
}
