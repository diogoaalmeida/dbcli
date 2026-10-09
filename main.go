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
	if bi, ok := debug.ReadBuildInfo(); ok {
		if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			return bi.Main.Version
		}
		if v := vcsVersion(bi); v != "" {
			return v
		}
	}
	return version
}

// vcsVersion builds a "dev+<short-revision>[-dirty]" string from the VCS
// stamp Go embeds automatically (since Go 1.18) in any build run from
// inside a git checkout. Without this, a plain `go build -o dbcli .`
// (the README's own documented build method) reports a bare "dev" with
// no way to tell which commit it came from; every other install path
// (goreleaser binaries, `go install pkg@version`) already resolves a
// real version via bi.Main.Version above.
func vcsVersion(bi *debug.BuildInfo) string {
	var revision string
	var modified bool
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if revision == "" {
		return ""
	}
	if len(revision) > 7 {
		revision = revision[:7]
	}
	v := "dev+" + revision
	if modified {
		v += "-dirty"
	}
	return v
}

func main() {
	os.Exit(run(os.Args[1:]))
}

// run is main's dispatch logic, factored out so tests can exercise exit
// codes and the version/help plain-text output without the process
// actually exiting.
func run(args []string) int {
	if len(args) < 1 {
		printUsage()
		return 2
	}

	switch args[0] {
	case "query":
		return cmd.Query(args[1:])
	case "explain":
		return cmd.Explain(args[1:])
	case "schemas":
		return cmd.Schemas(args[1:])
	case "schema":
		return cmd.Schema(args[1:])
	case "describe":
		return cmd.Describe(args[1:])
	case "sample":
		return cmd.Sample(args[1:])
	case "profiles":
		return cmd.Profiles(args[1:])
	case "version", "--version":
		// version and help are the only two commands that print plain
		// text instead of the JSON envelope every other command uses:
		// there's no query/result to report, just a version string or
		// usage text, so a JSON wrapper would add nothing.
		fmt.Println("dbcli " + resolveVersion())
		return 0
	case "help", "--help", "-h":
		printUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
		printUsage()
		return 2
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `dbcli: read-only SQL for AI agents

Usage:
  dbcli query "<SQL>" [--profile NAME] [--limit N] [--timeout Ns] [--format json|table]
  dbcli explain "<SQL>" [--profile NAME] [--analyze] [--timeout Ns] [--format json|table]
  dbcli schemas [--profile NAME] [--timeout Ns]
  dbcli schema [--profile NAME] [--schema public] [--timeout Ns]
  dbcli describe <table> [--profile NAME] [--schema public] [--timeout Ns]
  dbcli sample <table> [--profile NAME] [--schema public] [--limit 20] [--timeout Ns] [--format json|table]
  dbcli profiles list
  dbcli profiles add <name> [dsn]   (reads dsn from stdin if omitted)
  dbcli profiles remove <name>
  dbcli version

Connection: --profile NAME resolves against ~/.config/dbcli/profiles.env,
or set DATABASE_URL directly. See README.md for setup, including creating
a least-privilege read-only database role.
`)
}
