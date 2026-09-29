/*
Command dbcli is a read-only SQL CLI built for AI agents. Every query runs
inside a read-only, timeout-bounded, row-capped transaction, and every
result, success or failure, is a stable JSON envelope on stdout.

Installation:

	go install github.com/diogoaalmeida/dbcli@latest

Usage:

	dbcli <command> [flags] [args]

Commands:

	query "<SQL>" [--profile NAME] [--limit N] [--timeout Ns] [--format json|table]
	    Run a validated, read-only query and print the result.
	explain "<SQL>" [--profile NAME] [--analyze]
	    Print the query plan. --analyze runs EXPLAIN ANALYZE (executes the query).
	schemas [--profile NAME]
	    List non-system schemas with their table counts.
	schema [--profile NAME] [--schema public]
	    List tables in one schema.
	describe <table> [--profile NAME] [--schema public]
	    Show a table's columns, indexes, and foreign keys.
	sample <table> [--profile NAME] [--limit 20]
	    Print up to N rows from a table.
	profiles list | add <name> <dsn> | remove <name>
	    Manage named connection profiles in ~/.config/dbcli/profiles.env.
	version
	    Print the build version.

Connection:

	Every command resolves its connection via --profile NAME (looked up in
	~/.config/dbcli/profiles.env) or, if omitted, the DATABASE_URL
	environment variable. See the README for setting up a least-privilege
	database role before pointing dbcli at a real database.

For the safety guarantees (read-only transaction enforcement, statement
timeouts, row caps) and full setup instructions, see the README:
https://github.com/diogoaalmeida/dbcli
*/
package main
