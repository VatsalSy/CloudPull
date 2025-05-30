/**
 * Complex Query Builders for CloudPull
 *
 * Features:
 * - Advanced query builders for reporting
 * - Cross-table analytics queries
 * - Performance optimized queries
 * - Resume functionality queries
 *
 * Author: CloudPull Team
 * Update History:
 * - 2025-01-29: Initial implementation with complex queries
 */

package state

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// QueryBuilder provides complex query functionality.
type QueryBuilder struct {
	db *DB
}

// NewQueryBuilder creates a new query builder.
func NewQueryBuilder(db *DB) *QueryBuilder {
	return &QueryBuilder{db: db}
}

// SessionProgress represents detailed session progress.
type SessionProgress struct {
	StartTime           time.Time `db:"start_time" json:"start_time"`
	SessionID           string    `db:"session_id" json:"session_id"`
	RootFolderName      string    `db:"root_folder_name" json:"root_folder_name"`
	Status              string    `db:"status" json:"status"`
	ScannedFolders      int64     `db:"scanned_folders" json:"scanned_folders"`
	TotalFolders        int64     `db:"total_folders" json:"total_folders"`
	ElapsedSeconds      float64   `db:"elapsed_seconds" json:"elapsed_seconds"`
	TotalFiles          int64     `db:"total_files" json:"total_files"`
	CompletedFiles      int64     `db:"completed_files" json:"completed_files"`
	FailedFiles         int64     `db:"failed_files" json:"failed_files"`
	TotalBytes          int64     `db:"total_bytes" json:"total_bytes"`
	CompletedBytes      int64     `db:"completed_bytes" json:"completed_bytes"`
	TransferRate        float64   `db:"transfer_rate" json:"transfer_rate"`
	EstimatedCompletion float64   `db:"estimated_completion" json:"estimated_completion"`
}

// GetSessionProgress retrieves detailed progress for a session.
func (q *QueryBuilder) GetSessionProgress(ctx context.Context, sessionID string) (*SessionProgress, error) {
	query := `
    WITH folder_stats AS (
      SELECT 
        COUNT(*) as total_folders,
        SUM(CASE WHEN status = 'scanned' THEN 1 ELSE 0 END) as scanned_folders
      FROM folders
      WHERE session_id = $1
    ),
    current_time AS (
      SELECT (julianday('now') - julianday(s.start_time)) * 86400 as elapsed_seconds
      FROM sessions s
      WHERE s.id = $1
    )
    SELECT 
      s.id as session_id,
      s.root_folder_name,
      s.status,
      s.start_time,
      ct.elapsed_seconds,
      COALESCE(fs.total_folders, 0) as total_folders,
      COALESCE(fs.scanned_folders, 0) as scanned_folders,
      s.total_files,
      s.completed_files,
      s.failed_files,
      s.total_bytes,
      s.completed_bytes,
      CASE 
        WHEN ct.elapsed_seconds > 0 THEN s.completed_bytes / ct.elapsed_seconds
        ELSE 0
      END as transfer_rate,
      CASE 
        WHEN s.completed_bytes > 0 AND s.completed_bytes < s.total_bytes THEN
          (s.total_bytes - s.completed_bytes) / (s.completed_bytes / ct.elapsed_seconds)
        ELSE 0
      END as estimated_completion
    FROM sessions s
    CROSS JOIN folder_stats fs
    CROSS JOIN current_time ct
    WHERE s.id = $1`

	var progress SessionProgress
	err := q.db.GetContext(ctx, &progress, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session progress: %w", err)
	}

	return &progress, nil
}

// FolderTree represents a folder with its children count.
type FolderTree struct {
	ID           string `db:"id" json:"id"`
	DriveID      string `db:"drive_id" json:"drive_id"`
	ParentID     string `db:"parent_id" json:"parent_id,omitempty"`
	Name         string `db:"name" json:"name"`
	Path         string `db:"path" json:"path"`
	Status       string `db:"status" json:"status"`
	ChildCount   int64  `db:"child_count" json:"child_count"`
	FileCount    int64  `db:"file_count" json:"file_count"`
	TotalSize    int64  `db:"total_size" json:"total_size"`
	DownloadSize int64  `db:"downloaded_size" json:"downloaded_size"`
}

// GetFolderTree retrieves the folder tree structure with statistics.
func (q *QueryBuilder) GetFolderTree(ctx context.Context, sessionID string, parentID *string) ([]*FolderTree, error) {
	query := `
    SELECT 
      f.id,
      f.drive_id,
      f.parent_id,
      f.name,
      f.path,
      f.status,
      (SELECT COUNT(*) FROM folders WHERE parent_id = f.id) as child_count,
      (SELECT COUNT(*) FROM files WHERE folder_id = f.id) as file_count,
      COALESCE((SELECT SUM(size) FROM files WHERE folder_id = f.id), 0) as total_size,
      COALESCE((SELECT SUM(bytes_downloaded) FROM files WHERE folder_id = f.id), 0) as downloaded_size
    FROM folders f
    WHERE f.session_id = $1`

	args := []interface{}{sessionID}

	if parentID == nil {
		query += " AND f.parent_id IS NULL"
	} else {
		query += " AND f.parent_id = $2"
		args = append(args, *parentID)
	}

	query += " ORDER BY f.name"

	var folders []*FolderTree
	err := q.db.SelectContext(ctx, &folders, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get folder tree: %w", err)
	}

	return folders, nil
}

// ErrorSummary represents error statistics.
type ErrorSummary struct {
	LastOccurred time.Time `db:"last_occurred" json:"last_occurred"`
	ErrorType    string    `db:"error_type" json:"error_type"`
	ErrorCode    string    `db:"error_code" json:"error_code,omitempty"`
	ItemType     string    `db:"item_type" json:"item_type"`
	Count        int64     `db:"count" json:"count"`
	IsRetryable  bool      `db:"is_retryable" json:"is_retryable"`
}

// GetErrorSummary retrieves error summary for a session.
func (q *QueryBuilder) GetErrorSummary(ctx context.Context, sessionID string) ([]*ErrorSummary, error) {
	query := `
    SELECT 
      error_type,
      error_code,
      item_type,
      COUNT(*) as count,
      MAX(created_at) as last_occurred,
      is_retryable
    FROM error_log
    WHERE session_id = $1
    GROUP BY error_type, error_code, item_type, is_retryable
    ORDER BY count DESC`

	var errors []*ErrorSummary
	err := q.db.SelectContext(ctx, &errors, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get error summary: %w", err)
	}

	return errors, nil
}

// ResumableState represents the state needed to resume a session.
type ResumableState struct {
	Session          *Session       `json:"session"`
	PartialDownloads []*PartialFile `json:"partial_downloads"`
	PendingFolders   int64          `json:"pending_folders"`
	PendingFiles     int64          `json:"pending_files"`
	FailedRetryable  int64          `json:"failed_retryable"`
}

// PartialFile represents a partially downloaded file.
type PartialFile struct {
	FileID          string  `db:"file_id" json:"file_id"`
	Name            string  `db:"name" json:"name"`
	Path            string  `db:"path" json:"path"`
	Size            int64   `db:"size" json:"size"`
	BytesDownloaded int64   `db:"bytes_downloaded" json:"bytes_downloaded"`
	Progress        float64 `db:"progress" json:"progress"`
}

// GetResumableState retrieves the state needed to resume a session.
func (q *QueryBuilder) GetResumableState(ctx context.Context, sessionID string) (*ResumableState, error) {
	// Get session
	sessionStore := NewSessionStore(q.db)
	session, err := sessionStore.Get(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	state := &ResumableState{Session: session}

	// Count pending folders
	err = q.db.GetContext(ctx, &state.PendingFolders,
		"SELECT COUNT(*) FROM folders WHERE session_id = $1 AND status = $2",
		sessionID, FolderStatusPending)
	if err != nil {
		return nil, fmt.Errorf("failed to count pending folders: %w", err)
	}

	// Count pending files
	err = q.db.GetContext(ctx, &state.PendingFiles,
		"SELECT COUNT(*) FROM files WHERE session_id = $1 AND status IN ($2, $3)",
		sessionID, FileStatusPending, FileStatusDownloading)
	if err != nil {
		return nil, fmt.Errorf("failed to count pending files: %w", err)
	}

	// Get partial downloads
	partialQuery := `
    SELECT 
      id as file_id,
      name,
      path,
      size,
      bytes_downloaded,
      ROUND(CAST(bytes_downloaded AS FLOAT) / size * 100, 2) as progress
    FROM files
    WHERE session_id = $1 
      AND status = $2 
      AND bytes_downloaded > 0
    ORDER BY bytes_downloaded DESC`

	err = q.db.SelectContext(ctx, &state.PartialDownloads, partialQuery, sessionID, FileStatusDownloading)
	if err != nil {
		return nil, fmt.Errorf("failed to get partial downloads: %w", err)
	}

	// Count retryable failures
	err = q.db.GetContext(ctx, &state.FailedRetryable,
		"SELECT COUNT(*) FROM files WHERE session_id = $1 AND status = $2 AND download_attempts < 3",
		sessionID, FileStatusFailed)
	if err != nil {
		return nil, fmt.Errorf("failed to count retryable failures: %w", err)
	}

	return state, nil
}

// TransferStats represents transfer statistics over time.
type TransferStats struct {
	Timestamp      time.Time `db:"timestamp" json:"timestamp"`
	BytesPerSecond float64   `db:"bytes_per_second" json:"bytes_per_second"`
	FilesPerMinute float64   `db:"files_per_minute" json:"files_per_minute"`
}

// GetTransferStats retrieves transfer statistics for charting.
func (q *QueryBuilder) GetTransferStats(ctx context.Context, sessionID string, interval time.Duration) ([]*TransferStats, error) {
	// This would require a more complex schema with transfer history
	// For now, return current stats
	var stats []*TransferStats

	progress, err := q.GetSessionProgress(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	stats = append(stats, &TransferStats{
		Timestamp:      time.Now(),
		BytesPerSecond: progress.TransferRate,
		FilesPerMinute: float64(progress.CompletedFiles) / (progress.ElapsedSeconds / 60),
	})

	return stats, nil
}

// DuplicateFile represents a potential duplicate file.
type DuplicateFile struct {
	DriveID1 string `db:"drive_id1" json:"drive_id1"`
	DriveID2 string `db:"drive_id2" json:"drive_id2"`
	Name     string `db:"name" json:"name"`
	Path1    string `db:"path1" json:"path1"`
	Path2    string `db:"path2" json:"path2"`
	Checksum string `db:"checksum" json:"checksum,omitempty"`
	Size     int64  `db:"size" json:"size"`
}

// FindDuplicates finds duplicate files in a session.
func (q *QueryBuilder) FindDuplicates(ctx context.Context, sessionID string) ([]*DuplicateFile, error) {
	query := `
    SELECT 
      f1.drive_id as drive_id1,
      f2.drive_id as drive_id2,
      f1.name,
      f1.size,
      f1.path as path1,
      f2.path as path2,
      f1.md5_checksum as checksum
    FROM files f1
    JOIN files f2 ON 
      f1.session_id = f2.session_id
      AND f1.name = f2.name
      AND f1.size = f2.size
      AND f1.id < f2.id
    WHERE f1.session_id = $1
      AND (f1.md5_checksum IS NULL OR f1.md5_checksum = f2.md5_checksum)
    ORDER BY f1.size DESC`

	var duplicates []*DuplicateFile
	err := q.db.SelectContext(ctx, &duplicates, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to find duplicates: %w", err)
	}

	return duplicates, nil
}

// SearchFiles searches for files by name pattern.
func (q *QueryBuilder) SearchFiles(ctx context.Context, sessionID string, pattern string, limit int) ([]*File, error) {
	// Escape special characters and add wildcards
	pattern = "%" + strings.ReplaceAll(pattern, "%", "\\%") + "%"

	query := `
    SELECT * FROM files 
    WHERE session_id = $1 
      AND name LIKE $2 ESCAPE '\'
    ORDER BY name
    LIMIT $3`

	var files []*File
	err := q.db.SelectContext(ctx, &files, query, sessionID, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search files: %w", err)
	}

	return files, nil
}

// GetLargeFiles retrieves the largest files in a session.
func (q *QueryBuilder) GetLargeFiles(ctx context.Context, sessionID string, limit int) ([]*File, error) {
	query := `
    SELECT * FROM files 
    WHERE session_id = $1 
    ORDER BY size DESC 
    LIMIT $2`

	var files []*File
	err := q.db.SelectContext(ctx, &files, query, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get large files: %w", err)
	}

	return files, nil
}

// GetOldestSessions retrieves sessions older than the specified duration.
func (q *QueryBuilder) GetOldestSessions(ctx context.Context, olderThan time.Duration) ([]*Session, error) {
	cutoff := time.Now().Add(-olderThan)

	query := `
    SELECT * FROM sessions 
    WHERE created_at < $1 
    ORDER BY created_at ASC`

	var sessions []*Session
	err := q.db.SelectContext(ctx, &sessions, query, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to get old sessions: %w", err)
	}

	return sessions, nil
}

// CleanupOldSessions deletes sessions older than the specified duration.
func (q *QueryBuilder) CleanupOldSessions(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)

	query := `
    DELETE FROM sessions 
    WHERE created_at < $1 
      AND status IN ($2, $3, $4)`

	result, err := q.db.ExecContext(ctx, query, cutoff,
		SessionStatusCompleted, SessionStatusFailed, SessionStatusCancelled)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old sessions: %w", err)
	}

	return result.RowsAffected()
}
