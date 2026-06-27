//go:build integration

package storage_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/memclutter/nocodb-migrator/internal/api"
	"github.com/memclutter/nocodb-migrator/internal/storage"
	"github.com/memclutter/nocodb-migrator/internal/testutil"
)

// TestEnsureMigrationsTableAcrossBackends is the issue #1 regression guard: it
// creates the Migrations table against NocoDB running on SQLite, MySQL, and
// Postgres and asserts success and that Direction/Status are SingleSelect with
// their choices. On the pre-fix bulk-create path the MySQL and Postgres subtests
// fail (e.g. ER_PARSE_ERROR on MySQL from a value-less enum); all pass after the
// fix that adds the select fields via separate field-create calls.
func TestEnsureMigrationsTableAcrossBackends(t *testing.T) {
	for _, backend := range []testutil.Backend{testutil.SQLite, testutil.MySQL, testutil.Postgres} {
		backend := backend
		t.Run(string(backend), func(t *testing.T) {
			nocodb := testutil.StartNocoDBOn(t, backend)
			client := api.NewClient(nocodb.URL, nocodb.Token, nocodb.BaseID)

			require.NoError(t, storage.NewMigrationsStorage(client).EnsureMigrationsTable(),
				"EnsureMigrationsTable must succeed on %s", backend)

			table, err := client.GetTableByName("Migrations")
			require.NoError(t, err)

			byTitle := map[string]api.Field{}
			for _, f := range table.Fields {
				byTitle[f.Title] = f
			}
			for _, title := range []string{"Direction", "Status"} {
				f, ok := byTitle[title]
				require.True(t, ok, "%s field must exist on %s", title, backend)
				assert.Equal(t, "SingleSelect", f.Type, "%s must stay SingleSelect on %s", title, backend)
				assert.NotEmpty(t, f.Options["choices"], "%s must carry its choices on %s", title, backend)
			}
		})
	}
}
