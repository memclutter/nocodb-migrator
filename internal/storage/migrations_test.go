package storage_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/memclutter/nocodb-migrator/internal/api"
	"github.com/memclutter/nocodb-migrator/internal/storage"
	"github.com/memclutter/nocodb-migrator/internal/testutil"
)

// TestEnsureMigrationsTableCreatesTableThenAddsSelectFields guards the issue #1
// fix: the bulk create must carry only the non-select fields, and Direction /
// Status must be added as separate SingleSelect field-create calls (with their
// choices) so the enum materializes on external SQL backends.
func TestEnsureMigrationsTableCreatesTableThenAddsSelectFields(t *testing.T) {
	f := testutil.NewFake(t)
	s := storage.NewMigrationsStorage(f.Client())

	require.NoError(t, s.EnsureMigrationsTable())

	var create *testutil.RecordedRequest
	var fieldReqs []testutil.RecordedRequest
	for _, r := range f.Requests() {
		switch {
		case r.Method == "POST" && r.Path == "/api/v3/meta/bases/"+testutil.TestBaseID+"/tables":
			rr := r
			create = &rr
		case r.Method == "POST" && strings.HasSuffix(r.Path, "/fields"):
			fieldReqs = append(fieldReqs, r)
		}
	}

	// The table is created with only the non-select fields.
	require.NotNil(t, create, "expected a POST /tables call")
	var tc api.TableCreate
	require.NoError(t, create.DecodeBody(&tc))
	assert.Equal(t, "Migrations", tc.Title)
	types := map[string]string{}
	for _, fld := range tc.Fields {
		types[fld.Title] = fld.Type
	}
	assert.Equal(t, map[string]string{
		"Timestamp": "Number",
		"Name":      "SingleLineText",
		"AppliedAt": "DateTime",
	}, types, "bulk create must carry only the non-select fields")

	// Direction and Status are added as separate SingleSelect field-create calls.
	require.Len(t, fieldReqs, 2, "Direction and Status must be added separately")
	selects := map[string]api.FieldCreate{}
	for _, r := range fieldReqs {
		var fc api.FieldCreate
		require.NoError(t, r.DecodeBody(&fc))
		selects[fc.Title] = fc
	}
	for _, title := range []string{"Direction", "Status"} {
		fc, ok := selects[title]
		require.True(t, ok, "expected a field-create for %s", title)
		assert.Equal(t, "SingleSelect", fc.Type)
		assert.NotEmpty(t, fc.Options["choices"], "%s must carry its choices", title)
	}
}

// TestEnsureMigrationsTableHealsMissingSelectFields covers a partially-created
// table (exists, but missing the select fields): a re-run must add them.
func TestEnsureMigrationsTableHealsMissingSelectFields(t *testing.T) {
	f := testutil.NewFake(t)
	c := f.Client()

	_, err := c.CreateTable(&api.TableCreate{Title: "Migrations", Fields: []api.FieldCreate{
		{Title: "Timestamp", Type: "Number"},
		{Title: "Name", Type: "SingleLineText"},
		{Title: "AppliedAt", Type: "DateTime"},
	}})
	require.NoError(t, err)

	require.NoError(t, storage.NewMigrationsStorage(c).EnsureMigrationsTable())

	table, err := c.GetTableByName("Migrations")
	require.NoError(t, err)
	titles := map[string]string{}
	for _, fld := range table.Fields {
		titles[fld.Title] = fld.Type
	}
	assert.Equal(t, "SingleSelect", titles["Direction"])
	assert.Equal(t, "SingleSelect", titles["Status"])
}

// TestEnsureMigrationsTableIsIdempotent checks a second call does not create the
// table again when it already exists.
func TestEnsureMigrationsTableIsIdempotent(t *testing.T) {
	f := testutil.NewFake(t)
	s := storage.NewMigrationsStorage(f.Client())

	require.NoError(t, s.EnsureMigrationsTable())
	require.NoError(t, s.EnsureMigrationsTable())

	creates := 0
	for _, r := range f.Requests() {
		if r.Method == "POST" && r.Path == "/api/v3/meta/bases/"+testutil.TestBaseID+"/tables" {
			creates++
		}
	}
	assert.Equal(t, 1, creates, "table must be created at most once")
}

// TestRecordAndQueryMigrations round-trips records through the in-base store.
func TestRecordAndQueryMigrations(t *testing.T) {
	f := testutil.NewFake(t)
	s := storage.NewMigrationsStorage(f.Client())

	require.NoError(t, s.RecordMigration(1000, "init", "up", "success"))
	require.NoError(t, s.RecordMigration(2000, "add_users", "up", "success"))

	applied, err := s.GetAppliedMigrations()
	require.NoError(t, err)
	require.Len(t, applied, 2)

	ok, err := s.IsMigrationApplied(2000, "add_users")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = s.IsMigrationApplied(3000, "missing")
	require.NoError(t, err)
	assert.False(t, ok)

	ts, name, err := s.GetCurrentVersion()
	require.NoError(t, err)
	assert.EqualValues(t, 2000, ts)
	assert.Equal(t, "add_users", name)
}

// TestDeleteMigrationRecord removes the matching up-direction record.
func TestDeleteMigrationRecord(t *testing.T) {
	f := testutil.NewFake(t)
	s := storage.NewMigrationsStorage(f.Client())

	require.NoError(t, s.RecordMigration(1000, "init", "up", "success"))
	require.NoError(t, s.RecordMigration(2000, "add_users", "up", "success"))

	require.NoError(t, s.DeleteMigrationRecord(2000, "add_users"))

	ts, name, err := s.GetCurrentVersion()
	require.NoError(t, err)
	assert.EqualValues(t, 1000, ts)
	assert.Equal(t, "init", name)
}

func TestDeleteMigrationRecordNotFound(t *testing.T) {
	f := testutil.NewFake(t)
	s := storage.NewMigrationsStorage(f.Client())

	require.NoError(t, s.EnsureMigrationsTable())
	err := s.DeleteMigrationRecord(9999, "nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
