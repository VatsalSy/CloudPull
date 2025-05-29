/**
 * Error Handling System Tests
 * 
 * Unit tests for error types, handler, and retry logic to ensure
 * robust error handling behavior.
 * 
 * Author: CloudPull Team
 * Created: 2025-01-29
 */

package errors

import (
  "context"
  "fmt"
  "testing"
  "time"
  
  "github.com/stretchr/testify/assert"
  "github.com/stretchr/testify/require"
)

// Mock logger for testing
type mockLogger struct {
  logs []logEntry
}

type logEntry struct {
  level   string
  message string
  err     error
  fields  []interface{}
}

func (m *mockLogger) Error(err error, msg string, fields ...interface{}) {
  m.logs = append(m.logs, logEntry{level: "error", message: msg, err: err, fields: fields})
}

func (m *mockLogger) Warn(msg string, fields ...interface{}) {
  m.logs = append(m.logs, logEntry{level: "warn", message: msg, fields: fields})
}

func (m *mockLogger) Info(msg string, fields ...interface{}) {
  m.logs = append(m.logs, logEntry{level: "info", message: msg, fields: fields})
}

func (m *mockLogger) Debug(msg string, fields ...interface{}) {
  m.logs = append(m.logs, logEntry{level: "debug", message: msg, fields: fields})
}

// Test error type functionality
func TestErrorTypes(t *testing.T) {
  tests := []struct {
    name       string
    errorType  ErrorType
    retryable  bool
    stringRep  string
  }{
    {"Network", ErrorTypeNetwork, true, "Network"},
    {"APIQuota", ErrorTypeAPIQuota, true, "APIQuota"},
    {"Permission", ErrorTypePermission, false, "Permission"},
    {"Storage", ErrorTypeStorage, true, "Storage"},
    {"Corruption", ErrorTypeCorruption, true, "Corruption"},
    {"Configuration", ErrorTypeConfiguration, false, "Configuration"},
    {"Context", ErrorTypeContext, false, "Context"},
    {"Unknown", ErrorTypeUnknown, false, "Unknown"},
  }
  
  for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
      assert.Equal(t, tt.retryable, tt.errorType.IsRetryable())
      assert.Equal(t, tt.stringRep, tt.errorType.String())
    })
  }
}

// Test error creation and methods
func TestError(t *testing.T) {
  baseErr := fmt.Errorf("base error")
  
  t.Run("NewError", func(t *testing.T) {
    err := New(ErrorTypeNetwork, "test_op", "/path/to/file", baseErr)
    
    assert.Equal(t, ErrorTypeNetwork, err.Type)
    assert.Equal(t, "test_op", err.Op)
    assert.Equal(t, "/path/to/file", err.Path)
    assert.Equal(t, baseErr, err.Err)
    assert.NotZero(t, err.Timestamp)
    assert.NotNil(t, err.Context)
  })
  
  t.Run("ErrorMethods", func(t *testing.T) {
    err := New(ErrorTypeNetwork, "test_op", "/path/to/file", baseErr)
    
    // Test Error() method
    expectedMsg := "Network: test_op [/path/to/file] base error"
    assert.Equal(t, expectedMsg, err.Error())
    
    // Test Unwrap()
    assert.Equal(t, baseErr, err.Unwrap())
    
    // Test IsRetryable()
    assert.True(t, err.IsRetryable())
  })
  
  t.Run("WithMethods", func(t *testing.T) {
    err := New(ErrorTypeAPIQuota, "api_call", "", baseErr).
      WithCode(429).
      WithContext("endpoint", "/api/files").
      WithRetry(1, 3, 5*time.Second)
    
    assert.Equal(t, 429, err.Code)
    assert.Equal(t, "/api/files", err.Context["endpoint"])
    assert.NotNil(t, err.Retry)
    assert.Equal(t, 1, err.Retry.Attempt)
    assert.Equal(t, 3, err.Retry.MaxAttempts)
    assert.Equal(t, 5*time.Second, err.Retry.BackoffDuration)
  })
  
  t.Run("ShouldRetry", func(t *testing.T) {
    // Retryable error with retry info
    err1 := New(ErrorTypeNetwork, "test", "", baseErr).
      WithRetry(1, 3, time.Second)
    assert.True(t, err1.ShouldRetry())
    
    // Retryable error at max attempts
    err2 := New(ErrorTypeNetwork, "test", "", baseErr).
      WithRetry(3, 3, time.Second)
    assert.False(t, err2.ShouldRetry())
    
    // Non-retryable error
    err3 := New(ErrorTypePermission, "test", "", baseErr)
    assert.False(t, err3.ShouldRetry())
  })
}

// Test error batch functionality
func TestErrorBatch(t *testing.T) {
  t.Run("BasicOperations", func(t *testing.T) {
    batch := &ErrorBatch{Op: "batch_test"}
    
    assert.False(t, batch.HasErrors())
    assert.Contains(t, batch.Error(), "no errors")
    
    // Add errors
    err1 := New(ErrorTypeNetwork, "op1", "file1", fmt.Errorf("network error"))
    err2 := New(ErrorTypePermission, "op2", "file2", fmt.Errorf("permission denied"))
    err3 := New(ErrorTypeStorage, "op3", "file3", fmt.Errorf("disk full"))
    
    batch.Add(err1)
    batch.Add(err2)
    batch.Add(err3)
    
    assert.True(t, batch.HasErrors())
    assert.Equal(t, 3, len(batch.Errors))
    assert.Contains(t, batch.Error(), "3 errors occurred")
  })
  
  t.Run("RetryableErrors", func(t *testing.T) {
    batch := &ErrorBatch{Op: "batch_test"}
    
    batch.Add(New(ErrorTypeNetwork, "op1", "", nil))
    batch.Add(New(ErrorTypePermission, "op2", "", nil))
    batch.Add(New(ErrorTypeStorage, "op3", "", nil))
    
    retryable := batch.RetryableErrors()
    assert.Equal(t, 2, len(retryable))
  })
  
  t.Run("ErrorsByType", func(t *testing.T) {
    batch := &ErrorBatch{Op: "batch_test"}
    
    batch.Add(New(ErrorTypeNetwork, "op1", "", nil))
    batch.Add(New(ErrorTypeNetwork, "op2", "", nil))
    batch.Add(New(ErrorTypePermission, "op3", "", nil))
    
    byType := batch.ErrorsByType()
    assert.Equal(t, 2, len(byType[ErrorTypeNetwork]))
    assert.Equal(t, 1, len(byType[ErrorTypePermission]))
  })
}

// Test error handler
func TestHandler(t *testing.T) {
  ctx := context.Background()
  logger := &mockLogger{}
  handler := NewHandler(logger)
  
  t.Run("HandleError", func(t *testing.T) {
    tests := []struct {
      name     string
      err      *Error
      expected RecoveryStrategy
    }{
      {
        name:     "PermissionError",
        err:      New(ErrorTypePermission, "test", "", nil),
        expected: RecoveryStrategyNone,
      },
      {
        name:     "NetworkErrorWithRetry",
        err:      New(ErrorTypeNetwork, "test", "", nil).WithRetry(1, 3, time.Second),
        expected: RecoveryStrategyBackoff,
      },
      {
        name:     "NetworkErrorMaxRetries",
        err:      New(ErrorTypeNetwork, "test", "", nil).WithRetry(3, 3, time.Second),
        expected: RecoveryStrategyNone,
      },
      {
        name:     "CorruptionError",
        err:      New(ErrorTypeCorruption, "test", "", nil).WithRetry(0, 2, time.Second),
        expected: RecoveryStrategyRestart,
      },
    }
    
    for _, tt := range tests {
      t.Run(tt.name, func(t *testing.T) {
        strategy := handler.HandleError(ctx, tt.err)
        assert.Equal(t, tt.expected, strategy)
      })
    }
  })
  
  t.Run("PrepareRetry", func(t *testing.T) {
    err := New(ErrorTypeNetwork, "test", "", nil)
    
    // First retry
    err = handler.PrepareRetry(err)
    assert.NotNil(t, err.Retry)
    assert.Equal(t, 1, err.Retry.Attempt)
    assert.Equal(t, 5, err.Retry.MaxAttempts)
    assert.True(t, err.Retry.BackoffDuration >= 1*time.Second)
    
    // Second retry
    err = handler.PrepareRetry(err)
    assert.Equal(t, 2, err.Retry.Attempt)
    assert.True(t, err.Retry.BackoffDuration >= err.Retry.BackoffDuration)
  })
  
  t.Run("RecoverWithStrategy", func(t *testing.T) {
    attempts := 0
    operation := func() error {
      attempts++
      if attempts < 3 {
        return fmt.Errorf("operation failed")
      }
      return nil
    }
    
    err := New(ErrorTypeNetwork, "test", "", fmt.Errorf("initial error"))
    
    // Test successful retry
    result := handler.RecoverWithStrategy(ctx, err, RecoveryStrategyRetry, operation)
    assert.Nil(t, result)
    assert.Equal(t, 3, attempts)
  })
}

// Test exponential backoff
func TestExponentialBackoff(t *testing.T) {
  t.Run("BasicBackoff", func(t *testing.T) {
    config := &BackoffConfig{
      InitialInterval:     100 * time.Millisecond,
      MaxInterval:         1 * time.Second,
      Multiplier:          2.0,
      RandomizationFactor: 0,
    }
    
    backoff := NewExponentialBackoff(config)
    
    // First backoff
    interval1 := backoff.NextBackOff()
    assert.Equal(t, 100*time.Millisecond, interval1)
    
    // Second backoff
    interval2 := backoff.NextBackOff()
    assert.Equal(t, 200*time.Millisecond, interval2)
    
    // Third backoff
    interval3 := backoff.NextBackOff()
    assert.Equal(t, 400*time.Millisecond, interval3)
  })
  
  t.Run("MaxInterval", func(t *testing.T) {
    config := &BackoffConfig{
      InitialInterval:     500 * time.Millisecond,
      MaxInterval:         1 * time.Second,
      Multiplier:          3.0,
      RandomizationFactor: 0,
    }
    
    backoff := NewExponentialBackoff(config)
    
    // Should hit max interval
    backoff.NextBackOff()
    interval := backoff.NextBackOff()
    assert.Equal(t, 1*time.Second, interval)
  })
  
  t.Run("MaxElapsedTime", func(t *testing.T) {
    config := &BackoffConfig{
      InitialInterval: 100 * time.Millisecond,
      MaxElapsedTime:  200 * time.Millisecond,
    }
    
    backoff := NewExponentialBackoff(config)
    
    // First backoff should work
    interval1 := backoff.NextBackOff()
    assert.True(t, interval1 > 0)
    
    // Wait to exceed max elapsed time
    time.Sleep(250 * time.Millisecond)
    
    // Next backoff should return -1
    interval2 := backoff.NextBackOff()
    assert.Equal(t, time.Duration(-1), interval2)
  })
}

// Test retry operation
func TestRetryOperation(t *testing.T) {
  t.Run("SuccessfulRetry", func(t *testing.T) {
    ctx := context.Background()
    attempts := 0
    
    operation := func() error {
      attempts++
      if attempts < 3 {
        return fmt.Errorf("temporary error")
      }
      return nil
    }
    
    shouldRetry := func(err error) bool {
      return err != nil && attempts < 5
    }
    
    config := &BackoffConfig{
      InitialInterval:     10 * time.Millisecond,
      MaxInterval:         100 * time.Millisecond,
      Multiplier:          2.0,
      RandomizationFactor: 0,
    }
    
    err := RetryOperation(ctx, operation, config, shouldRetry)
    assert.Nil(t, err)
    assert.Equal(t, 3, attempts)
  })
  
  t.Run("ContextCancellation", func(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    
    operation := func() error {
      return fmt.Errorf("error")
    }
    
    shouldRetry := func(err error) bool {
      return true
    }
    
    config := &BackoffConfig{
      InitialInterval: 100 * time.Millisecond,
    }
    
    // Cancel context after short delay
    go func() {
      time.Sleep(50 * time.Millisecond)
      cancel()
    }()
    
    err := RetryOperation(ctx, operation, config, shouldRetry)
    assert.Equal(t, context.Canceled, err)
  })
}

// Test adaptive backoff
func TestAdaptiveBackoff(t *testing.T) {
  t.Run("SuccessAdaptation", func(t *testing.T) {
    config := &BackoffConfig{
      InitialInterval:     1 * time.Second,
      MaxInterval:         10 * time.Second,
      Multiplier:          2.0,
      RandomizationFactor: 0,
    }
    
    adaptive := NewAdaptiveBackoff(config, &AdaptiveConfig{
      SuccessThreshold: 2,
      AdaptationFactor: 0.5,
    })
    
    // Record successes
    adaptive.RecordSuccess()
    adaptive.RecordSuccess()
    
    // Interval should be reduced
    interval := adaptive.NextBackOff()
    assert.True(t, interval < 1*time.Second)
  })
  
  t.Run("ErrorAdaptation", func(t *testing.T) {
    config := &BackoffConfig{
      InitialInterval:     1 * time.Second,
      MaxInterval:         10 * time.Second,
      Multiplier:          2.0,
      RandomizationFactor: 0,
    }
    
    adaptive := NewAdaptiveBackoff(config, &AdaptiveConfig{
      ErrorThreshold:   3,
      AdaptationFactor: 0.5,
    })
    
    // Record repeated errors
    for i := 0; i < 3; i++ {
      adaptive.RecordError(ErrorTypeNetwork)
    }
    
    // Interval should be increased
    interval := adaptive.NextBackOff()
    assert.True(t, interval > 1*time.Second)
  })
}

// Test context error detection
func TestIsContextError(t *testing.T) {
  assert.True(t, IsContextError(context.Canceled))
  assert.True(t, IsContextError(context.DeadlineExceeded))
  assert.False(t, IsContextError(fmt.Errorf("other error")))
  assert.False(t, IsContextError(nil))
}

// Test wrap with context
func TestWrapWithContext(t *testing.T) {
  ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Hour))
  defer cancel()
  
  baseErr := fmt.Errorf("base error")
  wrapped := WrapWithContext(ctx, baseErr, "test_op", "/path/to/file")
  
  assert.NotNil(t, wrapped)
  assert.Equal(t, "test_op", wrapped.Op)
  assert.Equal(t, "/path/to/file", wrapped.Path)
  assert.Equal(t, baseErr, wrapped.Err)
  assert.NotNil(t, wrapped.Context["deadline"])
  
  // Test nil error
  assert.Nil(t, WrapWithContext(ctx, nil, "test_op", ""))
  
  // Test already wrapped error
  rewrapped := WrapWithContext(ctx, wrapped, "new_op", "new_path")
  assert.Equal(t, wrapped, rewrapped)
}