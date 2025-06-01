/**
 * Session CRUD Operations for CloudPull
 *
 * Features:
 * - Create, Read, Update, Delete operations for sessions
 * - Session status management
 * - Progress tracking
 * - Resume functionality support
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

// SessionStore handles session-related database operations.
type SessionStore struct {
	db DBInterface
}

// NewSessionStore creates a new session store.
func NewSessionStore(db *DB) *SessionStore {
	return &SessionStore{db: db}
}

// Create creates a new session.
func (s *SessionStore) Create(ctx context.Context, session *Session) error {
	query := `
    INSERT INTO sessions (
      root_folder_id, root_folder_name, destination_path,
      status, total_files, completed_files, failed_files,
      skipped_files, total_bytes, completed_bytes
    ) VALUES (
      :root_folder_id, :root_folder_name, :destination_path,
      :status, :total_files, :completed_files, :failed_files,
      :skipped_files, :total_bytes, :completed_bytes
    ) RETURNING id, created_at, updated_at, start_time`

	stmt, err := s.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	err = stmt.QueryRowContext(ctx, session).Scan(
		&session.ID,
		&session.CreatedAt,
		&session.UpdatedAt,
		&session.StartTime,
	)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	return nil
}

// Get retrieves a session by ID.
func (s *SessionStore) Get(ctx context.Context, id string) (*Session, error) {
	var session Session
	query := `SELECT * FROM sessions WHERE id = $1`

	err := s.db.GetContext(ctx, &session, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return &session, nil
}

// GetActive retrieves all active sessions.
func (s *SessionStore) GetActive(ctx context.Context) ([]*Session, error) {
	var sessions []*Session
	query := `SELECT * FROM sessions WHERE status = $1 ORDER BY start_time DESC`

	err := s.db.SelectContext(ctx, &sessions, query, SessionStatusActive)
	if err != nil {
		return nil, fmt.Errorf("failed to get active sessions: %w", err)
	}

	return sessions, nil
}

// GetByStatus retrieves sessions by status.
func (s *SessionStore) GetByStatus(ctx context.Context, status string) ([]*Session, error) {
	var sessions []*Session
	query := `SELECT * FROM sessions WHERE status = $1 ORDER BY start_time DESC`

	err := s.db.SelectContext(ctx, &sessions, query, status)
	if err != nil {
		return nil, fmt.Errorf("failed to get sessions by status: %w", err)
	}

	return sessions, nil
}

// List retrieves sessions with pagination.
func (s *SessionStore) List(ctx context.Context, limit, offset int) ([]*Session, error) {
	var sessions []*Session
	query := `SELECT * FROM sessions ORDER BY start_time DESC LIMIT $1 OFFSET $2`

	err := s.db.SelectContext(ctx, &sessions, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	return sessions, nil
}

// Update updates a session.
func (s *SessionStore) Update(ctx context.Context, session *Session) error {
	query := `
    UPDATE sessions SET
      root_folder_name = :root_folder_name,
      destination_path = :destination_path,
      end_time = :end_time,
      status = :status,
      total_files = :total_files,
      completed_files = :completed_files,
      failed_files = :failed_files,
      skipped_files = :skipped_files,
      total_bytes = :total_bytes,
      completed_bytes = :completed_bytes
    WHERE id = :id`

	result, err := s.db.NamedExecContext(ctx, query, session)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("session not found: %s", session.ID)
	}

	return nil
}

// UpdateStatus updates the session status.
func (s *SessionStore) UpdateStatus(ctx context.Context, id, status string) error {
	query := `UPDATE sessions SET status = $1, updated_at = $2 WHERE id = $3 AND updated_at = (
		SELECT updated_at FROM sessions WHERE id = $3
	)`

	result, err := s.db.ExecContext(ctx, query, status, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("session not found or concurrent update detected: %s", id)
	}

	return nil
}

// UpdateProgress updates session progress counters.
func (s *SessionStore) UpdateProgress(ctx context.Context, id string, delta SessionProgressDelta) error {
	query := `
    UPDATE sessions SET
      total_files = total_files + $1,
      completed_files = completed_files + $2,
      failed_files = failed_files + $3,
      skipped_files = skipped_files + $4,
      total_bytes = total_bytes + $5,
      completed_bytes = completed_bytes + $6,
      updated_at = $7
    WHERE id = $8`

	result, err := s.db.ExecContext(ctx, query,
		delta.TotalFiles,
		delta.CompletedFiles,
		delta.FailedFiles,
		delta.SkippedFiles,
		delta.TotalBytes,
		delta.CompletedBytes,
		time.Now().UTC(),
		id,
	)
	if err != nil {
		return fmt.Errorf("failed to update session progress: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("session not found: %s", id)
	}

	return nil
}

// Complete marks a session as completed.
func (s *SessionStore) Complete(ctx context.Context, id string) error {
	query := `
    UPDATE sessions
    SET status = $1, end_time = $2
    WHERE id = $3 AND status = $4`

	result, err := s.db.ExecContext(ctx, query,
		SessionStatusCompleted,
		time.Now().UTC(),
		id,
		SessionStatusActive,
	)
	if err != nil {
		return fmt.Errorf("failed to complete session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("session not found or not active: %s", id)
	}

	return nil
}

// Pause marks a session as paused.
func (s *SessionStore) Pause(ctx context.Context, id string) error {
	return s.UpdateStatus(ctx, id, SessionStatusPaused)
}

// Resume marks a session as active.
func (s *SessionStore) Resume(ctx context.Context, id string) error {
	return s.UpdateStatus(ctx, id, SessionStatusActive)
}

// Cancel marks a session as canceled.
func (s *SessionStore) Cancel(ctx context.Context, id string) error {
	query := `
    UPDATE sessions
    SET status = $1, end_time = $2
    WHERE id = $3 AND status IN ($4, $5)`

	result, err := s.db.ExecContext(ctx, query,
		SessionStatusCancelled,
		time.Now().UTC(),
		id,
		SessionStatusActive,
		SessionStatusPaused,
	)
	if err != nil {
		return fmt.Errorf("failed to cancel session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("session not found or already completed: %s", id)
	}

	return nil
}

// Delete deletes a session and all related data.
func (s *SessionStore) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM sessions WHERE id = $1`

	result, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("session not found: %s", id)
	}

	return nil
}

// GetSummary retrieves a session summary.
func (s *SessionStore) GetSummary(ctx context.Context, id string) (*SessionSummary, error) {
	var summary SessionSummary
	query := `SELECT * FROM session_summary WHERE id = $1`

	err := s.db.GetContext(ctx, &summary, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get session summary: %w", err)
	}

	return &summary, nil
}

// ListSummaries retrieves session summaries with pagination.
func (s *SessionStore) ListSummaries(ctx context.Context, limit, offset int) ([]*SessionSummary, error) {
	var summaries []*SessionSummary
	query := `SELECT * FROM session_summary ORDER BY start_time DESC LIMIT $1 OFFSET $2`

	err := s.db.SelectContext(ctx, &summaries, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list session summaries: %w", err)
	}

	return summaries, nil
}

// GetResumableSessions retrieves sessions that can be resumed.
func (s *SessionStore) GetResumableSessions(ctx context.Context) ([]*Session, error) {
	var sessions []*Session
	query := `
    SELECT * FROM sessions
    WHERE status IN ($1, $2)
    ORDER BY start_time DESC`

	err := s.db.SelectContext(ctx, &sessions, query, SessionStatusPaused, SessionStatusFailed)
	if err != nil {
		return nil, fmt.Errorf("failed to get resumable sessions: %w", err)
	}

	return sessions, nil
}

// SessionProgressDelta represents changes to session progress counters.
type SessionProgressDelta struct {
	TotalFiles     int64
	CompletedFiles int64
	FailedFiles    int64
	SkippedFiles   int64
	TotalBytes     int64
	CompletedBytes int64
}

// WithTx returns a SessionStore that uses the given transaction.
func (s *SessionStore) WithTx(tx *sqlx.Tx) *SessionStore {
	return &SessionStore{
		db: WrapTx(tx),
	}
}
