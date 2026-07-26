package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

const (
	upSuffix   = ".up.sql"
	downSuffix = ".down.sql"
)

// migrationFile pairs a migration version with the path to its SQL file.
type migrationFile struct {
	version string
	path    string
}

// ensureMigrationsTable creates the bookkeeping table used to track which
// migrations have already been applied, if it doesn't already exist.
func ensureMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
	const query = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`
	if _, err := pool.Exec(ctx, query); err != nil {
		return fmt.Errorf("migrate: failed to create schema_migrations table: %w", err)
	}
	return nil
}

// appliedVersions returns the set of migration versions already recorded in
// schema_migrations.
func appliedVersions(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("migrate: failed to query applied versions: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("migrate: failed to scan applied version: %w", err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrate: error iterating applied versions: %w", err)
	}

	return applied, nil
}

// lastAppliedVersion returns the most recently applied migration version, or
// an empty string if none have been applied.
func lastAppliedVersion(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var version string
	err := pool.QueryRow(ctx, "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("migrate: failed to query last applied version: %w", err)
	}
	return version, nil
}

// listMigrationFiles returns migration files in dir matching suffix, sorted
// ascending by version.
func listMigrationFiles(dir, suffix string) ([]migrationFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("migrate: failed to read migrations directory: %w", err)
	}

	var files []migrationFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), suffix) {
			continue
		}
		version := strings.TrimSuffix(entry.Name(), suffix)
		files = append(files, migrationFile{
			version: version,
			path:    filepath.Join(dir, entry.Name()),
		})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].version < files[j].version })
	return files, nil
}

// runUp applies every migration in dir that has not yet been recorded in
// schema_migrations, in ascending version order.
func runUp(ctx context.Context, pool *pgxpool.Pool, dir string, log *zap.Logger) error {
	if err := ensureMigrationsTable(ctx, pool); err != nil {
		return err
	}

	applied, err := appliedVersions(ctx, pool)
	if err != nil {
		return err
	}

	files, err := listMigrationFiles(dir, upSuffix)
	if err != nil {
		return err
	}

	for _, f := range files {
		if applied[f.version] {
			continue
		}
		if err := applyMigration(ctx, pool, f, log); err != nil {
			return err
		}
	}

	return nil
}

// applyMigration executes a single up migration and records it as applied,
// inside one transaction.
func applyMigration(ctx context.Context, pool *pgxpool.Pool, f migrationFile, log *zap.Logger) error {
	sqlBytes, err := os.ReadFile(f.path)
	if err != nil {
		return fmt.Errorf("migrate: failed to read %s: %w", f.path, err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("migrate: failed to begin transaction for %s: %w", f.version, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
		return fmt.Errorf("migrate: failed to apply %s: %w", f.version, err)
	}

	if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", f.version); err != nil {
		return fmt.Errorf("migrate: failed to record %s: %w", f.version, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migrate: failed to commit %s: %w", f.version, err)
	}

	log.Info("migration applied", zap.String("version", f.version))
	return nil
}

// runDown rolls back the single most recently applied migration.
func runDown(ctx context.Context, pool *pgxpool.Pool, dir string, log *zap.Logger) error {
	if err := ensureMigrationsTable(ctx, pool); err != nil {
		return err
	}

	version, err := lastAppliedVersion(ctx, pool)
	if err != nil {
		return err
	}
	if version == "" {
		log.Info("no migrations to roll back")
		return nil
	}

	downPath := filepath.Join(dir, version+downSuffix)
	sqlBytes, err := os.ReadFile(downPath)
	if err != nil {
		return fmt.Errorf("migrate: failed to read %s: %w", downPath, err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("migrate: failed to begin transaction for %s: %w", version, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
		return fmt.Errorf("migrate: failed to roll back %s: %w", version, err)
	}

	if _, err := tx.Exec(ctx, "DELETE FROM schema_migrations WHERE version = $1", version); err != nil {
		return fmt.Errorf("migrate: failed to unrecord %s: %w", version, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migrate: failed to commit rollback of %s: %w", version, err)
	}

	log.Info("migration rolled back", zap.String("version", version))
	return nil
}
