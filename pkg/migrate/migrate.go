// Package migrate provides a universal database migration solution for sandbox0 services.
//
// It uses goose as the underlying migration engine and supports:
//   - Running migrations programmatically (embedded in Go code)
//   - PostgreSQL with pgx v5
//   - Idempotent migrations via version tracking
//   - SQL and Go-based migrations
//
// Usage:
//
//	import "github.com/sandbox0-ai/infra/pkg/migrate"
//
//	// Simple auto-migration on startup
//	if err := migrate.Up(ctx, db, "migrations"); err != nil {
//	    log.Fatal(err)
//	}
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

const dialect = "pgx"

// Options configures the migrator behavior.
type Options struct {
	// Logger is an optional logger for migration output.
	Logger Logger
	// TableName is the name of the migration tracking table.
	// Defaults to "goose_db_version".
	TableName string
	// Schema is the database schema to use for migrations.
	// If specified, sets search_path to this schema before running migrations.
	Schema string
}

// Logger defines the interface for logging migration progress.
type Logger interface {
	Printf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
}

// defaultLogger is a silent logger that discards output.
type defaultLogger struct{}

func (defaultLogger) Printf(string, ...interface{}) {}
func (defaultLogger) Fatalf(string, ...interface{}) {}

// Up runs all pending migrations in the specified directory.
//
// The migrations directory can be:
//   - An absolute path
//   - A relative path from the working directory
//   - A path relative to the service's binary location
//
// This function is idempotent - it tracks which migrations have been applied
// and only runs new ones.
func Up(ctx context.Context, pool *pgxpool.Pool, migrationsDir string, opts ...Options) error {
	var opt Options
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.Logger == nil {
		opt.Logger = defaultLogger{}
	}

	// Resolve the migrations directory
	resolvedDir, err := resolveDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("resolve migrations directory: %w", err)
	}

	// Convert pgx pool to sql.DB
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	// Set goose logger
	if opt.Logger != nil {
		goose.SetLogger(opt.Logger)
	}

	// Set custom table name if provided
	if opt.TableName != "" {
		goose.SetTableName(opt.TableName)
	}

	// Set schema search_path if specified
	if opt.Schema != "" {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("SET search_path TO %s", opt.Schema)); err != nil {
			return fmt.Errorf("set search_path: %w", err)
		}
	}

	// Run migrations
	if err := goose.UpContext(ctx, db, resolvedDir); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}

// Status prints the current migration status to stdout.
func Status(ctx context.Context, pool *pgxpool.Pool, migrationsDir string, opts ...Options) error {
	var opt Options
	if len(opts) > 0 {
		opt = opts[0]
	}

	resolvedDir, err := resolveDir(migrationsDir)
	if err != nil {
		return err
	}

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	// Set schema search_path if specified
	if opt.Schema != "" {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("SET search_path TO %s", opt.Schema)); err != nil {
			return fmt.Errorf("set search_path: %w", err)
		}
	}

	if opt.TableName != "" {
		goose.SetTableName(opt.TableName)
	}

	return goose.StatusContext(ctx, db, resolvedDir)
}

// Down rolls back the most recently applied migration.
func Down(ctx context.Context, pool *pgxpool.Pool, migrationsDir string, opts ...Options) error {
	var opt Options
	if len(opts) > 0 {
		opt = opts[0]
	}

	resolvedDir, err := resolveDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("resolve migrations directory: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	if opt.Logger != nil {
		goose.SetLogger(opt.Logger)
	}

	// Set schema search_path if specified
	if opt.Schema != "" {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("SET search_path TO %s", opt.Schema)); err != nil {
			return fmt.Errorf("set search_path: %w", err)
		}
	}

	if opt.TableName != "" {
		goose.SetTableName(opt.TableName)
	}

	if err := goose.DownContext(ctx, db, resolvedDir); err != nil {
		return fmt.Errorf("run down migration: %w", err)
	}

	return nil
}

// Create creates a new migration file in the specified directory.
//
// name: The name of the migration (e.g., "add_users_table")
// migrationType: The type of migration ("sql" or "go")
func Create(migrationsDir, name, migrationType string) error {
	resolvedDir, err := resolveDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("resolve migrations directory: %w", err)
	}

	db, err := sql.Open("pgx", "postgres:///")
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	return goose.Create(db, resolvedDir, name, migrationType)
}

// resolveDir resolves the migrations directory to an absolute path.
func resolveDir(migrationsDir string) (string, error) {
	dir := migrationsDir

	// If absolute, return as-is
	if filepath.IsAbs(dir) {
		if _, err := os.Stat(dir); err != nil {
			return "", fmt.Errorf("migrations directory not found: %s", dir)
		}
		return dir, nil
	}

	// Try relative to current working directory
	if _, err := os.Stat(dir); err == nil {
		return filepath.Abs(dir)
	}

	// Try relative to the binary location
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		resolved := filepath.Join(exeDir, dir)
		if _, err := os.Stat(resolved); err == nil {
			return resolved, nil
		}
	}

	return "", fmt.Errorf("migrations directory not found: %s", dir)
}
