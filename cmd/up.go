package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/memclutter/nocodb-migrator/internal/migration"
	"github.com/memclutter/nocodb-migrator/internal/storage"
)

// NewUpCommand creates the up command
func NewUpCommand() *cobra.Command {
	var count int

	cmd := &cobra.Command{
		Use:   "up [count]",
		Short: "Apply migrations",
		Long:  "Apply pending migrations. If count is specified, apply only that many migrations.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				var err error
				count, err = strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("invalid count: %w", err)
				}
			}
			return runUp(count)
		},
	}

	return cmd
}

func runUp(count int) error {
	// Initialize client
	client, err := initClient()
	if err != nil {
		return err
	}

	// Initialize storage
	storage := storage.NewMigrationsStorage(client)
	if err := storage.EnsureMigrationsTable(); err != nil {
		return fmt.Errorf("failed to ensure migrations table: %w", err)
	}

	// Get list of migration files
	migrationsDir := getMigrationsDir()
	migrationFiles, err := getMigrationFiles(migrationsDir, "up")
	if err != nil {
		return fmt.Errorf("failed to get migration files: %w", err)
	}

	if len(migrationFiles) == 0 {
		fmt.Println("No migrations found")
		return nil
	}

	// Get current version
	currentTimestamp, _, err := storage.GetCurrentVersion()
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	// Filter migrations that haven't been applied yet
	pendingMigrations := []MigrationFile{}
	for _, mf := range migrationFiles {
		applied, err := storage.IsMigrationApplied(mf.Timestamp, mf.Name)
		if err != nil {
			return fmt.Errorf("failed to check migration status: %w", err)
		}
		if !applied && mf.Timestamp > currentTimestamp {
			pendingMigrations = append(pendingMigrations, mf)
		}
	}

	if len(pendingMigrations) == 0 {
		fmt.Println("No pending migrations")
		return nil
	}

	// Limit count if specified
	if count > 0 && count < len(pendingMigrations) {
		pendingMigrations = pendingMigrations[:count]
	}

	// Create executor
	executor := migration.NewExecutor(client)

	// Apply migrations
	for _, mf := range pendingMigrations {
		fmt.Printf("Applying migration: %s\n", mf.Name)

		// Parse migration
		mig, err := migration.ParseMigration(mf.Path)
		if err != nil {
			// Record error (best effort)
			_ = storage.RecordMigration(mf.Timestamp, mf.Name, "up", "failed")
			return fmt.Errorf("failed to parse migration %s: %w", mf.Name, err)
		}

		// Execute migration
		if err := executor.ExecuteMigration(mig); err != nil {
			// Record error (best effort)
			_ = storage.RecordMigration(mf.Timestamp, mf.Name, "up", "failed")
			return fmt.Errorf("failed to execute migration %s: %w", mf.Name, err)
		}

		// Record success
		if err := storage.RecordMigration(mf.Timestamp, mf.Name, "up", "success"); err != nil {
			return fmt.Errorf("failed to record migration: %w", err)
		}

		fmt.Printf("Migration %s applied successfully\n", mf.Name)
	}

	fmt.Printf("Applied %d migration(s)\n", len(pendingMigrations))
	return nil
}

// MigrationFile represents a migration file
type MigrationFile struct {
	Timestamp int64
	Name      string
	Path      string
	Direction string
}

func getMigrationFiles(dir string, direction string) ([]MigrationFile, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var migrationFiles []MigrationFile

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		if !strings.HasSuffix(name, fmt.Sprintf(".%s.json", direction)) {
			continue
		}

		// Parse file name: {timestamp}-{name}.{direction}.json
		parts := strings.Split(name, "-")
		if len(parts) < 2 {
			continue
		}

		timestampStr := parts[0]
		timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
		if err != nil {
			continue
		}

		// Remove extension and direction
		namePart := strings.TrimSuffix(name, fmt.Sprintf(".%s.json", direction))
		migrationName := strings.TrimPrefix(namePart, timestampStr+"-")

		migrationFiles = append(migrationFiles, MigrationFile{
			Timestamp: timestamp,
			Name:      migrationName,
			Path:      filepath.Join(dir, name),
			Direction: direction,
		})
	}

	// Sort by timestamp
	sort.Slice(migrationFiles, func(i, j int) bool {
		return migrationFiles[i].Timestamp < migrationFiles[j].Timestamp
	})

	return migrationFiles, nil
}
