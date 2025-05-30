/**
 * Example Usage of Error Handling System
 *
 * Demonstrates how to integrate the error handling, retry logic, and logging
 * components in CloudPull operations.
 *
 * Author: CloudPull Team
 * Created: 2025-01-29
 */

package errors

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/VatsalSy/CloudPull/internal/logger"
)

// Example: File sync operation with error handling.
func ExampleFileSyncWithErrorHandling(ctx context.Context) error {
	// Initialize logger
	log := logger.New(&logger.Config{
		Level:         "debug",
		Pretty:        true,
		IncludeCaller: true,
	})

	// Create error handler
	handler := NewHandler(log)

	// Example file sync operation
	filePath := "/path/to/file.txt"

	// Wrap operation with error handling
	err := performFileSync(ctx, filePath)
	if err != nil {
		// Wrap error with context
		syncErr := WrapWithContext(ctx, err, "file_sync", filePath)

		// Determine recovery strategy
		strategy := handler.HandleError(ctx, syncErr)

		// Execute recovery
		if strategy != RecoveryStrategyNone {
			err = handler.RecoverWithStrategy(ctx, syncErr, strategy, func() error {
				return performFileSync(ctx, filePath)
			})
		}

		if err != nil {
			return err
		}
	}

	return nil
}

// Example: Batch operation with error aggregation.
func ExampleBatchOperationWithErrors(ctx context.Context, files []string) error {
	log := logger.Global()
	handler := NewHandler(log)

	// Create error batch
	batch := &ErrorBatch{
		Op: "batch_upload",
	}

	// Process files
	for _, file := range files {
		if err := uploadFile(ctx, file); err != nil {
			// Create structured error
			uploadErr := New(determineErrorType(err), "upload", file, err)

			// Add to batch
			batch.Add(uploadErr)
		}
	}

	// Handle batch errors if any occurred
	if batch.HasErrors() {
		// Get recovery strategies for each error
		strategies := handler.HandleBatchErrors(ctx, batch)

		// Retry retryable errors
		for _, err := range batch.RetryableErrors() {
			strategy := strategies[err]
			if strategy == RecoveryStrategyBackoff {
				retryErr := handler.RecoverWithStrategy(ctx, err, strategy, func() error {
					return uploadFile(ctx, err.Path)
				})

				if retryErr != nil {
					log.Error(retryErr, "Failed to recover from error",
						"file", err.Path,
						"error_type", err.Type.String(),
					)
				}
			}
		}

		// Log error summary
		errorsByType := batch.ErrorsByType()
		for errType, errors := range errorsByType {
			log.Warn("Batch errors by type",
				"type", errType.String(),
				"count", len(errors),
			)
		}
	}

	return nil
}

// Example: API call with quota handling.
func ExampleAPICallWithQuotaHandling(ctx context.Context) error {
	log := logger.Global()

	// Configure specific retry policy for API quota errors
	// TODO: Implement quota-specific retry policy
	// quotaPolicy := &RetryPolicy{
	//   MaxAttempts:  10,
	//   InitialDelay: 10 * time.Second,
	//   MaxDelay:     5 * time.Minute,
	//   Multiplier:   1.5,
	//   Jitter:       true,
	// }

	// Use adaptive backoff for API calls
	adaptiveBackoff := NewAdaptiveBackoff(
		&BackoffConfig{
			InitialInterval:     5 * time.Second,
			MaxInterval:         2 * time.Minute,
			Multiplier:          1.5,
			RandomizationFactor: 0.3,
		},
		DefaultAdaptiveConfig,
	)

	// Retry operation with adaptive backoff
	var lastErr error
	for i := 0; i < 10; i++ {
		err := makeAPICall(ctx)
		if err == nil {
			adaptiveBackoff.RecordSuccess()
			return nil
		}

		// Determine error type
		apiErr := categorizeAPIError(err)

		if !apiErr.IsRetryable() {
			return apiErr
		}

		// Record error for adaptive backoff
		adaptiveBackoff.RecordError(apiErr.Type)

		// Wait for next attempt
		backoff := adaptiveBackoff.NextBackOff()
		if backoff < 0 {
			break
		}

		log.Debug("Waiting for API retry",
			"attempt", i+1,
			"backoff", backoff,
			"error_type", apiErr.Type.String(),
		)

		select {
		case <-ctx.Done():
			return New(ErrorTypeContext, "api_call", "", ctx.Err())
		case <-time.After(backoff):
			continue
		}

	}

	return lastErr
}

// Example: Storage operation with corruption handling.
func ExampleStorageOperationWithCorruptionHandling(ctx context.Context) error {
	log := logger.Global()
	handler := NewHandler(log)

	// Create checkpointed operation
	checkpoint := &OperationCheckpoint{
		ID:        "storage_sync_123",
		StartTime: time.Now(),
		Progress:  0,
	}

	// Perform operation with checkpoint recovery
	err := performStorageOperation(ctx, checkpoint)
	if err != nil {
		storageErr := categorizeStorageError(err)

		if storageErr.Type == ErrorTypeCorruption {
			log.Warn("Corruption detected, restarting from checkpoint",
				"checkpoint_id", checkpoint.ID,
				"progress", checkpoint.Progress,
			)

			// Reset to last known good state
			checkpoint.Reset()

			// Retry with restart strategy
			err = handler.RecoverWithStrategy(ctx, storageErr, RecoveryStrategyRestart, func() error {
				return performStorageOperation(ctx, checkpoint)
			})
		}
	}

	return err
}

// Helper types and functions for examples

type OperationCheckpoint struct {
	StartTime time.Time
	State     map[string]interface{}
	ID        string
	Progress  int
}

func (oc *OperationCheckpoint) Reset() {
	oc.Progress = 0
	oc.State = make(map[string]interface{})
}

func performFileSync(ctx context.Context, path string) error {
	// Simulated file sync
	return nil
}

func uploadFile(ctx context.Context, path string) error {
	// Simulated file upload
	return nil
}

func makeAPICall(ctx context.Context) error {
	// Simulated API call
	return nil
}

func performStorageOperation(ctx context.Context, checkpoint *OperationCheckpoint) error {
	// Simulated storage operation
	return nil
}

func determineErrorType(err error) ErrorType {
	// Simple error type detection
	if err == context.DeadlineExceeded || err == context.Canceled {
		return ErrorTypeContext
	}

	// Add more sophisticated error detection
	return ErrorTypeUnknown
}

func categorizeAPIError(err error) *Error {
	// Example API error categorization
	if httpErr, ok := err.(*httpError); ok {
		switch httpErr.StatusCode {
		case http.StatusTooManyRequests:
			return New(ErrorTypeAPIQuota, "api_call", "", err).
				WithCode(httpErr.StatusCode)
		case http.StatusUnauthorized, http.StatusForbidden:
			return New(ErrorTypePermission, "api_call", "", err).
				WithCode(httpErr.StatusCode)
		case http.StatusInternalServerError, http.StatusBadGateway:
			return New(ErrorTypeNetwork, "api_call", "", err).
				WithCode(httpErr.StatusCode)
		}
	}

	return New(ErrorTypeUnknown, "api_call", "", err)
}

func categorizeStorageError(err error) *Error {
	// Example storage error categorization
	errStr := err.Error()

	if contains(errStr, "corruption", "checksum", "invalid") {
		return New(ErrorTypeCorruption, "storage_operation", "", err)
	}

	if contains(errStr, "permission", "access denied") {
		return New(ErrorTypePermission, "storage_operation", "", err)
	}

	if contains(errStr, "disk full", "no space") {
		return New(ErrorTypeStorage, "storage_operation", "", err)
	}

	return New(ErrorTypeNetwork, "storage_operation", "", err)
}

func contains(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if len(s) >= len(substr) && s[:len(substr)] == substr {
			return true
		}
	}
	return false
}

type httpError struct {
	Message    string
	StatusCode int
}

func (e *httpError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}
