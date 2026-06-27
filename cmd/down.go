package cmd

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/memclutter/nocodb-migrator/internal/migration"
	"github.com/memclutter/nocodb-migrator/internal/storage"
)

// NewDownCommand creates the down command
func NewDownCommand() *cobra.Command {
	var count int

	cmd := &cobra.Command{
		Use:   "down [count]",
		Short: "Rollback migrations",
		Long:  "Rollback applied migrations. If count is specified, rollback only that many migrations.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				var err error
				count, err = strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("invalid count: %w", err)
				}
			}
			return runDown(count)
		},
	}

	return cmd
}

func runDown(count int) error {
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
	migrationFiles, err := getMigrationFiles(migrationsDir, "down")
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

	// Filter migrations that are applied
	appliedMigrations := []MigrationFile{}
	for _, mf := range migrationFiles {
		applied, err := storage.IsMigrationApplied(mf.Timestamp, mf.Name)
		if err != nil {
			return fmt.Errorf("failed to check migration status: %w", err)
		}
		if applied && mf.Timestamp <= currentTimestamp {
			appliedMigrations = append(appliedMigrations, mf)
		}
	}

	if len(appliedMigrations) == 0 {
		fmt.Println("No applied migrations to rollback")
		return nil
	}

	// Sort in reverse order (newest to oldest)
	sort.Slice(appliedMigrations, func(i, j int) bool {
		return appliedMigrations[i].Timestamp > appliedMigrations[j].Timestamp
	})

	// Limit count if specified
	if count > 0 && count < len(appliedMigrations) {
		appliedMigrations = appliedMigrations[:count]
	}

	// Create executor
	executor := migration.NewExecutor(client)

	// Rollback migrations
	for _, mf := range appliedMigrations {
		fmt.Printf("Rolling back migration: %s\n", mf.Name)

		// Parse migration
		mig, err := migration.ParseMigration(mf.Path)
		if err != nil {
			return fmt.Errorf("failed to parse migration %s: %w", mf.Name, err)
		}

		// Execute migration
		if err := executor.ExecuteMigration(mig); err != nil {
			return fmt.Errorf("failed to execute migration %s: %w", mf.Name, err)
		}

		// Delete migration record from Migrations table
		if err := storage.DeleteMigrationRecord(mf.Timestamp, mf.Name); err != nil {
			return fmt.Errorf("failed to delete migration record: %w", err)
		}

		fmt.Printf("Migration %s rolled back successfully\n", mf.Name)
	}

	fmt.Printf("Rolled back %d migration(s)\n", len(appliedMigrations))
	return nil
}
