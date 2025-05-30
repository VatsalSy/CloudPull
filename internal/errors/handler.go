/**
 * Error Handler for CloudPull
 *
 * Implements error handling strategies with configurable retry policies,
 * error recovery, and context-aware error propagation.
 *
 * Author: CloudPull Team
 * Created: 2025-01-29
 */

package errors

import (
	"context"
	"time"
)

// RecoveryStrategy defines how to recover from specific error types.
type RecoveryStrategy int

const (
	// RecoveryStrategyNone indicates no recovery possible
	RecoveryStrategyNone RecoveryStrategy = iota

	// RecoveryStrategyRetry indicates simple retry
	RecoveryStrategyRetry

	// RecoveryStrategyBackoff indicates retry with exponential backoff
	RecoveryStrategyBackoff

	// RecoveryStrategyRestart indicates restart from beginning
	RecoveryStrategyRestart

	// RecoveryStrategySkip indicates skip and continue
	RecoveryStrategySkip
)

// RetryPolicy defines the retry behavior for errors.
type RetryPolicy struct {
	// MaxAttempts is the maximum number of retry attempts
	MaxAttempts int

	// InitialDelay is the initial delay between retries
	InitialDelay time.Duration

	// MaxDelay is the maximum delay between retries
	MaxDelay time.Duration

	// Multiplier is the backoff multiplier
	Multiplier float64

	// Jitter adds randomness to prevent thundering herd
	Jitter bool
}

// DefaultRetryPolicies provides default retry policies for each error type.
var DefaultRetryPolicies = map[ErrorType]*RetryPolicy{
	ErrorTypeNetwork: {
		MaxAttempts:  5,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
	},
	ErrorTypeAPIQuota: {
		MaxAttempts:  10,
		InitialDelay: 5 * time.Second,
		MaxDelay:     5 * time.Minute,
		Multiplier:   2.0,
		Jitter:       true,
	},
	ErrorTypeStorage: {
		MaxAttempts:  3,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
		Jitter:       false,
	},
	ErrorTypeCorruption: {
		MaxAttempts:  2,
		InitialDelay: 5 * time.Second,
		MaxDelay:     10 * time.Second,
		Multiplier:   1.0,
		Jitter:       false,
	},
}

// Handler manages error handling and recovery.
type Handler struct {
	policies map[ErrorType]*RetryPolicy
	logger   Logger
}

// Logger interface for error logging.
type Logger interface {
	Error(err error, msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Debug(msg string, fields ...interface{})
}

// NewHandler creates a new error handler.
func NewHandler(logger Logger) *Handler {
	return &Handler{
		policies: DefaultRetryPolicies,
		logger:   logger,
	}
}

// SetRetryPolicy sets a custom retry policy for an error type.
func (h *Handler) SetRetryPolicy(errorType ErrorType, policy *RetryPolicy) {
	h.policies[errorType] = policy
}

// GetRetryPolicy returns the retry policy for an error type.
func (h *Handler) GetRetryPolicy(errorType ErrorType) *RetryPolicy {
	if policy, ok := h.policies[errorType]; ok {
		return policy
	}
	return nil
}

// HandleError processes an error and determines the recovery strategy.
func (h *Handler) HandleError(ctx context.Context, err *Error) RecoveryStrategy {
	// Log the error with context
	h.logError(err)

	// Check for context cancellation
	if err.Type == ErrorTypeContext {
		return RecoveryStrategyNone
	}

	// Determine recovery strategy based on error type
	switch err.Type {
	case ErrorTypePermission, ErrorTypeConfiguration:
		return RecoveryStrategyNone

	case ErrorTypeNetwork, ErrorTypeAPIQuota, ErrorTypeStorage:
		if err.ShouldRetry() {
			return RecoveryStrategyBackoff
		}
		return RecoveryStrategyNone

	case ErrorTypeCorruption:
		if err.ShouldRetry() {
			return RecoveryStrategyRestart
		}
		return RecoveryStrategyNone

	default:
		return RecoveryStrategyNone
	}
}

// PrepareRetry prepares an error for retry.
func (h *Handler) PrepareRetry(err *Error) *Error {
	policy := h.GetRetryPolicy(err.Type)
	if policy == nil {
		return err
	}

	// Initialize retry info if not present
	if err.Retry == nil {
		err.Retry = &RetryInfo{
			Attempt:     0,
			MaxAttempts: policy.MaxAttempts,
		}
	}

	// Increment attempt
	err.Retry.Attempt++

	// Calculate backoff
	backoff := calculateBackoff(
		err.Retry.Attempt,
		policy.InitialDelay,
		policy.MaxDelay,
		policy.Multiplier,
		policy.Jitter,
	)

	err.Retry.BackoffDuration = backoff
	err.Retry.NextRetry = time.Now().Add(backoff)

	return err
}

// WaitForRetry waits for the appropriate backoff duration.
func (h *Handler) WaitForRetry(ctx context.Context, err *Error) error {
	if err.Retry == nil {
		return nil
	}

	h.logger.Debug("Waiting for retry",
		"error_type", err.Type.String(),
		"attempt", err.Retry.Attempt,
		"backoff", err.Retry.BackoffDuration,
		"next_retry", err.Retry.NextRetry,
	)

	timer := time.NewTimer(err.Retry.BackoffDuration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// HandleBatchErrors processes a batch of errors.
func (h *Handler) HandleBatchErrors(ctx context.Context, batch *ErrorBatch) map[*Error]RecoveryStrategy {
	strategies := make(map[*Error]RecoveryStrategy)

	// Group errors by type for efficient handling
	errorsByType := batch.ErrorsByType()

	// Log batch error summary
	h.logger.Warn("Batch operation encountered errors",
		"operation", batch.Op,
		"total_errors", len(batch.Errors),
		"error_types", len(errorsByType),
	)

	// Process each error
	for _, err := range batch.Errors {
		strategies[err] = h.HandleError(ctx, err)
	}

	return strategies
}

// RecoverWithStrategy executes a recovery strategy.
func (h *Handler) RecoverWithStrategy(
	ctx context.Context,
	err *Error,
	strategy RecoveryStrategy,
	operation func() error,
) error {

	switch strategy {
	case RecoveryStrategyNone:
		return err

	case RecoveryStrategyRetry:
		return h.retryOperation(ctx, err, operation, false)

	case RecoveryStrategyBackoff:
		return h.retryOperation(ctx, err, operation, true)

	case RecoveryStrategyRestart:
		h.logger.Info("Restarting operation from beginning",
			"error_type", err.Type.String(),
			"operation", err.Op,
		)
		// Reset error state
		err.Retry = nil
		return h.retryOperation(ctx, err, operation, true)

	case RecoveryStrategySkip:
		h.logger.Warn("Skipping failed operation",
			"error_type", err.Type.String(),
			"operation", err.Op,
			"path", err.Path,
		)
		return nil

	default:
		return err
	}
}

// retryOperation performs the retry logic.
func (h *Handler) retryOperation(
	ctx context.Context,
	err *Error,
	operation func() error,
	withBackoff bool,
) error {
	// Prepare for retry
	err = h.PrepareRetry(err)

	for err.ShouldRetry() {
		// Wait for backoff if required
		if withBackoff {
			if waitErr := h.WaitForRetry(ctx, err); waitErr != nil {
				return New(ErrorTypeContext, "retry wait", "", waitErr)
			}
		}

		// Attempt operation
		if opErr := operation(); opErr == nil {
			h.logger.Info("Operation succeeded after retry",
				"error_type", err.Type.String(),
				"operation", err.Op,
				"attempt", err.Retry.Attempt,
			)
			return nil
		} else {
			// Update error with new attempt
			err.Err = opErr
			err = h.PrepareRetry(err)
		}
	}

	h.logger.Error(err, "Operation failed after all retry attempts",
		"error_type", err.Type.String(),
		"operation", err.Op,
		"attempts", err.Retry.Attempt,
	)

	return err
}

// logError logs an error with appropriate context.
func (h *Handler) logError(err *Error) {
	fields := []interface{}{
		"error_type", err.Type.String(),
		"operation", err.Op,
		"timestamp", err.Timestamp,
	}

	if err.Path != "" {
		fields = append(fields, "path", err.Path)
	}

	if err.Code != 0 {
		fields = append(fields, "code", err.Code)
	}

	if err.Retry != nil {
		fields = append(fields,
			"attempt", err.Retry.Attempt,
			"max_attempts", err.Retry.MaxAttempts,
		)
	}

	// Add context fields
	for k, v := range err.Context {
		fields = append(fields, k, v)
	}

	h.logger.Error(err, "Error occurred", fields...)
}

// WrapWithContext wraps an error with context information.
func WrapWithContext(ctx context.Context, err error, op, path string) *Error {
	if err == nil {
		return nil
	}

	// Check if already wrapped
	if e, ok := err.(*Error); ok {
		return e
	}

	// Determine error type
	errorType := GetErrorType(err)

	// Create wrapped error
	wrapped := New(errorType, op, path, err)

	// Add context information
	if deadline, ok := ctx.Deadline(); ok {
		wrapped.WithContext("deadline", deadline)
	}

	return wrapped
}
