package migration

import (
	"os"
	"testing"
)

func TestParseMigration(t *testing.T) {
	// Create temporary migration file
	tmpFile, err := os.CreateTemp("", "test-migration-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	migrationContent := `{
  "operations": [
    {
      "type": "create_table",
      "table": "TestTable",
      "columns": [
        {
          "name": "Id",
          "type": "ID",
          "required": true
        },
        {
          "name": "Name",
          "type": "SingleLineText",
          "required": true
        }
      ]
    }
  ]
}
`

	if _, err := tmpFile.WriteString(migrationContent); err != nil {
		t.Fatalf("Failed to write migration content: %v", err)
	}
	_ = tmpFile.Close()

	// Parse migration
	migration, err := ParseMigration(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to parse migration: %v", err)
	}

	// Check result
	if len(migration.Operations) != 1 {
		t.Fatalf("Expected 1 operation, got %d", len(migration.Operations))
	}

	op := migration.Operations[0]
	if op.Type != "create_table" {
		t.Errorf("Expected operation type 'create_table', got '%s'", op.Type)
	}
	if op.Table != "TestTable" {
		t.Errorf("Expected table name 'TestTable', got '%s'", op.Table)
	}
	if len(op.Columns) != 2 {
		t.Errorf("Expected 2 columns, got %d", len(op.Columns))
	}
}

func TestValidateMigration(t *testing.T) {
	tests := []struct {
		name      string
		migration *Migration
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid migration",
			migration: &Migration{
				Operations: []Operation{
					{
						Type:  "create_table",
						Table: "TestTable",
						Columns: []ColumnDefinition{
							{Name: "Id", Type: "ID", Required: true},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "empty operations",
			migration: &Migration{
				Operations: []Operation{},
			},
			wantError: true,
		},
		{
			name: "invalid operation type",
			migration: &Migration{
				Operations: []Operation{
					{Type: "invalid_type"},
				},
			},
			wantError: true,
		},
		{
			name: "create_table without table name",
			migration: &Migration{
				Operations: []Operation{
					{
						Type: "create_table",
						Columns: []ColumnDefinition{
							{Name: "Id", Type: "ID"},
						},
					},
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMigration(tt.migration)
			if tt.wantError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestValidateColumn(t *testing.T) {
	tests := []struct {
		name      string
		column    *ColumnDefinition
		wantError bool
	}{
		{
			name: "valid column",
			column: &ColumnDefinition{
				Name: "TestColumn",
				Type: "SingleLineText",
			},
			wantError: false,
		},
		{
			name: "missing name",
			column: &ColumnDefinition{
				Type: "SingleLineText",
			},
			wantError: true,
		},
		{
			name: "missing type",
			column: &ColumnDefinition{
				Name: "TestColumn",
			},
			wantError: true,
		},
		{
			name: "invalid type",
			column: &ColumnDefinition{
				Name: "TestColumn",
				Type: "InvalidType",
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateColumn(tt.column)
			if tt.wantError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}
