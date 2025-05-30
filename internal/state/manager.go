/**
 * State Manager for CloudPull
 * 
 * Features:
 * - Unified interface for all state operations
 * - Transaction management
 * - Error logging integration
 * - Thread-safe operations
 * 
 * Author: CloudPull Team
 * Update History:
 * - 2025-01-29: Initial implementation with comprehensive state management
 */

package state

import (
  "context"
  "database/sql"
  "fmt"
  "runtime"
  "sync"
  "time"

  "github.com/jmoiron/sqlx"
)

// Manager provides a unified interface for state management
type Manager struct {
  db         *DB
  sessions   *SessionStore
  folders    *FolderStore
  files      *FileStore
  queries    *QueryBuilder
  mu         sync.RWMutex
}

// NewManager creates a new state manager
func NewManager(cfg DBConfig) (*Manager, error) {
  db, err := NewDB(cfg)
  if err != nil {
    return nil, fmt.Errorf("failed to create database: %w", err)
  }

  return &Manager{
    db:       db,
    sessions: NewSessionStore(db),
    folders:  NewFolderStore(db),
    files:    NewFileStore(db),
    queries:  NewQueryBuilder(db),
  }, nil
}

// Close closes the state manager
func (m *Manager) Close() error {
  return m.db.Close()
}

// DB returns the underlying database connection
func (m *Manager) DB() *DB {
  return m.db
}

// Sessions returns the session store
func (m *Manager) Sessions() *SessionStore {
  return m.sessions
}

// Folders returns the folder store
func (m *Manager) Folders() *FolderStore {
  return m.folders
}

// Files returns the file store
func (m *Manager) Files() *FileStore {
  return m.files
}

// Queries returns the query builder
func (m *Manager) Queries() *QueryBuilder {
  return m.queries
}


// LogError logs an error to the error_log table
func (m *Manager) LogError(ctx context.Context, sessionID, itemID, itemType, errorType string, err error) error {
  var errorCode, errorMessage, stackTrace sql.NullString
  
  if err != nil {
    errorMessage = sql.NullString{String: err.Error(), Valid: true}
    
    // Get stack trace for debugging
    buf := make([]byte, 4096)
    n := runtime.Stack(buf, false)
    stackTrace = sql.NullString{String: string(buf[:n]), Valid: true}
  }

  query := `
    INSERT INTO error_log (
      session_id, item_id, item_type, error_type,
      error_code, error_message, stack_trace, is_retryable
    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

  _, dbErr := m.db.ExecContext(ctx, query,
    sessionID, itemID, itemType, errorType,
    errorCode, errorMessage, stackTrace, true,
  )
  
  if dbErr != nil {
    return fmt.Errorf("failed to log error: %w", dbErr)
  }

  return nil
}

// UpdateSessionProgress atomically updates session progress
func (m *Manager) UpdateSessionProgress(ctx context.Context, sessionID string, fileCompleted bool, bytesCompleted int64, failed bool) error {
  delta := SessionProgressDelta{
    CompletedBytes: bytesCompleted,
  }

  if fileCompleted {
    if failed {
      delta.FailedFiles = 1
    } else {
      delta.CompletedFiles = 1
    }
  }

  return m.sessions.UpdateProgress(ctx, sessionID, delta)
}

// MarkFileComplete marks a file as complete and updates session progress
func (m *Manager) MarkFileComplete(ctx context.Context, fileID, sessionID string) error {
  return m.db.WithTx(ctx, func(tx *sqlx.Tx) error {
    // Get file info
    var size int64
    err := tx.GetContext(ctx, &size, "SELECT size FROM files WHERE id = $1", fileID)
    if err != nil {
      return fmt.Errorf("failed to get file size: %w", err)
    }

    // Mark file as complete
    fileStore := m.files.WithTx(tx)
    err = fileStore.MarkAsCompleted(ctx, fileID, time.Now())
    if err != nil {
      return err
    }

    // Update session progress
    sessionStore := m.sessions.WithTx(tx)
    delta := SessionProgressDelta{
      CompletedFiles: 1,
      CompletedBytes: size,
    }
    return sessionStore.UpdateProgress(ctx, sessionID, delta)
  })
}

// MarkFileFailed marks a file as failed and logs the error
func (m *Manager) MarkFileFailed(ctx context.Context, fileID, sessionID string, err error) error {
  return m.db.WithTx(ctx, func(tx *sqlx.Tx) error {
    // Mark file as failed
    fileStore := m.files.WithTx(tx)
    fileErr := fileStore.MarkAsFailed(ctx, fileID, err.Error())
    if fileErr != nil {
      return fileErr
    }

    // Update session progress
    sessionStore := m.sessions.WithTx(tx)
    delta := SessionProgressDelta{
      FailedFiles: 1,
    }
    sessionErr := sessionStore.UpdateProgress(ctx, sessionID, delta)
    if sessionErr != nil {
      return sessionErr
    }

    // Log error
    return m.LogError(ctx, sessionID, fileID, "file", "download_failed", err)
  })
}

// GetNextPendingFile retrieves the next file to download
func (m *Manager) GetNextPendingFile(ctx context.Context, sessionID string) (*File, error) {
  // First check for partially downloaded files
  query := `
    SELECT * FROM files 
    WHERE session_id = $1 
      AND status = $2 
      AND bytes_downloaded > 0
    ORDER BY bytes_downloaded DESC
    LIMIT 1`

  var file File
  err := m.db.GetContext(ctx, &file, query, sessionID, FileStatusDownloading)
  if err == nil {
    return &file, nil
  } else if err != sql.ErrNoRows {
    return nil, fmt.Errorf("failed to get partial download: %w", err)
  }

  // Then get next pending file (smallest first)
  query = `
    SELECT * FROM files 
    WHERE session_id = $1 
      AND status = $2
    ORDER BY size ASC
    LIMIT 1`

  err = m.db.GetContext(ctx, &file, query, sessionID, FileStatusPending)
  if err != nil {
    if err == sql.ErrNoRows {
      return nil, nil // No more files
    }
    return nil, fmt.Errorf("failed to get pending file: %w", err)
  }

  return &file, nil
}

// GetNextPendingFolder retrieves the next folder to scan
func (m *Manager) GetNextPendingFolder(ctx context.Context, sessionID string) (*Folder, error) {
  query := `
    SELECT * FROM folders 
    WHERE session_id = $1 
      AND status = $2
    ORDER BY path
    LIMIT 1`

  var folder Folder
  err := m.db.GetContext(ctx, &folder, query, sessionID, FolderStatusPending)
  if err != nil {
    if err == sql.ErrNoRows {
      return nil, nil // No more folders
    }
    return nil, fmt.Errorf("failed to get pending folder: %w", err)
  }

  return &folder, nil
}

// ResumeSession prepares a session for resumption
func (m *Manager) ResumeSession(ctx context.Context, sessionID string) error {
  return m.db.WithTx(ctx, func(tx *sqlx.Tx) error {
    // Update session status
    sessionStore := m.sessions.WithTx(tx)
    err := sessionStore.Resume(ctx, sessionID)
    if err != nil {
      return err
    }

    // Reset failed files with remaining attempts
    fileStore := m.files.WithTx(tx)
    _, err = fileStore.ResetFailedFiles(ctx, sessionID, 3)
    if err != nil {
      return fmt.Errorf("failed to reset failed files: %w", err)
    }

    // Reset failed folders
    query := `
      UPDATE folders 
      SET status = $1, error_message = NULL 
      WHERE session_id = $2 AND status = $3`

    _, err = tx.ExecContext(ctx, query, FolderStatusPending, sessionID, FolderStatusFailed)
    if err != nil {
      return fmt.Errorf("failed to reset failed folders: %w", err)
    }

    return nil
  })
}

// GetSessionStats retrieves comprehensive statistics for a session
func (m *Manager) GetSessionStats(ctx context.Context, sessionID string) (*SessionStats, error) {
  stats := &SessionStats{SessionID: sessionID}

  // Get session progress
  progress, err := m.queries.GetSessionProgress(ctx, sessionID)
  if err != nil {
    return nil, err
  }
  stats.Progress = progress

  // Get file stats
  fileStats, err := m.files.GetStats(ctx, sessionID)
  if err != nil {
    return nil, err
  }
  stats.Files = fileStats

  // Get folder counts
  folderCounts, err := m.folders.CountByStatus(ctx, sessionID)
  if err != nil {
    return nil, err
  }
  stats.FolderCounts = folderCounts

  // Get error summary
  errors, err := m.queries.GetErrorSummary(ctx, sessionID)
  if err != nil {
    return nil, err
  }
  stats.Errors = errors

  return stats, nil
}

// SessionStats represents comprehensive session statistics
type SessionStats struct {
  SessionID    string                `json:"session_id"`
  Progress     *SessionProgress      `json:"progress"`
  Files        *FileStats            `json:"files"`
  FolderCounts map[string]int64      `json:"folder_counts"`
  Errors       []*ErrorSummary       `json:"errors"`
}

// HealthCheck performs a comprehensive health check
func (m *Manager) HealthCheck(ctx context.Context) error {
  // Check database connection
  if err := m.db.HealthCheck(ctx); err != nil {
    return fmt.Errorf("database health check failed: %w", err)
  }

  // Check table accessibility
  var count int
  err := m.db.GetContext(ctx, &count, "SELECT COUNT(*) FROM sessions")
  if err != nil {
    return fmt.Errorf("failed to query sessions table: %w", err)
  }

  return nil
}

// Vacuum performs database maintenance
func (m *Manager) Vacuum(ctx context.Context) error {
  m.mu.Lock()
  defer m.mu.Unlock()

  return m.db.Vacuum(ctx)
}

// GetConfig retrieves a configuration value
func (m *Manager) GetConfig(ctx context.Context, key string) (string, error) {
  var value string
  query := `SELECT value FROM config WHERE key = $1`
  
  err := m.db.GetContext(ctx, &value, query, key)
  if err != nil {
    if err == sql.ErrNoRows {
      return "", fmt.Errorf("config key not found: %s", key)
    }
    return "", fmt.Errorf("failed to get config: %w", err)
  }

  return value, nil
}

// SetConfig sets a configuration value
func (m *Manager) SetConfig(ctx context.Context, key, value string) error {
  query := `
    INSERT INTO config (key, value) VALUES ($1, $2)
    ON CONFLICT(key) DO UPDATE SET value = $2`

  _, err := m.db.ExecContext(ctx, query, key, value)
  if err != nil {
    return fmt.Errorf("failed to set config: %w", err)
  }

  return nil
}

// CreateSession creates a new session
func (m *Manager) CreateSession(ctx context.Context, rootFolderID, rootFolderName, destinationPath string) (*Session, error) {
  session := &Session{
    RootFolderID:    rootFolderID,
    RootFolderName:  sql.NullString{String: rootFolderName, Valid: rootFolderName != ""},
    DestinationPath: destinationPath,
    Status:          SessionStatusActive,
    StartTime:       time.Now(),
  }
  
  err := m.sessions.Create(ctx, session)
  if err != nil {
    return nil, err
  }
  
  return session, nil
}

// GetSession retrieves a session by ID
func (m *Manager) GetSession(ctx context.Context, sessionID string) (*Session, error) {
  return m.sessions.Get(ctx, sessionID)
}

// UpdateSession updates a session
func (m *Manager) UpdateSession(ctx context.Context, session *Session) error {
  return m.sessions.Update(ctx, session)
}

// UpdateSessionStatus updates the session status
func (m *Manager) UpdateSessionStatus(ctx context.Context, sessionID string, status string) error {
  return m.sessions.UpdateStatus(ctx, sessionID, status)
}

// UpdateSessionTotals updates the session total counts
func (m *Manager) UpdateSessionTotals(ctx context.Context, sessionID string, totalFiles, totalBytes int64) error {
  query := `
    UPDATE sessions 
    SET total_files = $2, total_bytes = $3, updated_at = $4
    WHERE id = $1`
  
  _, err := m.db.ExecContext(ctx, query, sessionID, totalFiles, totalBytes, time.Now())
  if err != nil {
    return fmt.Errorf("failed to update session totals: %w", err)
  }
  
  return nil
}

// CreateFolder creates a new folder
func (m *Manager) CreateFolder(ctx context.Context, folder *Folder) error {
  return m.folders.Create(ctx, folder)
}

// UpdateFolder updates a folder
func (m *Manager) UpdateFolder(ctx context.Context, folder *Folder) error {
  return m.folders.Update(ctx, folder)
}

// CreateFiles creates multiple files in a batch
func (m *Manager) CreateFiles(ctx context.Context, files []*File) error {
  return m.files.CreateBatch(ctx, files)
}

// UpdateFileStatus updates the status of a file
func (m *Manager) UpdateFileStatus(ctx context.Context, file *File) error {
  return m.files.UpdateStatus(ctx, file.ID, file.Status)
}

// GetPendingFiles retrieves pending files for a session
func (m *Manager) GetPendingFiles(ctx context.Context, sessionID string, limit int) ([]*File, error) {
  query := `
    SELECT * FROM files 
    WHERE session_id = $1 
      AND status IN ($2, $3)
    ORDER BY 
      CASE WHEN status = $3 THEN 0 ELSE 1 END,
      size ASC
    LIMIT $4`
  
  var files []*File
  err := m.db.SelectContext(ctx, &files, query, 
    sessionID, FileStatusPending, FileStatusDownloading, limit)
  if err != nil {
    return nil, fmt.Errorf("failed to get pending files: %w", err)
  }
  
  return files, nil
}