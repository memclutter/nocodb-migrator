package storage_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/memclutter/nocodb-migrator/internal/api"
	"github.com/memclutter/nocodb-migrator/internal/storage"
	"github.com/memclutter/nocodb-migrator/internal/testutil"
)

// TestEnsureMigrationsTableCreatesWithExpectedFields guards the Migrations-table
// payload: the regression behind issue #1 is a malformed create request, so we
// assert the create call carries exactly the expected fields and types.
func TestEnsureMigrationsTableCreatesWithExpectedFields(t *testing.T) {
	f := testutil.NewFake(t)
	s := storage.NewMigrationsStorage(f.Client())

	require.NoError(t, s.EnsureMigrationsTable())

	// Find the POST /tables request and inspect its body.
	var create *testutil.RecordedRequest
	for i, r := range f.Requests() {
		if r.Method == "POST" && r.Path == "/api/v3/meta/bases/"+testutil.TestBaseID+"/tables" {
			rr := f.Requests()[i]
			create = &rr
			break
		}
	}
	require.NotNil(t, create, "expected a POST /tables call")

	var req api.TableCreate
	require.NoError(t, create.DecodeBody(&req))
	assert.Equal(t, "Migrations", req.Title)

	byTitle := map[string]string{}
	for _, fld := range req.Fields {
		byTitle[fld.Title] = fld.Type
	}
	assert.Equal(t, "Number", byTitle["Timestamp"])
	assert.Equal(t, "SingleLineText", byTitle["Name"])
	assert.Equal(t, "DateTime", byTitle["AppliedAt"])
	assert.Equal(t, "SingleSelect", byTitle["Direction"])
	assert.Equal(t, "SingleSelect", byTitle["Status"])
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
