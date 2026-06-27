# Contributing

Thanks for your interest in improving `nocodb-migrate`. This document describes
how to set up the project, the conventions the codebase follows, and how to get
a change merged.

## Prerequisites

- Go 1.21 or newer (the module targets `go 1.21`).
- A reachable NocoDB instance with an API token and a base id is **only** needed
  to exercise the binary end to end — the unit test suite needs none (it drives
  an in-process mock NocoDB).
- Optional: [`golangci-lint`](https://golangci-lint.run/) and
  [`pre-commit`](https://pre-commit.com/) for the local checks below.
- Optional: a running Docker daemon, to run the integration suite
  (`go test -tags=integration ./...`).

## Getting started

```bash
git clone https://github.com/memclutter/nocodb-migrator.git
cd nocodb-migrator
go mod download
go build -o nocodb-migrate
```

Copy `.env.example` to `.env` and fill in your instance details to run the
binary against a real base:

```bash
cp .env.example .env
./nocodb-migrate info
```

## Development workflow

- Branch off `main` with a short-lived topic branch; keep one logical change per
  pull request.
- Run the checks below before pushing; CI must be green before review.

```bash
go build ./...        # compiles
go test ./...         # runs the unit suite (api / storage / executor, mock NocoDB)
gofmt -l .            # must print nothing (formatting)
golangci-lint run     # the linter the project standardizes on
```

CI runs the same `lint-unit` job (gofmt check, `golangci-lint run`,
`go test ./...`) on every push and pull request.

### Tests

The **unit suite** under `internal/` drives an in-process mock NocoDB
(`internal/testutil`), so `go test ./...` is fast, deterministic, and needs no
Docker or live instance. Add a test alongside any package you change; mock
responses must stay faithful to the documented Meta API v3 shapes.

The **integration suite** runs the real `up`/`down` command path against a
NocoDB container (via testcontainers-go) and is gated behind the `integration`
build tag, so it never runs in the default `go test ./...`:

```bash
go test -tags=integration ./...   # requires a running Docker daemon
```

It starts `nocodb/nocodb` (pinned in `internal/testutil`), bootstraps a token
and base, applies a migration, asserts the schema/data, and rolls back. Without
Docker the suite skips cleanly. A failure against real NocoDB means the
implementation diverges from the live Meta API v3 — treat it as a bug to fix,
not a test to weaken.

### pre-commit hooks

The repo ships a [`pre-commit`](https://pre-commit.com/) config that runs
`gofmt`, `golangci-lint`, and the unit tests on every commit. Enable it once:

```bash
pre-commit install
pre-commit run --all-files   # run the hooks across the whole tree
```

The hooks are `language: system`, so `go`, `gofmt`, and `golangci-lint` must be
on your `PATH`.

## Code style

- Format with `gofmt` / `goimports`; do not hand-format.
- Follow the existing layout: `cmd/` for cobra subcommands, `internal/api` for
  the NocoDB Meta API v3 client, `internal/migration` for parsing/validation and
  execution, `internal/storage` for the in-base `Migrations` table.
- Wrap errors with context (`fmt.Errorf("...: %w", err)`) rather than returning
  bare errors.
- Prefer table-driven tests; add a test alongside the package you change.

## Adding an operation or column type

The migration format is a contract. When adding a new operation type or NocoDB
column type:

1. Add it to the validation sets in `internal/migration/parser.go` (operation
   types / column types) and add the matching validation rules.
2. Implement execution in `internal/migration/operations.go`, wiring any new API
   call into `internal/api`.
3. Cover it with tests and document it in `README.md`.

## Commit messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(cmd): add a --dry-run flag to up
fix(api): handle empty where clause in delete_row
docs(readme): document the JSON migration format
```

Use `feat` / `fix` / `docs` / `refactor` / `test` / `chore` as appropriate; mark
breaking changes with a `!` or a `BREAKING CHANGE:` footer.

## Pull requests

- Describe what changes and why; link any related issue.
- Keep the diff focused and the history readable.
- Update `README.md` and `CHANGELOG.md` (under `## [Unreleased]`) when your
  change affects behaviour users can see.

## Releases

Releases follow [Semantic Versioning](https://semver.org/). While the major
version is `0`, the API and migration format may change in any release.
Maintainers cut releases by tagging `vMAJOR.MINOR.PATCH` and publishing notes
built from the changelog.
