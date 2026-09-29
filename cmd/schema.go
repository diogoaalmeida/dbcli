package cmd

import (
	"context"
	"flag"
)

func Schema(args []string) int {
	fs := flag.NewFlagSet("schema", flag.ContinueOnError)
	profile := fs.String("profile", "", "named connection profile")
	driverFlag := fs.String("driver", "", "override driver scheme (default: inferred from connection string)")
	schema := fs.String("schema", "public", "schema to list")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx := context.Background()
	conn, err := connect(ctx, *profile, *driverFlag)
	if err != nil {
		return fail(err, "connection_error")
	}
	defer conn.Close(ctx)

	tables, err := conn.ListSchema(ctx, *schema)
	if err != nil {
		return fail(err, "schema_error")
	}
	return writeData(tables)
}
