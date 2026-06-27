package migration

import (
	"encoding/json"
	"fmt"
	"os"
)

// Migration represents a migration
type Migration struct {
	Operations []Operation `json:"operations"`
}

// Operation represents a migration operation
type Operation struct {
	Type     string                 `json:"type"`
	Table    string                 `json:"table,omitempty"`
	Columns  []ColumnDefinition     `json:"columns,omitempty"`
	Column   *ColumnDefinition      `json:"column,omitempty"`
	FieldID  string                 `json:"field_id,omitempty"`
	Data     map[string]interface{} `json:"data,omitempty"`
	Where    map[string]interface{} `json:"where,omitempty"`
	RecordID string                 `json:"record_id,omitempty"`
}

// ColumnDefinition represents a column definition
type ColumnDefinition struct {
	Name         string                 `json:"name"`
	Type         string                 `json:"type"`
	Required     bool                   `json:"required,omitempty"`
	Unique       bool                   `json:"unique,omitempty"`
	DefaultValue interface{}            `json:"default_value,omitempty"`
	Description  string                 `json:"description,omitempty"`
	Options      map[string]interface{} `json:"options,omitempty"`
}

// ValidOperationTypes is a list of valid operation types
var ValidOperationTypes = map[string]bool{
	"create_table": true,
	"alter_table":  true,
	"drop_table":   true,
	"create_field": true,
	"alter_field":  true,
	"drop_field":   true,
	"insert_row":   true,
	"delete_row":   true,
}

// ParseMigration parses a migration JSON file
func ParseMigration(filePath string) (*Migration, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read migration file: %w", err)
	}

	var migration Migration
	if err := json.Unmarshal(data, &migration); err != nil {
		return nil, fmt.Errorf("failed to parse migration JSON: %w", err)
	}

	if err := ValidateMigration(&migration); err != nil {
		return nil, fmt.Errorf("migration validation failed: %w", err)
	}

	return &migration, nil
}

// ValidateMigration validates the migration structure
func ValidateMigration(m *Migration) error {
	if len(m.Operations) == 0 {
		return fmt.Errorf("migration must contain at least one operation")
	}

	for i, op := range m.Operations {
		if !ValidOperationTypes[op.Type] {
			return fmt.Errorf("invalid operation type '%s' at index %d", op.Type, i)
		}

		if err := ValidateOperation(&op); err != nil {
			return fmt.Errorf("operation validation failed at index %d: %w", i, err)
		}
	}

	return nil
}

// ValidateOperation validates a single operation
func ValidateOperation(op *Operation) error {
	switch op.Type {
	case "create_table":
		if op.Table == "" {
			return fmt.Errorf("create_table operation requires 'table' field")
		}
		if len(op.Columns) == 0 {
			return fmt.Errorf("create_table operation requires at least one column")
		}
		for i, col := range op.Columns {
			if err := ValidateColumn(&col); err != nil {
				return fmt.Errorf("column validation failed at index %d: %w", i, err)
			}
		}

	case "alter_table":
		if op.Table == "" {
			return fmt.Errorf("alter_table operation requires 'table' field")
		}

	case "drop_table":
		if op.Table == "" {
			return fmt.Errorf("drop_table operation requires 'table' field")
		}

	case "create_field":
		if op.Table == "" {
			return fmt.Errorf("create_field operation requires 'table' field")
		}
		if op.Column == nil {
			return fmt.Errorf("create_field operation requires 'column' field")
		}
		if err := ValidateColumn(op.Column); err != nil {
			return fmt.Errorf("column validation failed: %w", err)
		}

	case "alter_field":
		if op.Table == "" {
			return fmt.Errorf("alter_field operation requires 'table' field")
		}
		if op.FieldID == "" && op.Column == nil {
			return fmt.Errorf("alter_field operation requires either 'field_id' or 'column.name' field")
		}
		if op.Column != nil {
			if err := ValidateColumn(op.Column); err != nil {
				return fmt.Errorf("column validation failed: %w", err)
			}
		}

	case "drop_field":
		if op.Table == "" {
			return fmt.Errorf("drop_field operation requires 'table' field")
		}
		if op.FieldID == "" && (op.Column == nil || op.Column.Name == "") {
			return fmt.Errorf("drop_field operation requires either 'field_id' or 'column.name' field")
		}

	case "insert_row":
		if op.Table == "" {
			return fmt.Errorf("insert_row operation requires 'table' field")
		}
		if len(op.Data) == 0 {
			return fmt.Errorf("insert_row operation requires 'data' field with at least one field")
		}

	case "delete_row":
		if op.Table == "" {
			return fmt.Errorf("delete_row operation requires 'table' field")
		}
		if op.RecordID == "" && len(op.Where) == 0 {
			return fmt.Errorf("delete_row operation requires either 'record_id' or 'where' field")
		}
	}

	return nil
}

// ValidateColumn validates a column definition
func ValidateColumn(col *ColumnDefinition) error {
	if col.Name == "" {
		return fmt.Errorf("column 'name' is required")
	}
	if col.Type == "" {
		return fmt.Errorf("column 'type' is required")
	}

	// Validate type
	validTypes := map[string]bool{
		"SingleLineText":      true,
		"LongText":            true,
		"Number":              true,
		"Decimal":             true,
		"Currency":            true,
		"Percent":             true,
		"DateTime":            true,
		"Date":                true,
		"Email":               true,
		"PhoneNumber":         true,
		"URL":                 true,
		"SingleSelect":        true,
		"MultiSelect":         true,
		"Checkbox":            true,
		"Rating":              true,
		"Attachment":          true,
		"JSON":                true,
		"LinkToAnotherRecord": true,
		"User":                true,
		"CreatedTime":         true,
		"CreatedBy":           true,
		"LastModifiedTime":    true,
		"LastModifiedBy":      true,
		"ID":                  true,
	}

	if !validTypes[col.Type] {
		return fmt.Errorf("invalid column type '%s'", col.Type)
	}

	return nil
}
