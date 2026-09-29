package cmd

import (
	"fmt"
	"os"

	"github.com/diogoaalmeida/dbcli/internal/config"
)

func Profiles(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: dbcli profiles <list|add|remove> ...")
		return 2
	}

	switch args[0] {
	case "list":
		names, err := config.List()
		if err != nil {
			return fail(err, "config_error")
		}
		if names == nil {
			names = []string{}
		}
		return writeData(names)

	case "add":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: dbcli profiles add <name> <dsn>")
			return 2
		}
		if err := config.Add(args[1], args[2]); err != nil {
			return fail(err, "config_error")
		}
		fmt.Fprintf(os.Stdout, "profile %q saved\n", args[1])
		return 0

	case "remove":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: dbcli profiles remove <name>")
			return 2
		}
		if err := config.Remove(args[1]); err != nil {
			return fail(err, "config_error")
		}
		fmt.Fprintf(os.Stdout, "profile %q removed\n", args[1])
		return 0

	default:
		fmt.Fprintf(os.Stderr, "unknown profiles subcommand %q\n", args[0])
		return 2
	}
}
