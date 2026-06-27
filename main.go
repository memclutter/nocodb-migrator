package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"github.com/memclutter/nocodb-migrator/cmd"
)

var (
	version = "0.1.0"
)

func main() {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		// Ignore error if file not found
		log.Println("No .env file found, using environment variables")
	}

	rootCmd := &cobra.Command{
		Use:     "nocodb-migrate",
		Short:   "NocoDB migration tool",
		Long:    "A tool for managing database migrations in NocoDB using Meta API v3",
		Version: version,
	}

	// Add commands
	rootCmd.AddCommand(cmd.NewCreateCommand())
	rootCmd.AddCommand(cmd.NewUpCommand())
	rootCmd.AddCommand(cmd.NewDownCommand())
	rootCmd.AddCommand(cmd.NewInfoCommand())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
