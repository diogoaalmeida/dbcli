package cmd

import (
	"context"
	"flag"
)

func Schemas(args []string) int {
	fs := flag.NewFlagSet("schemas", flag.ContinueOnError)
	profile := fs.String("profile", "", "named connection profile")
	driverFlag := fs.String("driver", "", "override driver scheme (default: inferred from connection string)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx := context.Background()
	conn, err := connect(ctx, *profile, *driverFlag)
	if err != nil {
		return fail(err, "connection_error")
	}
	defer conn.Close(ctx)

	schemas, err := conn.ListSchemas(ctx)
	if err != nil {
		return fail(err, "schema_error")
	}
	return writeData(schemas)
}
