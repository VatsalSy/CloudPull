/**
 * Folder CRUD Operations for CloudPull
 *
 * Features:
 * - Create, Read, Update, Delete operations for folders
 * - Hierarchical folder structure support
 * - Batch operations for efficient scanning
 * - Status tracking for scan progress
 *
 * Author: CloudPull Team
 * Update History:
 * - 2025-01-29: Initial implementation with full CRUD support
 */

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

// FolderStore handles folder-related database operations.
type FolderStore struct {
	db DBInterface
}

// NewFolderStore creates a new folder store.
func NewFolderStore(db *DB) *FolderStore {
	return &FolderStore{db: db}
}

// Create creates a new folder.
func (s *FolderStore) Create(ctx context.Context, folder *Folder) error {
	query := `
    INSERT INTO folders (
      drive_id, parent_id, session_id, name, path, status, error_message
    ) VALUES (
      :drive_id, :parent_id, :session_id, :name, :path, :status, :error_message
    ) RETURNING id, created_at, updated_at`

	stmt, err := s.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	err = stmt.QueryRowContext(ctx, folder).Scan(
		&folder.ID,
		&folder.CreatedAt,
		&folder.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create folder: %w", err)
	}

	return nil
}

// CreateBatch creates multiple folders in a single transaction.
func (s *FolderStore) CreateBatch(ctx context.Context, folders []*Folder) error {
	if len(folders) == 0 {
		return nil
	}

	return s.db.WithTx(ctx, func(tx *sqlx.Tx) error {
		query := `
      INSERT INTO folders (
        drive_id, parent_id, session_id, name, path, status
      ) VALUES (
        :drive_id, :parent_id, :session_id, :name, :path, :status
      ) RETURNING id, created_at, updated_at`

		stmt, err := tx.PrepareNamedContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to prepare statement: %w", err)
		}
		defer stmt.Close()

		for _, folder := range folders {
			err = stmt.QueryRowContext(ctx, folder).Scan(
				&folder.ID,
				&folder.CreatedAt,
				&folder.UpdatedAt,
			)
			if err != nil {
				return fmt.Errorf("failed to create folder %s: %w", folder.Name, err)
			}
		}

		return nil
	})
}

// Get retrieves a folder by ID.
func (s *FolderStore) Get(ctx context.Context, id string) (*Folder, error) {
	var folder Folder
	query := `SELECT * FROM folders WHERE id = $1`

	err := s.db.GetContext(ctx, &folder, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("folder not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get folder: %w", err)
	}

	return &folder, nil
}

// GetByDriveID retrieves a folder by drive ID and session ID.
func (s *FolderStore) GetByDriveID(ctx context.Context, driveID, sessionID string) (*Folder, error) {
	var folder Folder
	query := `SELECT * FROM folders WHERE drive_id = $1 AND session_id = $2`

	err := s.db.GetContext(ctx, &folder, query, driveID, sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Not found is not an error for this method
		}
		return nil, fmt.Errorf("failed to get folder by drive ID: %w", err)
	}

	return &folder, nil
}

// GetChildren retrieves child folders of a parent.
func (s *FolderStore) GetChildren(ctx context.Context, parentID, sessionID string) ([]*Folder, error) {
	var folders []*Folder
	query := `
    SELECT * FROM folders 
    WHERE parent_id = $1 AND session_id = $2 
    ORDER BY name`

	err := s.db.SelectContext(ctx, &folders, query, parentID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get child folders: %w", err)
	}

	return folders, nil
}

// GetBySession retrieves all folders for a session.
func (s *FolderStore) GetBySession(ctx context.Context, sessionID string) ([]*Folder, error) {
	var folders []*Folder
	query := `SELECT * FROM folders WHERE session_id = $1 ORDER BY path`

	err := s.db.SelectContext(ctx, &folders, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get folders by session: %w", err)
	}

	return folders, nil
}

// GetByStatus retrieves folders by status for a session.
func (s *FolderStore) GetByStatus(ctx context.Context, sessionID, status string) ([]*Folder, error) {
	var folders []*Folder
	query := `
    SELECT * FROM folders 
    WHERE session_id = $1 AND status = $2 
    ORDER BY path`

	err := s.db.SelectContext(ctx, &folders, query, sessionID, status)
	if err != nil {
		return nil, fmt.Errorf("failed to get folders by status: %w", err)
	}

	return folders, nil
}

// GetPendingFolders retrieves folders that need to be scanned.
func (s *FolderStore) GetPendingFolders(ctx context.Context, sessionID string, limit int) ([]*Folder, error) {
	var folders []*Folder
	query := `
    SELECT * FROM folders 
    WHERE session_id = $1 AND status = $2 
    ORDER BY path 
    LIMIT $3`

	err := s.db.SelectContext(ctx, &folders, query, sessionID, FolderStatusPending, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending folders: %w", err)
	}

	return folders, nil
}

// Update updates a folder.
func (s *FolderStore) Update(ctx context.Context, folder *Folder) error {
	query := `
    UPDATE folders SET
      name = :name,
      path = :path,
      status = :status,
      error_message = :error_message
    WHERE id = :id`

	result, err := s.db.NamedExecContext(ctx, query, folder)
	if err != nil {
		return fmt.Errorf("failed to update folder: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("folder not found: %s", folder.ID)
	}

	return nil
}

// UpdateStatus updates the folder status.
func (s *FolderStore) UpdateStatus(ctx context.Context, id, status string) error {
	query := `UPDATE folders SET status = $1 WHERE id = $2`

	result, err := s.db.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update folder status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("folder not found: %s", id)
	}

	return nil
}

// UpdateStatusBatch updates multiple folder statuses.
func (s *FolderStore) UpdateStatusBatch(ctx context.Context, ids []string, status string) error {
	if len(ids) == 0 {
		return nil
	}

	// Use a transaction to ensure all updates succeed or none do
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare the update statement once
	stmt, err := tx.PrepareContext(ctx, "UPDATE folders SET status = $1 WHERE id = $2")
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Execute update for each ID
	updatedCount := 0
	for _, id := range ids {
		result, err := stmt.ExecContext(ctx, status, id)
		if err != nil {
			return fmt.Errorf("failed to update folder %s: %w", id, err)
		}

		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected for folder %s: %w", id, err)
		}

		if rows == 0 {
			return fmt.Errorf("folder not found: %s", id)
		}

		updatedCount++
	}

	// Verify all folders were updated
	if updatedCount != len(ids) {
		return fmt.Errorf("expected to update %d folders, but updated %d", len(ids), updatedCount)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// MarkAsScanning marks a folder as being scanned.
func (s *FolderStore) MarkAsScanning(ctx context.Context, id string) error {
	return s.UpdateStatus(ctx, id, FolderStatusScanning)
}

// MarkAsScanned marks a folder as scanned.
func (s *FolderStore) MarkAsScanned(ctx context.Context, id string) error {
	return s.UpdateStatus(ctx, id, FolderStatusScanned)
}

// MarkAsFailed marks a folder as failed with error message.
func (s *FolderStore) MarkAsFailed(ctx context.Context, id string, errorMsg string) error {
	query := `
    UPDATE folders 
    SET status = $1, error_message = $2 
    WHERE id = $3`

	result, err := s.db.ExecContext(ctx, query, FolderStatusFailed, errorMsg, id)
	if err != nil {
		return fmt.Errorf("failed to mark folder as failed: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("folder not found: %s", id)
	}

	return nil
}

// Delete deletes a folder and all its children.
func (s *FolderStore) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM folders WHERE id = $1`

	result, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete folder: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("folder not found: %s", id)
	}

	return nil
}

// GetPath retrieves the full path of folders from root to the given folder.
func (s *FolderStore) GetPath(ctx context.Context, folderID string) ([]*Folder, error) {
	// Recursive CTE to get the path from root to folder
	query := `
    WITH RECURSIVE folder_path AS (
      SELECT * FROM folders WHERE id = $1
      UNION ALL
      SELECT f.* FROM folders f
      INNER JOIN folder_path fp ON f.id = fp.parent_id
    )
    SELECT * FROM folder_path ORDER BY path`

	var folders []*Folder
	err := s.db.SelectContext(ctx, &folders, query, folderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get folder path: %w", err)
	}

	return folders, nil
}

// CountByStatus counts folders by status for a session.
func (s *FolderStore) CountByStatus(ctx context.Context, sessionID string) (map[string]int64, error) {
	query := `
    SELECT status, COUNT(*) as count 
    FROM folders 
    WHERE session_id = $1 
    GROUP BY status`

	rows, err := s.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to count folders by status: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		counts[status] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return counts, nil
}

// WithTx returns a FolderStore that uses the given transaction.
func (s *FolderStore) WithTx(tx *sqlx.Tx) *FolderStore {
	return &FolderStore{
		db: WrapTx(tx),
	}
}
