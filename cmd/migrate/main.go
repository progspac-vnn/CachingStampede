// Command migrate applies or rolls back SQL schema migrations found in the
// migrations/ directory, tracking applied versions in a schema_migrations
// table.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/progspac-vnn/CachingStampede/internal/config"
	"github.com/progspac-vnn/CachingStampede/internal/db"
	"github.com/progspac-vnn/CachingStampede/internal/logger"
)

const migrationsDir = "migrations"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 || (os.Args[1] != "up" && os.Args[1] != "down") {
		return fmt.Errorf("usage: migrate <up|down>")
	}
	command := os.Args[1]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	log, err := logger.New(cfg.Env)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	defer func() { _ = log.Sync() }()

	ctx := context.Background()

	postgres, err := db.Connect(ctx, cfg.Postgres, log)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	defer postgres.Close()

	switch command {
	case "up":
		return runUp(ctx, postgres.Pool, migrationsDir, log)
	case "down":
		return runDown(ctx, postgres.Pool, migrationsDir, log)
	}

	return nil
}
