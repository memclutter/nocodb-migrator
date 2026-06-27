package migration_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/memclutter/nocodb-migrator/internal/api"
	"github.com/memclutter/nocodb-migrator/internal/migration"
	"github.com/memclutter/nocodb-migrator/internal/testutil"
)

// seedTable creates a table with one field via the client so name-resolving
// operations have something to resolve against.
func seedTable(t *testing.T, c *api.Client, title, field, fieldType string) {
	t.Helper()
	_, err := c.CreateTable(&api.TableCreate{
		Title:  title,
		Fields: []api.FieldCreate{{Title: field, Type: fieldType}},
	})
	require.NoError(t, err)
}

// lastMutation returns the method+path of the last non-GET request.
func lastMutation(reqs []testutil.RecordedRequest) (string, string) {
	for i := len(reqs) - 1; i >= 0; i-- {
		if reqs[i].Method != "GET" {
			return reqs[i].Method, reqs[i].Path
		}
	}
	return "", ""
}

// TestExecuteOperationDispatch checks each operation type reaches the right
// endpoint via the executor.
func TestExecuteOperationDispatch(t *testing.T) {
	tests := []struct {
		name       string
		seed       func(t *testing.T, c *api.Client)
		op         migration.Operation
		wantMethod string
		wantPath   string
	}{
		{
			name: "create_table",
			op: migration.Operation{
				Type:    "create_table",
				Table:   "T",
				Columns: []migration.ColumnDefinition{{Name: "Name", Type: "SingleLineText"}},
			},
			wantMethod: "POST",
			wantPath:   "/api/v3/meta/bases/" + testutil.TestBaseID + "/tables",
		},
		{
			name: "alter_table",
			seed: func(t *testing.T, c *api.Client) { seedTable(t, c, "T", "Name", "SingleLineText") },
			op: migration.Operation{
				Type:  "alter_table",
				Table: "T",
				Data:  map[string]interface{}{"title": "T2"},
			},
			wantMethod: "PATCH",
			wantPath:   "/api/v3/meta/bases/" + testutil.TestBaseID + "/tables/tbl1",
		},
		{
			name: "drop_table",
			seed: func(t *testing.T, c *api.Client) { seedTable(t, c, "T", "Name", "SingleLineText") },
			op: migration.Operation{
				Type:  "drop_table",
				Table: "T",
			},
			wantMethod: "DELETE",
			wantPath:   "/api/v3/meta/bases/" + testutil.TestBaseID + "/tables/tbl1",
		},
		{
			name: "create_field",
			seed: func(t *testing.T, c *api.Client) { seedTable(t, c, "T", "Name", "SingleLineText") },
			op: migration.Operation{
				Type:   "create_field",
				Table:  "T",
				Column: &migration.ColumnDefinition{Name: "Age", Type: "Number"},
			},
			wantMethod: "POST",
			wantPath:   "/api/v3/meta/bases/" + testutil.TestBaseID + "/tables/tbl1/fields",
		},
		{
			name: "alter_field",
			seed: func(t *testing.T, c *api.Client) { seedTable(t, c, "T", "Name", "SingleLineText") },
			op: migration.Operation{
				Type:   "alter_field",
				Table:  "T",
				Column: &migration.ColumnDefinition{Name: "Name", Type: "LongText"},
			},
			wantMethod: "PATCH",
			wantPath:   "/api/v3/meta/bases/" + testutil.TestBaseID + "/fields/fld1",
		},
		{
			name: "drop_field",
			seed: func(t *testing.T, c *api.Client) { seedTable(t, c, "T", "Name", "SingleLineText") },
			op: migration.Operation{
				Type:   "drop_field",
				Table:  "T",
				Column: &migration.ColumnDefinition{Name: "Name", Type: "SingleLineText"},
			},
			wantMethod: "DELETE",
			wantPath:   "/api/v3/meta/bases/" + testutil.TestBaseID + "/fields/fld1",
		},
		{
			name: "insert_row",
			seed: func(t *testing.T, c *api.Client) { seedTable(t, c, "T", "Name", "SingleLineText") },
			op: migration.Operation{
				Type:  "insert_row",
				Table: "T",
				Data:  map[string]interface{}{"Name": "x"},
			},
			wantMethod: "POST",
			wantPath:   "/api/v3/data/" + testutil.TestBaseID + "/tbl1/records",
		},
		{
			name: "delete_row by id",
			seed: func(t *testing.T, c *api.Client) { seedTable(t, c, "T", "Name", "SingleLineText") },
			op: migration.Operation{
				Type:     "delete_row",
				Table:    "T",
				RecordID: "1",
			},
			wantMethod: "DELETE",
			wantPath:   "/api/v3/data/" + testutil.TestBaseID + "/tbl1/records",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := testutil.NewFake(t)
			c := f.Client()
			if tt.seed != nil {
				tt.seed(t, c)
			}
			e := migration.NewExecutor(c)

			op := tt.op
			require.NoError(t, e.ExecuteOperation(&op))

			method, path := lastMutation(f.Requests())
			assert.Equal(t, tt.wantMethod, method)
			assert.Equal(t, tt.wantPath, path)
		})
	}
}

func TestExecuteOperationUnknownType(t *testing.T) {
	f := testutil.NewFake(t)
	e := migration.NewExecutor(f.Client())

	op := migration.Operation{Type: "bogus"}
	err := e.ExecuteOperation(&op)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown operation type")
}
