# Contributing

Thanks for your interest in improving `nocodb-migrate`. This document describes
how to set up the project, the conventions the codebase follows, and how to get
a change merged.

## Prerequisites

- Go 1.21 or newer (the module targets `go 1.21`).
- A reachable NocoDB instance with an API token and a base id, for any change you
  want to exercise end to end.

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
go test ./...         # runs unit tests (e.g. the migration parser)
gofmt -l .            # must print nothing (formatting)
go vet ./...          # static checks
```

If you have [golangci-lint](https://golangci-lint.run/) installed, run it too —
it is the linter the project standardizes on.

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
