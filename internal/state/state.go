/**
 * State Package - Main entry point
 *
 * Features:
 * - Package documentation
 * - Type aliases for common interfaces
 * - Helper functions
 *
 * Author: CloudPull Team
 * Update History:
 * - 2025-01-29: Initial implementation
 */

// Package state provides database state management for CloudPull.
// It includes session tracking, file and folder management, progress monitoring,
// and resume functionality through SQLite database persistence.
package state

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// DefaultDatabasePath is the default location for the SQLite database.
const DefaultDatabasePath = "cloudpull.db"

// DefaultChunkSize is the default size for download chunks (5MB).
const DefaultChunkSize = 5 * 1024 * 1024

// MaxRetryAttempts is the maximum number of retry attempts for failed downloads.
const MaxRetryAttempts = 3

// State provides the main interface for state management.
type State interface {
	// Session management
	CreateSession(ctx context.Context, rootFolderID, rootFolderName, destinationPath string) (*Session, error)
	GetSession(ctx context.Context, id string) (*Session, error)
	ResumeSession(ctx context.Context, id string) error

	// Progress tracking
	GetSessionStats(ctx context.Context, sessionID string) (*SessionStats, error)
	UpdateSessionProgress(ctx context.Context, sessionID string, fileCompleted bool, bytesCompleted int64, failed bool) error

	// File operations
	GetNextPendingFile(ctx context.Context, sessionID string) (*File, error)
	MarkFileComplete(ctx context.Context, fileID, sessionID string) error
	MarkFileFailed(ctx context.Context, fileID, sessionID string, err error) error

	// Folder operations
	GetNextPendingFolder(ctx context.Context, sessionID string) (*Folder, error)

	// Maintenance
	Close() error
	HealthCheck(ctx context.Context) error
	Vacuum(ctx context.Context) error
}

// Ensure Manager implements State interface.
var _ State = (*Manager)(nil)

// New creates a new state manager with default configuration.
func New(databasePath string) (State, error) {
	cfg := DefaultConfig()
	cfg.Path = databasePath
	return NewManager(cfg)
}

// NewWithConfig creates a new state manager with custom configuration.
func NewWithConfig(cfg DBConfig) (State, error) {
	return NewManager(cfg)
}

// IsRetryableError determines if an error should trigger a retry.
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// TODO: Add more sophisticated error classification
	// For now, consider most errors retryable except specific ones
	errStr := err.Error()

	// Non-retryable errors
	nonRetryable := []string{
		"permission denied",
		"no such file",
		"disk full",
		"quota exceeded",
	}

	for _, nr := range nonRetryable {
		if containsIgnoreCase(errStr, nr) {
			return false
		}
	}

	return true
}

// containsIgnoreCase checks if s contains substr ignoring case.
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// FormatBytes formats bytes into human readable format.
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// FormatDuration formats a duration into human readable format.
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		secs := int(d.Seconds())
		if secs == 1 {
			return "1 second"
		}
		return fmt.Sprintf("%d seconds", secs)
	}

	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		minUnit := "minutes"
		if mins == 1 {
			minUnit = "minute"
		}
		secUnit := "seconds"
		if secs == 1 {
			secUnit = "second"
		}
		return fmt.Sprintf("%d %s %d %s", mins, minUnit, secs, secUnit)
	}

	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	hourUnit := "hours"
	if hours == 1 {
		hourUnit = "hour"
	}
	minUnit := "minutes"
	if mins == 1 {
		minUnit = "minute"
	}
	return fmt.Sprintf("%d %s %d %s", hours, hourUnit, mins, minUnit)
}

// CalculateETA calculates estimated time of arrival based on progress.
func CalculateETA(bytesCompleted, totalBytes int64, elapsedTime time.Duration) time.Duration {
	if bytesCompleted == 0 || bytesCompleted >= totalBytes || elapsedTime == 0 {
		return 0
	}

	rate := float64(bytesCompleted) / elapsedTime.Seconds()
	remaining := float64(totalBytes - bytesCompleted)

	return time.Duration(remaining/rate) * time.Second
}
