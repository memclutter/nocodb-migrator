package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/memclutter/nocodb-migrator/internal/storage"
)

// NewInfoCommand creates the info command
func NewInfoCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show migration status",
		Long:  "Show current migration version and applied migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInfo()
		},
	}

	return cmd
}

func runInfo() error {
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

	// Get current version
	timestamp, name, err := storage.GetCurrentVersion()
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	if timestamp == 0 {
		fmt.Println("No migrations applied")
		return nil
	}

	fmt.Printf("Current version: %d-%s\n", timestamp, name)

	// Get list of applied migrations
	migrations, err := storage.GetAppliedMigrations()
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	if len(migrations) == 0 {
		fmt.Println("No migration history found")
		return nil
	}

	fmt.Println("\nApplied migrations:")
	for _, m := range migrations {
		fmt.Printf("  %d-%s [%s] - %s\n", m.Timestamp, m.Name, m.Direction, m.Status)
	}

	return nil
}
