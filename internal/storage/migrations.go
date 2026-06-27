package storage

import (
	"fmt"
	"time"

	"github.com/memclutter/nocodb-migrator/internal/api"
)

// MigrationsStorage manages the migrations table in NocoDB
type MigrationsStorage struct {
	client    *api.Client
	tableName string
	tableID   string
}

// MigrationRecord represents a migration record in the Migrations table
type MigrationRecord struct {
	ID        int       `json:"Id"`
	Timestamp int64     `json:"Timestamp"`
	Name      string    `json:"Name"`
	AppliedAt time.Time `json:"AppliedAt"`
	Direction string    `json:"Direction"`
	Status    string    `json:"Status"`
	CreatedAt time.Time `json:"CreatedAt"`
}

// NewMigrationsStorage creates a new migrations storage
func NewMigrationsStorage(client *api.Client) *MigrationsStorage {
	return &MigrationsStorage{
		client:    client,
		tableName: "Migrations",
	}
}

// migrationsSelectFields returns the SingleSelect fields of the Migrations table.
// They are created via separate field-create calls rather than inline in the
// table-create request: a single bulk create does not materialize the select
// choices into the column on external SQL backends, so NocoDB emits a value-less
// enum (e.g. `Direction` enum) that MySQL rejects with a syntax error. Adding the
// fields after the table exists produces a proper enum('up','down') instead.
func migrationsSelectFields() []api.FieldCreate {
	return []api.FieldCreate{
		{
			Title:    "Direction",
			Type:     "SingleSelect",
			Required: true,
			Options: map[string]interface{}{
				"choices": []map[string]interface{}{
					{"title": "up", "color": "#36BFFF"},
					{"title": "down", "color": "#36BFFF"},
				},
			},
		},
		{
			Title:    "Status",
			Type:     "SingleSelect",
			Required: true,
			Options: map[string]interface{}{
				"choices": []map[string]interface{}{
					{"title": "success", "color": "#36BFFF"},
					{"title": "failed", "color": "#FF0000"},
				},
			},
		},
	}
}

// EnsureMigrationsTable creates the Migrations table if it doesn't exist. The
// SingleSelect fields are always added through separate field-create calls (see
// migrationsSelectFields), and any that are missing on an existing table are
// added, so a partially-created table heals on the next run.
func (s *MigrationsStorage) EnsureMigrationsTable() error {
	// Table already exists: make sure its select fields are present.
	if table, err := s.client.GetTableByName(s.tableName); err == nil {
		s.tableID = table.ID
		return s.ensureSelectFields(table)
	}

	// Create the table with only the non-select fields, then add the
	// SingleSelect fields separately (see migrationsSelectFields).
	baseFields := []api.FieldCreate{
		{Title: "Timestamp", Type: "Number", Required: true},
		{Title: "Name", Type: "SingleLineText", Required: true},
		{Title: "AppliedAt", Type: "DateTime", Required: true},
	}

	table, err := s.client.CreateTable(&api.TableCreate{Title: s.tableName, Fields: baseFields})
	if err != nil {
		return fmt.Errorf("failed to create Migrations table: %w", err)
	}
	s.tableID = table.ID

	return s.ensureSelectFields(table)
}

// ensureSelectFields adds any of the Migrations table's SingleSelect fields that
// are not already present on the given table.
func (s *MigrationsStorage) ensureSelectFields(table *api.Table) error {
	existing := make(map[string]bool, len(table.Fields))
	for _, f := range table.Fields {
		existing[f.Title] = true
	}

	for _, fc := range migrationsSelectFields() {
		if existing[fc.Title] {
			continue
		}
		fc := fc
		if _, err := s.client.CreateField(table.ID, &fc); err != nil {
			return fmt.Errorf("failed to add %s field to Migrations table: %w", fc.Title, err)
		}
	}
	return nil
}

// GetAppliedMigrations gets the list of applied migrations
func (s *MigrationsStorage) GetAppliedMigrations() ([]MigrationRecord, error) {
	if err := s.EnsureMigrationsTable(); err != nil {
		return nil, err
	}

	// Get table
	table, err := s.client.GetTableByName(s.tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to get Migrations table: %w", err)
	}

	// Get all records via Data API
	recordList, err := s.client.GetRecords(table.ID, 1000, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get migration records: %w", err)
	}

	// Convert records to MigrationRecord
	migrations := make([]MigrationRecord, 0, len(recordList.List))
	for _, record := range recordList.List {
		mr := MigrationRecord{}

		// Parse fields from record
		if id, ok := record["Id"].(float64); ok {
			mr.ID = int(id)
		}
		if timestamp, ok := record["Timestamp"].(float64); ok {
			mr.Timestamp = int64(timestamp)
		}
		if name, ok := record["Name"].(string); ok {
			mr.Name = name
		}
		if appliedAt, ok := record["AppliedAt"].(string); ok {
			if t, err := time.Parse(time.RFC3339, appliedAt); err == nil {
				mr.AppliedAt = t
			}
		}
		if direction, ok := record["Direction"].(string); ok {
			mr.Direction = direction
		}
		if status, ok := record["Status"].(string); ok {
			mr.Status = status
		}
		if createdAt, ok := record["CreatedAt"].(string); ok {
			if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
				mr.CreatedAt = t
			}
		}

		migrations = append(migrations, mr)
	}

	return migrations, nil
}

// RecordMigration records information about a migration application
func (s *MigrationsStorage) RecordMigration(timestamp int64, name, direction, status string) error {
	if err := s.EnsureMigrationsTable(); err != nil {
		return err
	}

	record := api.Record{
		"Timestamp": timestamp,
		"Name":      name,
		"AppliedAt": time.Now().Format(time.RFC3339),
		"Direction": direction,
		"Status":    status,
	}

	_, err := s.client.InsertRecord(s.tableID, record)
	return err
}

// GetCurrentVersion gets the current version (timestamp of the last applied migration)
func (s *MigrationsStorage) GetCurrentVersion() (int64, string, error) {
	migrations, err := s.GetAppliedMigrations()
	if err != nil {
		return 0, "", err
	}

	if len(migrations) == 0 {
		return 0, "", nil
	}

	// Find the last successful migration in "up" direction
	var lastMigration *MigrationRecord
	for i := range migrations {
		if migrations[i].Direction == "up" && migrations[i].Status == "success" {
			if lastMigration == nil || migrations[i].Timestamp > lastMigration.Timestamp {
				lastMigration = &migrations[i]
			}
		}
	}

	if lastMigration == nil {
		return 0, "", nil
	}

	return lastMigration.Timestamp, lastMigration.Name, nil
}

// IsMigrationApplied checks if a migration is applied
func (s *MigrationsStorage) IsMigrationApplied(timestamp int64, name string) (bool, error) {
	migrations, err := s.GetAppliedMigrations()
	if err != nil {
		return false, err
	}

	for _, m := range migrations {
		if m.Timestamp == timestamp && m.Name == name && m.Status == "success" {
			return true, nil
		}
	}

	return false, nil
}

// DeleteMigrationRecord deletes a migration record from the Migrations table
func (s *MigrationsStorage) DeleteMigrationRecord(timestamp int64, name string) error {
	if err := s.EnsureMigrationsTable(); err != nil {
		return err
	}

	// Get table
	table, err := s.client.GetTableByName(s.tableName)
	if err != nil {
		return fmt.Errorf("failed to get Migrations table: %w", err)
	}

	// Get all records
	recordList, err := s.client.GetRecords(table.ID, 1000, 0)
	if err != nil {
		return fmt.Errorf("failed to get migration records: %w", err)
	}

	// Find record with matching timestamp and name (only with "up" direction)
	for _, record := range recordList.List {
		recordTimestamp, ok1 := record["Timestamp"].(float64)
		recordName, ok2 := record["Name"].(string)
		recordDirection, ok4 := record["Direction"].(string)
		// Try to get ID in different formats (Id or id)
		recordIDRaw, ok3 := record["Id"]
		if !ok3 {
			recordIDRaw, ok3 = record["id"]
		}

		if ok1 && ok2 && ok3 && ok4 {
			if int64(recordTimestamp) == timestamp && recordName == name && recordDirection == "up" {
				// Convert ID to string
				var idStr string
				switch v := recordIDRaw.(type) {
				case float64:
					idStr = fmt.Sprintf("%.0f", v)
				case string:
					idStr = v
				case int:
					idStr = fmt.Sprintf("%d", v)
				case int64:
					idStr = fmt.Sprintf("%d", v)
				default:
					return fmt.Errorf("invalid record ID type: %T", v)
				}
				// Delete record by ID
				return s.client.DeleteRecord(table.ID, idStr)
			}
		}
	}

	return fmt.Errorf("migration record not found: %d-%s", timestamp, name)
}
