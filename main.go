package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/diogoaalmeida/dbcli/cmd"
)

// version is overridden at release build time via -ldflags "-X main.version=...",
// which only applies to goreleaser's prebuilt binaries. Everyone else gets
// here via `go install github.com/diogoaalmeida/dbcli@<version>`, so
// resolveVersion() falls back to the module version Go embeds automatically
// in that case.
var version = "dev"

func resolveVersion() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return version
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "query":
		os.Exit(cmd.Query(os.Args[2:]))
	case "explain":
		os.Exit(cmd.Explain(os.Args[2:]))
	case "schemas":
		os.Exit(cmd.Schemas(os.Args[2:]))
	case "schema":
		os.Exit(cmd.Schema(os.Args[2:]))
	case "describe":
		os.Exit(cmd.Describe(os.Args[2:]))
	case "sample":
		os.Exit(cmd.Sample(os.Args[2:]))
	case "profiles":
		os.Exit(cmd.Profiles(os.Args[2:]))
	case "version", "--version":
		fmt.Println("dbcli " + resolveVersion())
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `dbcli: read-only SQL for AI agents

Usage:
  dbcli query "<SQL>" [--profile NAME] [--limit N] [--timeout Ns] [--format json|table]
  dbcli explain "<SQL>" [--profile NAME] [--analyze]
  dbcli schemas [--profile NAME]
  dbcli schema [--profile NAME] [--schema public]
  dbcli describe <table> [--profile NAME] [--schema public]
  dbcli sample <table> [--profile NAME] [--limit 20]
  dbcli profiles list
  dbcli profiles add <name> <dsn>
  dbcli profiles remove <name>
  dbcli version

Connection: --profile NAME resolves against ~/.config/dbcli/profiles.env,
or set DATABASE_URL directly. See README.md for setup, including creating
a least-privilege read-only database role.
`)
}
