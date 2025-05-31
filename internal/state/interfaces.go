/*
Database Interfaces for CloudPull State Management

Features:
- Common interface for database operations
- Support for both DB and Transaction contexts
- Type-safe database operations

Author: CloudPull Team
Update History:
- 2025-01-30: Initial implementation
*/

package state

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
)

// on both *DB and *sqlx.Tx.
type DBInterface interface {
	// Core sqlx methods
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	PrepareNamedContext(ctx context.Context, query string) (*sqlx.NamedStmt, error)
	NamedExecContext(ctx context.Context, query string, arg interface{}) (sql.Result, error)

	// Transaction support (only available on *DB)
	WithTx(ctx context.Context, fn func(*sqlx.Tx) error) error
}

// Ensure *DB implements DBInterface.
var _ DBInterface = (*DB)(nil)

// txWrapper wraps a transaction to implement DBInterface.
type txWrapper struct {
	*sqlx.Tx
}

// WithTx on a transaction just executes the function with itself.
func (t *txWrapper) WithTx(_ context.Context, fn func(*sqlx.Tx) error) error {
	return fn(t.Tx)
}

// Ensure txWrapper implements DBInterface.
var _ DBInterface = (*txWrapper)(nil)

// WrapTx wraps a transaction to implement DBInterface.
func WrapTx(tx *sqlx.Tx) DBInterface {
	return &txWrapper{Tx: tx}
}
