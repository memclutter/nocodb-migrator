//go:build integration

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/memclutter/nocodb-migrator/internal/api"
	"github.com/memclutter/nocodb-migrator/internal/testutil"
)

const (
	upMigration = `{
  "operations": [
    {
      "type": "create_table",
      "table": "Widgets",
      "columns": [
        {"name": "Title", "type": "SingleLineText", "required": true},
        {"name": "Qty", "type": "Number"}
      ]
    },
    {"type": "insert_row", "table": "Widgets", "data": {"Title": "first", "Qty": 7}}
  ]
}`

	downMigration = `{
  "operations": [
    {"type": "drop_table", "table": "Widgets"}
  ]
}`
)

// TestUpDownAgainstRealNocoDB runs the real up/down command path against a
// dockerized NocoDB and asserts the resulting schema/data, then the rollback.
// It is the cross-check the unit mocks cannot give: a failure here means the
// implementation diverges from the live Meta API v3 (an issue-#2 finding), not
// that the test is wrong.
func TestUpDownAgainstRealNocoDB(t *testing.T) {
	nocodb := testutil.StartNocoDB(t)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "1000-create_widgets.up.json"), []byte(upMigration), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "1000-create_widgets.down.json"), []byte(downMigration), 0o644))

	t.Setenv("NOCODB_URL", nocodb.URL)
	t.Setenv("NOCODB_API_TOKEN", nocodb.Token)
	t.Setenv("NOCODB_BASE_ID", nocodb.BaseID)
	t.Setenv("NOCODB_MIGRATIONS_DIR", dir)

	client := api.NewClient(nocodb.URL, nocodb.Token, nocodb.BaseID)

	// --- up ---
	require.NoError(t, runUp(0), "up should apply the pending migration")

	table, err := client.GetTableByName("Widgets")
	require.NoError(t, err, "Widgets table should exist after up")
	titles := fieldTitles(table)
	assert.Contains(t, titles, "Title")
	assert.Contains(t, titles, "Qty")

	records, err := client.GetRecords(table.ID, 100, 0)
	require.NoError(t, err)
	require.Len(t, records.List, 1, "the inserted row should be present")
	assert.Equal(t, "first", records.List[0]["Title"])

	// The apply must be recorded in the Migrations table.
	migTable, err := client.GetTableByName("Migrations")
	require.NoError(t, err, "Migrations table should exist")
	migRecords, err := client.GetRecords(migTable.ID, 100, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, migRecords.List, "the apply should be recorded")

	// --- down ---
	require.NoError(t, runDown(0), "down should roll the migration back")

	_, err = client.GetTableByName("Widgets")
	require.Error(t, err, "Widgets table should be gone after down")
	assert.Contains(t, err.Error(), "not found")
}

func fieldTitles(table *api.Table) []string {
	titles := make([]string, 0, len(table.Fields))
	for _, f := range table.Fields {
		titles = append(titles, f.Title)
	}
	return titles
}
