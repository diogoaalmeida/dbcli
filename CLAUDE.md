# CLAUDE.md

Guidance for working on this repository's own code. If you're looking
for how to *use* the compiled `dbcli` tool from another project, see
`README.md` or the bundled `skills/dbcli/SKILL.md` instead — this file
is for developing dbcli itself.

It's also symlinked as `AGENTS.md` for any other tool that looks for
that name instead — same content either way.

## What this is

A read-only SQL CLI for Postgres, built for AI agents. Every query is
classified, then runs inside a Postgres `READ ONLY` transaction
regardless of what the classifier decided, with a hard row cap and a
statement timeout. See `README.md`'s "Why this is safe to point at a
real database" section for the full guarantee.

## Build and test

```bash
go build ./...
go vet ./...
gofmt -l .                 # must be empty
golangci-lint run ./...    # must report 0 issues; config in .golangci.yml
go test ./...
```

CI installs golangci-lint v2.14.0 with `go install`, so it is always
built with the Go version from `go.mod` (a golangci-lint built with an
older Go refuses to lint a newer module). Use a build of the same
version or newer locally, and bump the pin in `ci.yml` when `go.mod`'s
Go version moves past what that release supports.

Most of the test suite needs a real Postgres instance:

```bash
export DBCLI_TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
```

Tests skip themselves (`t.Skip`) when that's unset. CI runs the full
suite against Postgres 13 through 18 (`.github/workflows/ci.yml`); a
`lint` job (`gofmt`+`go vet`+`go build`+`golangci-lint`) gates the matrix.

For any throwaway manual verification, spin up a local Postgres rather
than reasoning about behavior from memory:

```bash
docker run -d --name dbcli-dev -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:15
```

## Project conventions

- **Never fabricate output.** Every claim in the README's worked
  example, every bug-fix claim, and every code-review finding gets
  verified against a real Postgres before being accepted as true or
  committed as fixed. This has caught real mistakes, including
  self-authored ones — don't skip it.
- **Commit in small, logical steps**, not one large commit per
  feature. Each phase of work ships as its own branch/PR.
- **Fix lint findings, don't hide them.** Don't loosen `.golangci.yml`
  or add `//nolint` to get a clean run. If a finding really can't be
  fixed, annotate it with a reason, as `stripComments` does for
  `gocyclo`.
- **Commit messages are a single title line:**
  `<area>: <lowercase imperative summary>`, no trailing period, e.g.
  `ci:`, `docs:`, `test:`, `postgres:`, `cmd:`, `lint:`. No body.
- **Safety is layered on purpose.** The SQL classifier
  (`internal/postgres/dbsafety.go`) and the read-only transaction
  (`internal/postgres/exec.go`) are independent defenses — a gap in one
  must not mean a write gets through. Don't remove either on the
  assumption the other covers it.
- **No raw identifier interpolation.** Table/schema names are checked
  against the catalog and quoted with `pgx.Identifier`, never
  string-interpolated into SQL.
- Test fixture credentials are always throwaway/local
  (`postgres:postgres@localhost`) — never real credentials, even in
  manual verification.

## Layout

- `cmd/` — one file per CLI subcommand; thin wrappers over `internal/`.
- `internal/driver/` — the engine-neutral `Conn`/`Driver` interfaces.
- `internal/postgres/` — the only driver implementation today.
- `internal/config/`, `internal/output/` — profile storage, JSON/table
  rendering.
- `.claude-plugin/`, `skills/` — the Claude Code plugin bundled in this
  same repo (see `skills/dbcli/SKILL.md`), so dbcli's GitHub repo is
  directly installable as a plugin.
