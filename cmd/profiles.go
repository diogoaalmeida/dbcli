package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

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
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: dbcli profiles add <name> [dsn]")
			fmt.Fprintln(os.Stderr, "       (omit dsn to read it from stdin, avoiding shell history/ps exposure)")
			return 2
		}
		dsn := ""
		if len(args) >= 3 {
			dsn = args[2]
		} else {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fail(fmt.Errorf("read dsn from stdin: %w", err), "config_error")
			}
			dsn = strings.TrimSpace(string(data))
		}
		if err := config.Add(args[1], dsn); err != nil {
			return fail(err, "config_error")
		}
		return writeData(map[string]string{"profile": args[1], "action": "saved"})

	case "remove":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: dbcli profiles remove <name>")
			return 2
		}
		if err := config.Remove(args[1]); err != nil {
			return fail(err, "config_error")
		}
		return writeData(map[string]string{"profile": args[1], "action": "removed"})

	default:
		fmt.Fprintf(os.Stderr, "unknown profiles subcommand %q\n", args[0])
		return 2
	}
}
