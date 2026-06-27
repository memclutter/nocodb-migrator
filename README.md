# nocodb-migrator

> Versioned, file-based migrations for [NocoDB](https://nocodb.com) — the
> `up` / `down` / `info` workflow you know from SQL migration tools, driven
> through NocoDB's Meta API v3.

[![Release](https://img.shields.io/github/v/release/memclutter/nocodb-migrator?sort=semver)](https://github.com/memclutter/nocodb-migrator/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/memclutter/nocodb-migrator.svg)](https://pkg.go.dev/github.com/memclutter/nocodb-migrator)
[![Go Report Card](https://goreportcard.com/badge/github.com/memclutter/nocodb-migrator)](https://goreportcard.com/report/github.com/memclutter/nocodb-migrator)
[![Go version](https://img.shields.io/github/go-mod/go-version/memclutter/nocodb-migrator)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

`nocodb-migrate` is a small command-line tool that brings repeatable, ordered
schema and data migrations to a NocoDB base. You describe each change as a pair
of timestamped JSON files (`*.up.json` / `*.down.json`), apply them in order,
and roll them back when needed. The history of what has run is tracked in a
`Migrations` table the tool keeps inside the base itself — there is no external
state store to manage.

- **Ordered & idempotent** — migrations apply oldest-first and never run twice.
- **Reversible** — every migration ships a `down` counterpart.
- **No extra infrastructure** — state lives in the target base over Meta API v3.
- **Single binary** — written in Go, configured entirely from the environment.

## Contents

- [Install](#install)
- [Configure](#configure)
- [Usage](#usage)
- [Migration format](#migration-format)
  - [Operations](#operations)
  - [Column types](#column-types)
  - [Operation examples](#operation-examples)
- [How state is tracked](#how-state-is-tracked)
- [Meta API v3 endpoints used](#meta-api-v3-endpoints-used)
- [Contributing](#contributing)
- [License](#license)

## Install

Requires Go 1.21+.

```bash
go install github.com/memclutter/nocodb-migrator@latest
# the binary is named `nocodb-migrate`
```

Or build from source:

```bash
git clone https://github.com/memclutter/nocodb-migrator.git
cd nocodb-migrator
go build -o nocodb-migrate
```

## Configure

Configuration is read from the environment. Copy `.env.example` to `.env` (it is
loaded automatically from the working directory), or export the variables
directly:

```env
NOCODB_URL=http://localhost:8080      # required — your NocoDB instance URL
NOCODB_API_TOKEN=your_api_token_here  # required — API token (sent as xc-token)
NOCODB_BASE_ID=your_base_id_here      # required — target base id
NOCODB_MIGRATIONS_DIR=./migrations    # optional — defaults to ./migrations
```

The three required variables must be set or any command that talks to NocoDB
exits with an error naming the missing one.

## Usage

```bash
# scaffold a new migration pair (does not touch NocoDB)
nocodb-migrate create add_users_table
#   -> migrations/{timestamp}-add_users_table.up.json
#   -> migrations/{timestamp}-add_users_table.down.json

# apply migrations (oldest first)
nocodb-migrate up        # all pending
nocodb-migrate up 3      # at most 3

# roll back (newest first)
nocodb-migrate down 1    # the latest applied migration
nocodb-migrate down 3    # the last 3

# show the current version and recorded history
nocodb-migrate info
```

`up` applies only migrations newer than the current version that have not already
succeeded; running it again is a no-op. `down` reverses applied migrations in
descending order and removes their tracking rows.

## Migration format

A migration file is a JSON object with an `operations` array, executed top to
bottom. The same shape is used for both `*.up.json` and `*.down.json` — the
direction is conveyed by the file name. Operations are validated before any of
them runs, so an invalid file fails without partially applying.

```json
{
  "operations": [
    {
      "type": "create_table",
      "table": "Users",
      "columns": [
        { "name": "Id", "type": "ID", "required": true },
        { "name": "Name", "type": "SingleLineText", "required": true },
        { "name": "Email", "type": "Email", "required": true, "unique": true },
        { "name": "Age", "type": "Number", "default_value": 0 }
      ]
    },
    {
      "type": "create_field",
      "table": "Users",
      "column": { "name": "Bio", "type": "LongText" }
    }
  ]
}
```

The matching `down.json` reverses it:

```json
{
  "operations": [
    { "type": "drop_table", "table": "Users" }
  ]
}
```

### Operations

| Type | Purpose | Key fields |
| --- | --- | --- |
| `create_table` | Create a table | `table`, `columns` (≥1) |
| `alter_table` | Rename / re-describe a table | `table`, `data.title`, `data.description` |
| `drop_table` | Delete a table | `table` |
| `create_field` | Add a field | `table`, `column` |
| `alter_field` | Modify a field | `table`, `field_id` **or** `column.name` |
| `drop_field` | Remove a field | `table`, `field_id` **or** `column.name` |
| `insert_row` | Insert a record | `table`, `data` (≥1 field) |
| `delete_row` | Delete record(s) | `table`, `record_id` **or** `where` |

A column definition has `name` and `type` (both required) plus optional
`required`, `unique`, `default_value`, `description`, and `options`.

### Column types

`SingleLineText`, `LongText`, `Number`, `Decimal`, `Currency`, `Percent`,
`DateTime`, `Date`, `Email`, `PhoneNumber`, `URL`, `SingleSelect`, `MultiSelect`,
`Checkbox`, `Rating`, `Attachment`, `JSON`, `LinkToAnotherRecord`, `User`,
`CreatedTime`, `CreatedBy`, `LastModifiedTime`, `LastModifiedBy`, `ID`.

A `LinkToAnotherRecord` column takes its target via `options.relatedTable` (a
table name, resolved to an id at apply time); `relation_type` defaults to `hm`
(has-many) when omitted.

### Operation examples

Create a table with a currency field:

```json
{
  "type": "create_table",
  "table": "Products",
  "columns": [
    { "name": "Id", "type": "ID", "required": true },
    { "name": "Title", "type": "SingleLineText", "required": true },
    { "name": "Price", "type": "Currency", "required": true, "options": { "code": "USD" } }
  ]
}
```

Alter a table:

```json
{ "type": "alter_table", "table": "Products", "data": { "title": "NewProducts", "description": "Updated" } }
```

Add, modify, and drop a field (by id or by name):

```json
{ "type": "create_field", "table": "Products", "column": { "name": "Description", "type": "LongText" } }
{ "type": "alter_field", "table": "Products", "column": { "name": "Description", "required": true } }
{ "type": "drop_field", "table": "Products", "column": { "name": "Description" } }
```

Insert and delete data (delete by id or by condition):

```json
{ "type": "insert_row", "table": "Products", "data": { "Title": "Product 1", "Price": 99.99 } }
{ "type": "delete_row", "table": "Products", "record_id": "42" }
{ "type": "delete_row", "table": "Products", "where": { "Title": "Product 1" } }
```

## How state is tracked

On first use the tool creates a `Migrations` table in the base (if absent) and
records each applied migration there:

| Field | Type | Meaning |
| --- | --- | --- |
| `Id` | Integer (PK) | Record id |
| `Timestamp` | Number | Timestamp from the file name (the ordering key) |
| `Name` | SingleLineText | Migration name |
| `AppliedAt` | DateTime | When the row was recorded |
| `Direction` | SingleSelect | `up` or `down` |
| `Status` | SingleSelect | `success` or `failed` |

The current version is the highest `Timestamp` among successful `up` rows.

## Meta API v3 endpoints used

All requests are scoped to `NOCODB_BASE_ID` and authenticate with the `xc-token`
header.

- **Tables** — `GET`/`POST /api/v3/meta/bases/{baseId}/tables`,
  `GET`/`PATCH`/`DELETE …/tables/{tableId}`
- **Fields** — `POST …/tables/{tableId}/fields`,
  `GET`/`PATCH`/`DELETE …/fields/{fieldId}`
- **Records** — `GET`/`POST`/`DELETE /api/v3/data/{baseId}/{tableId}/records`

## Contributing

Contributions are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md) for setup,
coding conventions, and the commit/PR process. Changes are recorded in
[CHANGELOG.md](CHANGELOG.md).

## License

Released under the [MIT License](LICENSE).
