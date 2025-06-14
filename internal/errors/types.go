/**
 * Error Types for CloudPull
 *
 * Defines structured error types with metadata for proper error handling,
 * categorization, and recovery strategies.
 *
 * Author: CloudPull Team
 * Created: 2025-01-29
 */

package errors

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrorType represents the category of error.
type ErrorType int

const (
	// ErrorTypeUnknown represents an unknown error.
	ErrorTypeUnknown ErrorType = iota

	// ErrorTypeNetwork represents network-related errors (transient).
	ErrorTypeNetwork

	// ErrorTypeAPIQuota represents API rate limit or quota errors.
	ErrorTypeAPIQuota

	// ErrorTypePermission represents permission/authorization errors (permanent).
	ErrorTypePermission

	// ErrorTypeStorage represents storage-related errors (may be transient).
	ErrorTypeStorage

	// ErrorTypeCorruption represents data corruption errors.
	ErrorTypeCorruption

	// ErrorTypeConfiguration represents configuration errors.
	ErrorTypeConfiguration

	// ErrorTypeContext represents context cancellation or timeout.
	ErrorTypeContext

	// ErrorTypeAPI represents general API errors.
	ErrorTypeAPI
)

// String returns the string representation of ErrorType.
func (et ErrorType) String() string {
	switch et {
	case ErrorTypeNetwork:
		return "Network"
	case ErrorTypeAPIQuota:
		return "APIQuota"
	case ErrorTypePermission:
		return "Permission"
	case ErrorTypeStorage:
		return "Storage"
	case ErrorTypeCorruption:
		return "Corruption"
	case ErrorTypeConfiguration:
		return "Configuration"
	case ErrorTypeContext:
		return "Context"
	case ErrorTypeAPI:
		return "API"
	default:
		return "Unknown"
	}
}

// IsRetryable returns whether the error type is retryable.
func (et ErrorType) IsRetryable() bool {
	switch et {
	case ErrorTypeNetwork, ErrorTypeAPIQuota, ErrorTypeStorage:
		return true
	case ErrorTypePermission, ErrorTypeConfiguration:
		return false
	case ErrorTypeCorruption:
		// Corruption errors are retryable but require special handling.
		// When a file corruption is detected (e.g., checksum mismatch,
		// incomplete download), the retry strategy should:
		// 1. Delete the corrupted local file
		// 2. Reset the file's download progress in the database
		// 3. Restart the download from the beginning
		// This ensures we don't append to or resume from corrupted data.
		// The retry should use exponential backoff to handle temporary
		// issues that might have caused the corruption.
		return true
	default:
		return false
	}
}

// Error represents a structured error with metadata.
type Error struct {
	Timestamp time.Time
	Err       error
	Retry     *RetryInfo
	Context   map[string]interface{}
	Op        string
	Path      string
	Type      ErrorType
	Code      int
}

// RetryInfo contains information about retry attempts.
type RetryInfo struct {
	NextRetry       time.Time
	Attempt         int
	MaxAttempts     int
	BackoffDuration time.Duration
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s: %s [%s] %v", e.Type, e.Op, e.Path, e.Err)
	}
	return fmt.Sprintf("%s: %s %v", e.Type, e.Op, e.Err)
}

// Unwrap returns the underlying error.
func (e *Error) Unwrap() error {
	return e.Err
}

// IsRetryable returns whether the error is retryable.
func (e *Error) IsRetryable() bool {
	return e.Type.IsRetryable()
}

// ShouldRetry checks if the error should be retried based on attempts.
func (e *Error) ShouldRetry() bool {
	if !e.IsRetryable() || e.Retry == nil {
		return false
	}
	return e.Retry.Attempt < e.Retry.MaxAttempts
}

// New creates a new Error.
func New(errorType ErrorType, op, path string, err error) *Error {
	return &Error{
		Type:      errorType,
		Op:        op,
		Path:      path,
		Err:       err,
		Timestamp: time.Now(),
		Context:   make(map[string]interface{}),
	}
}

// WithCode adds an error code.
func (e *Error) WithCode(code int) *Error {
	e.Code = code
	return e
}

// WithContext adds context information.
func (e *Error) WithContext(key string, value interface{}) *Error {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// WithRetry adds retry information.
func (e *Error) WithRetry(attempt, maxAttempts int, backoff time.Duration) *Error {
	e.Retry = &RetryInfo{
		Attempt:         attempt,
		MaxAttempts:     maxAttempts,
		NextRetry:       time.Now().Add(backoff),
		BackoffDuration: backoff,
	}
	return e
}

// ErrorBatch represents a collection of errors from batch operations.
type ErrorBatch struct {
	Op     string
	Errors []*Error
}

// Error implements the error interface for ErrorBatch.
func (eb *ErrorBatch) Error() string {
	if len(eb.Errors) == 0 {
		return fmt.Sprintf("%s: no errors", eb.Op)
	}
	return fmt.Sprintf("%s: %d errors occurred", eb.Op, len(eb.Errors))
}

// Add adds an error to the batch.
func (eb *ErrorBatch) Add(err *Error) {
	eb.Errors = append(eb.Errors, err)
}

// HasErrors returns whether the batch contains any errors.
func (eb *ErrorBatch) HasErrors() bool {
	return len(eb.Errors) > 0
}

// RetryableErrors returns only retryable errors.
func (eb *ErrorBatch) RetryableErrors() []*Error {
	var retryable []*Error
	for _, err := range eb.Errors {
		if err.IsRetryable() {
			retryable = append(retryable, err)
		}
	}
	return retryable
}

// ErrorsByType groups errors by their type.
func (eb *ErrorBatch) ErrorsByType() map[ErrorType][]*Error {
	grouped := make(map[ErrorType][]*Error)
	for _, err := range eb.Errors {
		grouped[err.Type] = append(grouped[err.Type], err)
	}
	return grouped
}

// IsContextError checks if the error is due to context cancellation.
func IsContextError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// GetErrorType attempts to determine the error type from a generic error.
func GetErrorType(err error) ErrorType {
	if err == nil {
		return ErrorTypeUnknown
	}

	// Check for context errors
	if IsContextError(err) {
		return ErrorTypeContext
	}

	// Convert to string for pattern matching
	errStr := err.Error()

	// Network errors (connection, timeout, DNS)
	networkPatterns := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"network is unreachable",
		"timeout",
		"i/o timeout",
		"TLS handshake timeout",
		"dial tcp",
		"lookup",
		"temporary failure in name resolution",
		"no route to host",
		"broken pipe",
	}
	for _, pattern := range networkPatterns {
		if containsIgnoreCase(errStr, pattern) {
			return ErrorTypeNetwork
		}
	}

	// Google Drive API quota errors (HTTP 429 and quota messages)
	quotaPatterns := []string{
		"429",
		"rate limit",
		"quota exceeded",
		"userRateLimitExceeded",
		"rateLimitExceeded",
		"dailyLimitExceeded",
		"quotaExceeded",
		"Too Many Requests",
	}
	for _, pattern := range quotaPatterns {
		if containsIgnoreCase(errStr, pattern) {
			return ErrorTypeAPIQuota
		}
	}

	// Storage errors (disk full, write errors)
	storagePatterns := []string{
		"disk full",
		"no space left on device",
		"insufficient storage",
		"cannot allocate memory",
		"file too large",
		"exceeds filesystem size",
	}
	for _, pattern := range storagePatterns {
		if containsIgnoreCase(errStr, pattern) {
			return ErrorTypeStorage
		}
	}

	// OAuth token expiration and authentication errors
	authPatterns := []string{
		"invalid_grant",
		"invalid_token",
		"token expired",
		"401 Unauthorized",
		"403 Forbidden",
		"access_denied",
		"permission denied",
		"insufficient_scope",
		"authError",
		"unauthenticated",
	}
	for _, pattern := range authPatterns {
		if containsIgnoreCase(errStr, pattern) {
			return ErrorTypePermission
		}
	}

	// Configuration errors
	configPatterns := []string{
		"invalid configuration",
		"missing configuration",
		"invalid_client",
		"malformed",
	}
	for _, pattern := range configPatterns {
		if containsIgnoreCase(errStr, pattern) {
			return ErrorTypeConfiguration
		}
	}

	// API errors (general API failures)
	apiPatterns := []string{
		"500 Internal Server Error",
		"502 Bad Gateway",
		"503 Service Unavailable",
		"504 Gateway Timeout",
		"API error",
		"backend error",
	}
	for _, pattern := range apiPatterns {
		if containsIgnoreCase(errStr, pattern) {
			return ErrorTypeAPI
		}
	}

	return ErrorTypeUnknown
}

// containsIgnoreCase checks if s contains substr ignoring case.
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
