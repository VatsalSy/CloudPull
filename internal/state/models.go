/**
 * Data Models for CloudPull State Management
 * 
 * Features:
 * - Struct definitions matching database schema
 * - JSON and database field mappings
 * - Validation methods
 * - Helper methods for common operations
 * 
 * Author: CloudPull Team
 * Update History:
 * - 2025-01-29: Initial implementation with all core models
 */

package state

import (
  "database/sql"
  "time"
)

// Session statuses
const (
  SessionStatusActive    = "active"
  SessionStatusPaused    = "paused"
  SessionStatusCompleted = "completed"
  SessionStatusFailed    = "failed"
  SessionStatusCancelled = "cancelled"
)

// Folder statuses
const (
  FolderStatusPending  = "pending"
  FolderStatusScanning = "scanning"
  FolderStatusScanned  = "scanned"
  FolderStatusFailed   = "failed"
)

// File statuses
const (
  FileStatusPending     = "pending"
  FileStatusDownloading = "downloading"
  FileStatusCompleted   = "completed"
  FileStatusFailed      = "failed"
  FileStatusSkipped     = "skipped"
)

// Chunk statuses
const (
  ChunkStatusPending     = "pending"
  ChunkStatusDownloading = "downloading"
  ChunkStatusCompleted   = "completed"
  ChunkStatusFailed      = "failed"
)

// Session represents a sync session
type Session struct {
  ID               string         `db:"id" json:"id"`
  RootFolderID     string         `db:"root_folder_id" json:"root_folder_id"`
  RootFolderName   sql.NullString `db:"root_folder_name" json:"root_folder_name"`
  DestinationPath  string         `db:"destination_path" json:"destination_path"`
  StartTime        time.Time      `db:"start_time" json:"start_time"`
  EndTime          sql.NullTime   `db:"end_time" json:"end_time"`
  Status           string         `db:"status" json:"status"`
  TotalFiles       int64          `db:"total_files" json:"total_files"`
  CompletedFiles   int64          `db:"completed_files" json:"completed_files"`
  FailedFiles      int64          `db:"failed_files" json:"failed_files"`
  SkippedFiles     int64          `db:"skipped_files" json:"skipped_files"`
  TotalBytes       int64          `db:"total_bytes" json:"total_bytes"`
  CompletedBytes   int64          `db:"completed_bytes" json:"completed_bytes"`
  CreatedAt        time.Time      `db:"created_at" json:"created_at"`
  UpdatedAt        time.Time      `db:"updated_at" json:"updated_at"`
}

// IsActive returns true if the session is active
func (s *Session) IsActive() bool {
  return s.Status == SessionStatusActive
}

// Progress returns the completion percentage
func (s *Session) Progress() float64 {
  if s.TotalFiles == 0 {
    return 0
  }
  return float64(s.CompletedFiles) / float64(s.TotalFiles) * 100
}

// BytesProgress returns the bytes completion percentage
func (s *Session) BytesProgress() float64 {
  if s.TotalBytes == 0 {
    return 0
  }
  return float64(s.CompletedBytes) / float64(s.TotalBytes) * 100
}

// Duration returns the session duration
func (s *Session) Duration() time.Duration {
  if s.EndTime.Valid {
    return s.EndTime.Time.Sub(s.StartTime)
  }
  return time.Since(s.StartTime)
}

// Folder represents a Google Drive folder
type Folder struct {
  ID           string         `db:"id" json:"id"`
  DriveID      string         `db:"drive_id" json:"drive_id"`
  ParentID     sql.NullString `db:"parent_id" json:"parent_id"`
  SessionID    string         `db:"session_id" json:"session_id"`
  Name         string         `db:"name" json:"name"`
  Path         string         `db:"path" json:"path"`
  Status       string         `db:"status" json:"status"`
  ErrorMessage sql.NullString `db:"error_message" json:"error_message,omitempty"`
  CreatedAt    time.Time      `db:"created_at" json:"created_at"`
  UpdatedAt    time.Time      `db:"updated_at" json:"updated_at"`
}

// HasError returns true if the folder has an error
func (f *Folder) HasError() bool {
  return f.ErrorMessage.Valid && f.ErrorMessage.String != ""
}

// File represents a Google Drive file
type File struct {
  ID                string         `db:"id" json:"id"`
  DriveID           string         `db:"drive_id" json:"drive_id"`
  FolderID          string         `db:"folder_id" json:"folder_id"`
  SessionID         string         `db:"session_id" json:"session_id"`
  Name              string         `db:"name" json:"name"`
  Path              string         `db:"path" json:"path"`
  Size              int64          `db:"size" json:"size"`
  MD5Checksum       sql.NullString `db:"md5_checksum" json:"md5_checksum,omitempty"`
  MimeType          sql.NullString `db:"mime_type" json:"mime_type,omitempty"`
  IsGoogleDoc       bool           `db:"is_google_doc" json:"is_google_doc"`
  ExportMimeType    sql.NullString `db:"export_mime_type" json:"export_mime_type,omitempty"`
  Status            string         `db:"status" json:"status"`
  BytesDownloaded   int64          `db:"bytes_downloaded" json:"bytes_downloaded"`
  DownloadAttempts  int            `db:"download_attempts" json:"download_attempts"`
  ErrorMessage      sql.NullString `db:"error_message" json:"error_message,omitempty"`
  DriveModifiedTime sql.NullTime   `db:"drive_modified_time" json:"drive_modified_time,omitempty"`
  LocalModifiedTime sql.NullTime   `db:"local_modified_time" json:"local_modified_time,omitempty"`
  CreatedAt         time.Time      `db:"created_at" json:"created_at"`
  UpdatedAt         time.Time      `db:"updated_at" json:"updated_at"`
}

// Progress returns the download progress percentage
func (f *File) Progress() float64 {
  if f.Size == 0 {
    return 100
  }
  return float64(f.BytesDownloaded) / float64(f.Size) * 100
}

// IsComplete returns true if the file download is complete
func (f *File) IsComplete() bool {
  return f.Status == FileStatusCompleted
}

// NeedsRetry returns true if the file should be retried
func (f *File) NeedsRetry() bool {
  return f.Status == FileStatusFailed && f.DownloadAttempts < 3
}

// DownloadChunk represents a file download chunk
type DownloadChunk struct {
  ID          int64        `db:"id" json:"id"`
  FileID      string       `db:"file_id" json:"file_id"`
  ChunkIndex  int          `db:"chunk_index" json:"chunk_index"`
  StartByte   int64        `db:"start_byte" json:"start_byte"`
  EndByte     int64        `db:"end_byte" json:"end_byte"`
  Status      string       `db:"status" json:"status"`
  Attempts    int          `db:"attempts" json:"attempts"`
  CreatedAt   time.Time    `db:"created_at" json:"created_at"`
  CompletedAt sql.NullTime `db:"completed_at" json:"completed_at,omitempty"`
}

// Size returns the chunk size in bytes
func (c *DownloadChunk) Size() int64 {
  return c.EndByte - c.StartByte + 1
}

// IsComplete returns true if the chunk is downloaded
func (c *DownloadChunk) IsComplete() bool {
  return c.Status == ChunkStatusCompleted
}

// ErrorLog represents an error log entry
type ErrorLog struct {
  ID           int64          `db:"id" json:"id"`
  SessionID    string         `db:"session_id" json:"session_id"`
  ItemID       string         `db:"item_id" json:"item_id"`
  ItemType     string         `db:"item_type" json:"item_type"`
  ErrorType    string         `db:"error_type" json:"error_type"`
  ErrorCode    sql.NullString `db:"error_code" json:"error_code,omitempty"`
  ErrorMessage sql.NullString `db:"error_message" json:"error_message,omitempty"`
  StackTrace   sql.NullString `db:"stack_trace" json:"stack_trace,omitempty"`
  RetryCount   int            `db:"retry_count" json:"retry_count"`
  IsRetryable  bool           `db:"is_retryable" json:"is_retryable"`
  CreatedAt    time.Time      `db:"created_at" json:"created_at"`
}

// Config represents a configuration entry
type Config struct {
  Key       string    `db:"key" json:"key"`
  Value     string    `db:"value" json:"value"`
  CreatedAt time.Time `db:"created_at" json:"created_at"`
  UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// SessionSummary represents the session_summary view
type SessionSummary struct {
  ID                  string         `db:"id" json:"id"`
  RootFolderName      sql.NullString `db:"root_folder_name" json:"root_folder_name"`
  DestinationPath     string         `db:"destination_path" json:"destination_path"`
  Status              string         `db:"status" json:"status"`
  TotalFiles          int64          `db:"total_files" json:"total_files"`
  CompletedFiles      int64          `db:"completed_files" json:"completed_files"`
  FailedFiles         int64          `db:"failed_files" json:"failed_files"`
  SkippedFiles        int64          `db:"skipped_files" json:"skipped_files"`
  ProgressPercent     sql.NullFloat64 `db:"progress_percent" json:"progress_percent"`
  TotalBytes          int64          `db:"total_bytes" json:"total_bytes"`
  CompletedBytes      int64          `db:"completed_bytes" json:"completed_bytes"`
  BytesProgressPercent sql.NullFloat64 `db:"bytes_progress_percent" json:"bytes_progress_percent"`
  StartTime           time.Time      `db:"start_time" json:"start_time"`
  EndTime             sql.NullTime   `db:"end_time" json:"end_time"`
  DurationSeconds     float64        `db:"duration_seconds" json:"duration_seconds"`
}

// PendingDownload represents the pending_downloads view
type PendingDownload struct {
  ID               string         `db:"id" json:"id"`
  DriveID          string         `db:"drive_id" json:"drive_id"`
  Name             string         `db:"name" json:"name"`
  Path             string         `db:"path" json:"path"`
  Size             int64          `db:"size" json:"size"`
  MimeType         sql.NullString `db:"mime_type" json:"mime_type"`
  IsGoogleDoc      bool           `db:"is_google_doc" json:"is_google_doc"`
  ExportMimeType   sql.NullString `db:"export_mime_type" json:"export_mime_type"`
  BytesDownloaded  int64          `db:"bytes_downloaded" json:"bytes_downloaded"`
  DownloadAttempts int            `db:"download_attempts" json:"download_attempts"`
  FolderPath       string         `db:"folder_path" json:"folder_path"`
}

// FileStats represents file statistics for a session
type FileStats struct {
  TotalCount      int64 `db:"total_count" json:"total_count"`
  TotalBytes      int64 `db:"total_bytes" json:"total_bytes"`
  CompletedCount  int64 `db:"completed_count" json:"completed_count"`
  CompletedBytes  int64 `db:"completed_bytes" json:"completed_bytes"`
  FailedCount     int64 `db:"failed_count" json:"failed_count"`
  SkippedCount    int64 `db:"skipped_count" json:"skipped_count"`
  PendingCount    int64 `db:"pending_count" json:"pending_count"`
  DownloadingCount int64 `db:"downloading_count" json:"downloading_count"`
}