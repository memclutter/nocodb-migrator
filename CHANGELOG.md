# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
While the major version is `0`, the API and migration format may change in any
release.

## [Unreleased]

## [0.0.2] - 2026-06-27

### Added

- Unit test suite over the NocoDB API client, the `Migrations` storage, and the
  migration executor, driven by an in-process mock NocoDB (`internal/testutil`)
  so it needs no live instance.
- GitHub Actions CI (`lint-unit`): gofmt check, `golangci-lint`, and unit tests
  on every push and pull request, with a committed `.golangci.yml`.
- A `pre-commit` configuration running gofmt, `golangci-lint`, and the unit tests
  on every commit.
- Integration tests that run the real `up`/`down` command path against a
  dockerized NocoDB (testcontainers-go), gated behind the `integration` build tag
  and run in a separate CI job — parameterized across SQLite, MySQL, and
  PostgreSQL backends.

### Fixed

- Creating the `Migrations` table now succeeds on external SQL backends (MySQL,
  PostgreSQL). The `Direction`/`Status` SingleSelect fields are added via
  separate field-create calls so NocoDB materializes a valid `enum(...)` column
  instead of failing with `ER_PARSE_ERROR` ("There was a syntax error in your
  SQL query"). The underlying NocoDB bulk-create limitation is tracked upstream
  in nocodb/nocodb#14164. (#1)

### Changed

- Relicensed the project from GPL-3.0 to the MIT License.
- Reworked `README.md`: status badges, quick-start, an operations table, and
  tightened reference sections.

## [0.0.1] - 2026-06-27

First release of `nocodb-migrate` — a command-line migration tool for NocoDB,
driving schema and data changes against a base through its Meta API v3.

### Added

- `create <name>` command that scaffolds a timestamped `*.up.json` /
  `*.down.json` migration file pair in the migrations directory.
- `up [count]` command that applies pending migrations in ascending timestamp
  order, `down [count]` that rolls them back newest-first, and `info` that
  reports the current version and recorded history.
- Eight migration operation types — `create_table`, `alter_table`,
  `drop_table`, `create_field`, `alter_field`, `drop_field`, `insert_row`,
  `delete_row` — with up-front JSON validation of operations and column types.
- Tracking of applied migrations in a `Migrations` table the tool creates inside
  the target base, requiring no external state store.
- Environment-based configuration (`NOCODB_URL`, `NOCODB_API_TOKEN`,
  `NOCODB_BASE_ID`, optional `NOCODB_MIGRATIONS_DIR`) with `.env` support.

[Unreleased]: https://github.com/memclutter/nocodb-migrator/compare/v0.0.2...HEAD
[0.0.2]: https://github.com/memclutter/nocodb-migrator/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/memclutter/nocodb-migrator/releases/tag/v0.0.1
