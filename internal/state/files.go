/**
 * File CRUD Operations for CloudPull
 *
 * Features:
 * - Create, Read, Update, Delete operations for files
 * - Download progress tracking
 * - Resume support with chunk management
 * - Batch operations for efficiency
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
	"time"

	"github.com/jmoiron/sqlx"
)

// FileStore handles file-related database operations.
type FileStore struct {
	db DBInterface
}

// NewFileStore creates a new file store.
func NewFileStore(db *DB) *FileStore {
	return &FileStore{db: db}
}

// Create creates a new file.
func (s *FileStore) Create(ctx context.Context, file *File) error {
	query := `
    INSERT INTO files (
      drive_id, folder_id, session_id, name, path, size,
      md5_checksum, mime_type, is_google_doc, export_mime_type,
      status, bytes_downloaded, download_attempts, error_message,
      drive_modified_time, local_modified_time
    ) VALUES (
      :drive_id, :folder_id, :session_id, :name, :path, :size,
      :md5_checksum, :mime_type, :is_google_doc, :export_mime_type,
      :status, :bytes_downloaded, :download_attempts, :error_message,
      :drive_modified_time, :local_modified_time
    ) RETURNING id, created_at, updated_at`

	stmt, err := s.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	err = stmt.QueryRowContext(ctx, file).Scan(
		&file.ID,
		&file.CreatedAt,
		&file.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}

	return nil
}

// CreateBatch creates multiple files in a single transaction.
func (s *FileStore) CreateBatch(ctx context.Context, files []*File) error {
	if len(files) == 0 {
		return nil
	}

	return s.db.WithTx(ctx, func(tx *sqlx.Tx) error {
		query := `
      INSERT INTO files (
        drive_id, folder_id, session_id, name, path, size,
        md5_checksum, mime_type, is_google_doc, export_mime_type,
        status, drive_modified_time
      ) VALUES (
        :drive_id, :folder_id, :session_id, :name, :path, :size,
        :md5_checksum, :mime_type, :is_google_doc, :export_mime_type,
        :status, :drive_modified_time
      ) RETURNING id, created_at, updated_at`

		stmt, err := tx.PrepareNamedContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to prepare statement: %w", err)
		}
		defer stmt.Close()

		for _, file := range files {
			err = stmt.QueryRowContext(ctx, file).Scan(
				&file.ID,
				&file.CreatedAt,
				&file.UpdatedAt,
			)
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", file.Name, err)
			}
		}

		return nil
	})
}

// Get retrieves a file by ID.
func (s *FileStore) Get(ctx context.Context, id string) (*File, error) {
	var file File
	query := `SELECT * FROM files WHERE id = $1`

	err := s.db.GetContext(ctx, &file, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("file not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get file: %w", err)
	}

	return &file, nil
}

// GetByDriveID retrieves a file by drive ID and session ID.
func (s *FileStore) GetByDriveID(ctx context.Context, driveID, sessionID string) (*File, error) {
	var file File
	query := `SELECT * FROM files WHERE drive_id = $1 AND session_id = $2`

	err := s.db.GetContext(ctx, &file, query, driveID, sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Not found is not an error for this method
		}
		return nil, fmt.Errorf("failed to get file by drive ID: %w", err)
	}

	return &file, nil
}

// GetByFolder retrieves files in a folder.
func (s *FileStore) GetByFolder(ctx context.Context, folderID string) ([]*File, error) {
	var files []*File
	query := `SELECT * FROM files WHERE folder_id = $1 ORDER BY name`

	err := s.db.SelectContext(ctx, &files, query, folderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get files by folder: %w", err)
	}

	return files, nil
}

// GetBySession retrieves all files for a session.
func (s *FileStore) GetBySession(ctx context.Context, sessionID string) ([]*File, error) {
	var files []*File
	query := `SELECT * FROM files WHERE session_id = $1 ORDER BY path, name`

	err := s.db.SelectContext(ctx, &files, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get files by session: %w", err)
	}

	return files, nil
}

// GetByStatus retrieves files by status for a session.
func (s *FileStore) GetByStatus(ctx context.Context, sessionID, status string) ([]*File, error) {
	var files []*File
	query := `
    SELECT * FROM files 
    WHERE session_id = $1 AND status = $2 
    ORDER BY size ASC` // Smaller files first

	err := s.db.SelectContext(ctx, &files, query, sessionID, status)
	if err != nil {
		return nil, fmt.Errorf("failed to get files by status: %w", err)
	}

	return files, nil
}

// GetPendingDownloads retrieves files pending download.
func (s *FileStore) GetPendingDownloads(ctx context.Context, sessionID string, limit int) ([]*PendingDownload, error) {
	var downloads []*PendingDownload
	query := `
    SELECT * FROM pending_downloads 
    WHERE session_id = $1 
    LIMIT $2`

	err := s.db.SelectContext(ctx, &downloads, query, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending downloads: %w", err)
	}

	return downloads, nil
}

// Update updates a file.
func (s *FileStore) Update(ctx context.Context, file *File) error {
	query := `
    UPDATE files SET
      name = :name,
      path = :path,
      size = :size,
      md5_checksum = :md5_checksum,
      mime_type = :mime_type,
      is_google_doc = :is_google_doc,
      export_mime_type = :export_mime_type,
      status = :status,
      bytes_downloaded = :bytes_downloaded,
      download_attempts = :download_attempts,
      error_message = :error_message,
      drive_modified_time = :drive_modified_time,
      local_modified_time = :local_modified_time
    WHERE id = :id`

	result, err := s.db.NamedExecContext(ctx, query, file)
	if err != nil {
		return fmt.Errorf("failed to update file: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("file not found: %s", file.ID)
	}

	return nil
}

// UpdateStatus updates the file status.
func (s *FileStore) UpdateStatus(ctx context.Context, id, status string) error {
	query := `UPDATE files SET status = $1 WHERE id = $2`

	result, err := s.db.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update file status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("file not found: %s", id)
	}

	return nil
}

// UpdateProgress updates file download progress.
func (s *FileStore) UpdateProgress(ctx context.Context, id string, bytesDownloaded int64) error {
	query := `
    UPDATE files 
    SET bytes_downloaded = $1, status = $2 
    WHERE id = $3`

	result, err := s.db.ExecContext(ctx, query, bytesDownloaded, FileStatusDownloading, id)
	if err != nil {
		return fmt.Errorf("failed to update file progress: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("file not found: %s", id)
	}

	return nil
}

// MarkAsDownloading marks a file as downloading.
func (s *FileStore) MarkAsDownloading(ctx context.Context, id string) error {
	query := `
    UPDATE files 
    SET status = $1, download_attempts = download_attempts + 1 
    WHERE id = $2`

	result, err := s.db.ExecContext(ctx, query, FileStatusDownloading, id)
	if err != nil {
		return fmt.Errorf("failed to mark file as downloading: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("file not found: %s", id)
	}

	return nil
}

// MarkAsCompleted marks a file as completed.
func (s *FileStore) MarkAsCompleted(ctx context.Context, id string, localModTime time.Time) error {
	query := `
    UPDATE files 
    SET status = $1, bytes_downloaded = size, local_modified_time = $2 
    WHERE id = $3`

	result, err := s.db.ExecContext(ctx, query, FileStatusCompleted, localModTime, id)
	if err != nil {
		return fmt.Errorf("failed to mark file as completed: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("file not found: %s", id)
	}

	return nil
}

// MarkAsFailed marks a file as failed with error message.
func (s *FileStore) MarkAsFailed(ctx context.Context, id string, errorMsg string) error {
	query := `
    UPDATE files 
    SET status = $1, error_message = $2 
    WHERE id = $3`

	result, err := s.db.ExecContext(ctx, query, FileStatusFailed, errorMsg, id)
	if err != nil {
		return fmt.Errorf("failed to mark file as failed: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("file not found: %s", id)
	}

	return nil
}

// MarkAsSkipped marks a file as skipped.
func (s *FileStore) MarkAsSkipped(ctx context.Context, id string, reason string) error {
	query := `
    UPDATE files 
    SET status = $1, error_message = $2 
    WHERE id = $3`

	result, err := s.db.ExecContext(ctx, query, FileStatusSkipped, reason, id)
	if err != nil {
		return fmt.Errorf("failed to mark file as skipped: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("file not found: %s", id)
	}

	return nil
}

// Delete deletes a file.
func (s *FileStore) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM files WHERE id = $1`

	result, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("file not found: %s", id)
	}

	return nil
}

// GetStats retrieves file statistics for a session.
func (s *FileStore) GetStats(ctx context.Context, sessionID string) (*FileStats, error) {
	query := `
    SELECT 
      COUNT(*) as total_count,
      COALESCE(SUM(size), 0) as total_bytes,
      COALESCE(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END), 0) as completed_count,
      COALESCE(SUM(CASE WHEN status = 'completed' THEN size ELSE 0 END), 0) as completed_bytes,
      COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0) as failed_count,
      COALESCE(SUM(CASE WHEN status = 'skipped' THEN 1 ELSE 0 END), 0) as skipped_count,
      COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0) as pending_count,
      COALESCE(SUM(CASE WHEN status = 'downloading' THEN 1 ELSE 0 END), 0) as downloading_count
    FROM files
    WHERE session_id = $1`

	var stats FileStats
	err := s.db.GetContext(ctx, &stats, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get file stats: %w", err)
	}

	return &stats, nil
}

// CountByStatus counts files by status for a session.
func (s *FileStore) CountByStatus(ctx context.Context, sessionID string) (map[string]int64, error) {
	query := `
    SELECT status, COUNT(*) as count 
    FROM files 
    WHERE session_id = $1 
    GROUP BY status`

	rows, err := s.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to count files by status: %w", err)
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

// GetFailedFiles retrieves failed files that can be retried.
func (s *FileStore) GetFailedFiles(ctx context.Context, sessionID string, maxAttempts int) ([]*File, error) {
	var files []*File
	query := `
    SELECT * FROM files 
    WHERE session_id = $1 
      AND status = $2 
      AND download_attempts < $3 
    ORDER BY download_attempts ASC, size ASC`

	err := s.db.SelectContext(ctx, &files, query, sessionID, FileStatusFailed, maxAttempts)
	if err != nil {
		return nil, fmt.Errorf("failed to get failed files: %w", err)
	}

	return files, nil
}

// ResetFailedFiles resets failed files to pending status.
func (s *FileStore) ResetFailedFiles(ctx context.Context, sessionID string, maxAttempts int) (int64, error) {
	query := `
    UPDATE files 
    SET status = $1, error_message = NULL 
    WHERE session_id = $2 
      AND status = $3 
      AND download_attempts < $4`

	result, err := s.db.ExecContext(ctx, query, FileStatusPending, sessionID, FileStatusFailed, maxAttempts)
	if err != nil {
		return 0, fmt.Errorf("failed to reset failed files: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rows, nil
}

// WithTx returns a FileStore that uses the given transaction.
func (s *FileStore) WithTx(tx *sqlx.Tx) *FileStore {
	return &FileStore{
		db: WrapTx(tx),
	}
}

// CreateChunks creates download chunks for a file.
func (s *FileStore) CreateChunks(ctx context.Context, fileID string, chunkSize int64) error {
	// Get file size
	var size int64
	err := s.db.GetContext(ctx, &size, "SELECT size FROM files WHERE id = $1", fileID)
	if err != nil {
		return fmt.Errorf("failed to get file size: %w", err)
	}

	// Calculate chunks
	var chunks []DownloadChunk
	chunkIndex := 0
	for offset := int64(0); offset < size; offset += chunkSize {
		endByte := offset + chunkSize - 1
		if endByte >= size {
			endByte = size - 1
		}

		chunks = append(chunks, DownloadChunk{
			FileID:     fileID,
			ChunkIndex: chunkIndex,
			StartByte:  offset,
			EndByte:    endByte,
			Status:     ChunkStatusPending,
		})
		chunkIndex++
	}

	// Insert chunks
	return s.db.WithTx(ctx, func(tx *sqlx.Tx) error {
		query := `
      INSERT INTO download_chunks (
        file_id, chunk_index, start_byte, end_byte, status
      ) VALUES (
        :file_id, :chunk_index, :start_byte, :end_byte, :status
      )`

		for _, chunk := range chunks {
			_, err := tx.NamedExecContext(ctx, query, chunk)
			if err != nil {
				return fmt.Errorf("failed to create chunk: %w", err)
			}
		}
		return nil
	})
}

// GetChunks retrieves chunks for a file.
func (s *FileStore) GetChunks(ctx context.Context, fileID string) ([]*DownloadChunk, error) {
	var chunks []*DownloadChunk
	query := `
    SELECT * FROM download_chunks 
    WHERE file_id = $1 
    ORDER BY chunk_index`

	err := s.db.SelectContext(ctx, &chunks, query, fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to get chunks: %w", err)
	}

	return chunks, nil
}

// UpdateChunkStatus updates a chunk status.
func (s *FileStore) UpdateChunkStatus(ctx context.Context, id int64, status string) error {
	query := `UPDATE download_chunks SET status = $1, completed_at = $2 WHERE id = $3`

	var completedAt sql.NullTime
	if status == ChunkStatusCompleted {
		completedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}

	result, err := s.db.ExecContext(ctx, query, status, completedAt, id)
	if err != nil {
		return fmt.Errorf("failed to update chunk status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("chunk not found: %d", id)
	}

	return nil
}
