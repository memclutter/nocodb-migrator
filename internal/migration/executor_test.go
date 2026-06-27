package migration_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/memclutter/nocodb-migrator/internal/migration"
	"github.com/memclutter/nocodb-migrator/internal/testutil"
)

// TestExecuteMigrationRunsAllOperations applies a two-operation migration and
// confirms both reach the server.
func TestExecuteMigrationRunsAllOperations(t *testing.T) {
	f := testutil.NewFake(t)
	c := f.Client()
	e := migration.NewExecutor(c)

	m := &migration.Migration{
		Operations: []migration.Operation{
			{
				Type:    "create_table",
				Table:   "T",
				Columns: []migration.ColumnDefinition{{Name: "Name", Type: "SingleLineText"}},
			},
			{
				Type:  "insert_row",
				Table: "T",
				Data:  map[string]interface{}{"Name": "x"},
			},
		},
	}

	require.NoError(t, e.ExecuteMigration(m))

	var posts int
	for _, r := range f.Requests() {
		if r.Method == "POST" {
			posts++
		}
	}
	assert.Equal(t, 2, posts, "both create_table and insert_row should POST")
}

// TestExecuteMigrationStopsOnFirstFailure asserts execution aborts on the first
// failing operation and wraps it with its index.
func TestExecuteMigrationStopsOnFirstFailure(t *testing.T) {
	f := testutil.NewFake(t)
	e := migration.NewExecutor(f.Client())

	m := &migration.Migration{
		Operations: []migration.Operation{
			{Type: "bogus"},
			{
				Type:    "create_table",
				Table:   "T",
				Columns: []migration.ColumnDefinition{{Name: "Name", Type: "SingleLineText"}},
			},
		},
	}

	err := e.ExecuteMigration(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operation 1")

	// The second operation must not have run.
	for _, r := range f.Requests() {
		assert.NotEqual(t, "POST", r.Method, "no operation should have executed after the failure")
	}
}
