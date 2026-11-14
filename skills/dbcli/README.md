# dbcli skill

Use this skill to explore a Postgres database and answer questions with
read-only SQL. It guides the agent through schemas, tables, columns,
and sample rows before writing a query, then explains how to read the
CLI's JSON output.

## Before you start

The agent needs shell access, the dbcli binary, and a connection you
have authorized. The skill includes installation instructions for
dbcli v1.1.0. Installing the skill alone does not install the binary.
Use a database role with only the permissions your task needs.

## Try it

Ask your agent: "Use dbcli to list the schemas in my local Postgres."
Specify a profile if you already have one. The agent should ask for
connection details if it cannot identify an authorized connection.

You can also ask it to describe a table, sample rows, or answer a
question with SQL. See `references/output-shapes.md` for JSON fields
and value formats.

## Permissions

The CLI checks SQL and runs queries in read-only transactions, with
row limits and a statement timeout. The skill also respects the host
agent's permission controls. It does not authorize access to a database.

## License

MIT. See LICENSE in this package.
