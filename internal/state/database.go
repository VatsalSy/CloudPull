/**
 * Database Connection Management for CloudPull
 *
 * Features:
 * - SQLite connection management with connection pooling
 * - Context support for cancellation
 * - Thread-safe concurrent access
 * - Schema initialization and migration
 * - Transaction support
 *
 * Author: CloudPull Team
 * Update History:
 * - 2025-01-29: Initial implementation with connection pooling
 */

package state

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schemaFS embed.FS

// DB represents the database connection manager.
type DB struct {
	*sqlx.DB
	path        string
	maxConns    int
	maxIdleTime time.Duration
	mu          sync.RWMutex
}

// DBConfig holds database configuration.
type DBConfig struct {
	Path         string
	MaxOpenConns int
	MaxIdleConns int
	MaxIdleTime  time.Duration
}

// DefaultConfig returns default database configuration.
func DefaultConfig() DBConfig {
	return DBConfig{
		Path:         "cloudpull.db",
		MaxOpenConns: 25,
		MaxIdleConns: 5,
		MaxIdleTime:  5 * time.Minute,
	}
}

// NewDB creates a new database connection.
func NewDB(cfg DBConfig) (*DB, error) {
	// Open database connection
	db, err := sqlx.Open("sqlite3", fmt.Sprintf("%s?_foreign_keys=on&_journal_mode=WAL", cfg.Path))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxIdleTime(cfg.MaxIdleTime)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	wrapper := &DB{
		DB:          db,
		path:        cfg.Path,
		maxConns:    cfg.MaxOpenConns,
		maxIdleTime: cfg.MaxIdleTime,
	}

	// Initialize schema
	if err := wrapper.InitSchema(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return wrapper, nil
}

// InitSchema initializes the database schema.
func (db *DB) InitSchema(ctx context.Context) error {
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("failed to read schema: %w", err)
	}

	// Execute schema in a transaction
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, string(schema)); err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}

	return tx.Commit()
}

// Close closes the database connection.
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	return db.DB.Close()
}

// WithTx executes a function within a transaction.
func (db *DB) WithTx(ctx context.Context, fn func(*sqlx.Tx) error) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("transaction failed: %w, rollback failed: %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// WithReadTx executes a function within a read-only transaction.
func (db *DB) WithReadTx(ctx context.Context, fn func(*sqlx.Tx) error) error {
	tx, err := db.BeginTxx(ctx, &sql.TxOptions{
		ReadOnly: true,
	})
	if err != nil {
		return fmt.Errorf("failed to begin read transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

// Vacuum performs database maintenance.
func (db *DB) Vacuum(ctx context.Context) error {
	_, err := db.ExecContext(ctx, "VACUUM")
	return err
}

// Stats returns database statistics.
func (db *DB) Stats() sql.DBStats {
	return db.DB.Stats()
}

// HealthCheck performs a database health check.
func (db *DB) HealthCheck(ctx context.Context) error {
	// Check connection
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	// Check basic query
	var result int
	if err := db.GetContext(ctx, &result, "SELECT 1"); err != nil {
		return fmt.Errorf("test query failed: %w", err)
	}

	return nil
}

// Exec is a wrapper around sqlx.Exec that uses context.
func (db *DB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return db.ExecContext(ctx, query, args...)
}

// Get is a wrapper around sqlx.Get that uses context.
func (db *DB) Get(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return db.GetContext(ctx, dest, query, args...)
}

// Select is a wrapper around sqlx.Select that uses context.
func (db *DB) Select(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return db.SelectContext(ctx, dest, query, args...)
}
