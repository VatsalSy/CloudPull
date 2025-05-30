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
  "fmt"
  "time"
)

// ErrorType represents the category of error
type ErrorType int

const (
  // ErrorTypeUnknown represents an unknown error
  ErrorTypeUnknown ErrorType = iota
  
  // ErrorTypeNetwork represents network-related errors (transient)
  ErrorTypeNetwork
  
  // ErrorTypeAPIQuota represents API rate limit or quota errors
  ErrorTypeAPIQuota
  
  // ErrorTypePermission represents permission/authorization errors (permanent)
  ErrorTypePermission
  
  // ErrorTypeStorage represents storage-related errors (may be transient)
  ErrorTypeStorage
  
  // ErrorTypeCorruption represents data corruption errors
  ErrorTypeCorruption
  
  // ErrorTypeConfiguration represents configuration errors
  ErrorTypeConfiguration
  
  // ErrorTypeContext represents context cancellation or timeout
  ErrorTypeContext
  
  // ErrorTypeAPI represents general API errors
  ErrorTypeAPI
)

// String returns the string representation of ErrorType
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

// IsRetryable returns whether the error type is retryable
func (et ErrorType) IsRetryable() bool {
  switch et {
  case ErrorTypeNetwork, ErrorTypeAPIQuota, ErrorTypeStorage:
    return true
  case ErrorTypePermission, ErrorTypeConfiguration:
    return false
  case ErrorTypeCorruption:
    return true // Retry from start
  default:
    return false
  }
}

// Error represents a structured error with metadata
type Error struct {
  // Type categorizes the error
  Type ErrorType
  
  // Op represents the operation being performed
  Op string
  
  // Path represents the file or resource path
  Path string
  
  // Err is the underlying error
  Err error
  
  // Code is the error code (e.g., HTTP status code)
  Code int
  
  // Retry contains retry-specific information
  Retry *RetryInfo
  
  // Context contains additional context information
  Context map[string]interface{}
  
  // Timestamp when the error occurred
  Timestamp time.Time
}

// RetryInfo contains information about retry attempts
type RetryInfo struct {
  // Attempt is the current retry attempt number
  Attempt int
  
  // MaxAttempts is the maximum number of retry attempts
  MaxAttempts int
  
  // NextRetry is when the next retry should occur
  NextRetry time.Time
  
  // BackoffDuration is the current backoff duration
  BackoffDuration time.Duration
}

// Error implements the error interface
func (e *Error) Error() string {
  if e.Path != "" {
    return fmt.Sprintf("%s: %s [%s] %v", e.Type, e.Op, e.Path, e.Err)
  }
  return fmt.Sprintf("%s: %s %v", e.Type, e.Op, e.Err)
}

// Unwrap returns the underlying error
func (e *Error) Unwrap() error {
  return e.Err
}

// IsRetryable returns whether the error is retryable
func (e *Error) IsRetryable() bool {
  return e.Type.IsRetryable()
}

// ShouldRetry checks if the error should be retried based on attempts
func (e *Error) ShouldRetry() bool {
  if !e.IsRetryable() || e.Retry == nil {
    return false
  }
  return e.Retry.Attempt < e.Retry.MaxAttempts
}

// New creates a new Error
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

// WithCode adds an error code
func (e *Error) WithCode(code int) *Error {
  e.Code = code
  return e
}

// WithContext adds context information
func (e *Error) WithContext(key string, value interface{}) *Error {
  if e.Context == nil {
    e.Context = make(map[string]interface{})
  }
  e.Context[key] = value
  return e
}

// WithRetry adds retry information
func (e *Error) WithRetry(attempt, maxAttempts int, backoff time.Duration) *Error {
  e.Retry = &RetryInfo{
    Attempt:         attempt,
    MaxAttempts:     maxAttempts,
    NextRetry:       time.Now().Add(backoff),
    BackoffDuration: backoff,
  }
  return e
}

// ErrorBatch represents a collection of errors from batch operations
type ErrorBatch struct {
  Errors []*Error
  Op     string
}

// Error implements the error interface for ErrorBatch
func (eb *ErrorBatch) Error() string {
  if len(eb.Errors) == 0 {
    return fmt.Sprintf("%s: no errors", eb.Op)
  }
  return fmt.Sprintf("%s: %d errors occurred", eb.Op, len(eb.Errors))
}

// Add adds an error to the batch
func (eb *ErrorBatch) Add(err *Error) {
  eb.Errors = append(eb.Errors, err)
}

// HasErrors returns whether the batch contains any errors
func (eb *ErrorBatch) HasErrors() bool {
  return len(eb.Errors) > 0
}

// RetryableErrors returns only retryable errors
func (eb *ErrorBatch) RetryableErrors() []*Error {
  var retryable []*Error
  for _, err := range eb.Errors {
    if err.IsRetryable() {
      retryable = append(retryable, err)
    }
  }
  return retryable
}

// ErrorsByType groups errors by their type
func (eb *ErrorBatch) ErrorsByType() map[ErrorType][]*Error {
  grouped := make(map[ErrorType][]*Error)
  for _, err := range eb.Errors {
    grouped[err.Type] = append(grouped[err.Type], err)
  }
  return grouped
}

// IsContextError checks if the error is due to context cancellation
func IsContextError(err error) bool {
  if err == nil {
    return false
  }
  return err == context.Canceled || err == context.DeadlineExceeded
}

// GetErrorType attempts to determine the error type from a generic error
func GetErrorType(err error) ErrorType {
  if err == nil {
    return ErrorTypeUnknown
  }
  
  // Check for context errors
  if IsContextError(err) {
    return ErrorTypeContext
  }
  
  // TODO: Add more error type detection based on error messages
  // or specific error types from various packages
  
  return ErrorTypeUnknown
}