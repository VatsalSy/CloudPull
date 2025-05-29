/**
 * Integration Example for Error Handling System
 * 
 * Demonstrates a complete integration of error handling, retry logic,
 * and logging for a cloud sync operation.
 * 
 * Author: CloudPull Team
 * Created: 2025-01-29
 */

package errors

import (
  "context"
  "fmt"
  "io"
  "net/http"
  "time"
  
  "github.com/cloudpull/cloudpull/internal/logger"
)

// CloudSyncManager demonstrates integrated error handling
type CloudSyncManager struct {
  logger  *logger.Logger
  handler *Handler
  client  *http.Client
}

// NewCloudSyncManager creates a new sync manager with error handling
func NewCloudSyncManager() *CloudSyncManager {
  // Initialize logger with appropriate config
  log := logger.New(&logger.Config{
    Level:         "info",
    Pretty:        true,
    IncludeCaller: true,
    Fields: map[string]interface{}{
      "component": "sync_manager",
    },
  })
  
  // Create error handler
  handler := NewHandler(log)
  
  // Configure custom retry policies for specific scenarios
  handler.SetRetryPolicy(ErrorTypeAPIQuota, &RetryPolicy{
    MaxAttempts:  20,
    InitialDelay: 30 * time.Second,
    MaxDelay:     10 * time.Minute,
    Multiplier:   1.5,
    Jitter:       true,
  })
  
  return &CloudSyncManager{
    logger:  log,
    handler: handler,
    client:  &http.Client{Timeout: 30 * time.Second},
  }
}

// SyncFile demonstrates error handling for a single file sync
func (csm *CloudSyncManager) SyncFile(ctx context.Context, localPath, remotePath string) error {
  // Create operation logger
  opLog := csm.logger.WithField("operation", "sync_file").
    WithField("local_path", localPath).
    WithField("remote_path", remotePath)
  
  opLog.Info("Starting file sync")
  
  // Perform sync with error handling
  err := csm.performSync(ctx, localPath, remotePath)
  if err != nil {
    // Wrap error with context
    syncErr := WrapWithContext(ctx, err, "sync_file", localPath)
    
    // Log initial error
    opLog.Error(syncErr, "Sync failed, attempting recovery")
    
    // Determine recovery strategy
    strategy := csm.handler.HandleError(ctx, syncErr)
    
    // Execute recovery if possible
    if strategy != RecoveryStrategyNone {
      opLog.Info("Attempting recovery", "strategy", strategy)
      
      err = csm.handler.RecoverWithStrategy(ctx, syncErr, strategy, func() error {
        return csm.performSync(ctx, localPath, remotePath)
      })
      
      if err == nil {
        opLog.Info("Recovery successful")
      } else {
        opLog.Error(err, "Recovery failed")
        return err
      }
    } else {
      return syncErr
    }
  }
  
  opLog.Info("File sync completed successfully")
  return nil
}

// SyncBatch demonstrates batch error handling
func (csm *CloudSyncManager) SyncBatch(ctx context.Context, files []SyncItem) error {
  opLog := csm.logger.WithField("operation", "sync_batch").
    WithField("file_count", len(files))
  
  opLog.Info("Starting batch sync")
  
  // Create error batch
  batch := &ErrorBatch{
    Op: "batch_sync",
  }
  
  // Track successful syncs
  successCount := 0
  
  // Process each file
  for i, item := range files {
    itemLog := opLog.WithField("item_index", i).
      WithField("local_path", item.LocalPath)
    
    itemLog.Debug("Syncing item")
    
    if err := csm.performSync(ctx, item.LocalPath, item.RemotePath); err != nil {
      // Categorize error
      syncErr := csm.categorizeError(err, "sync_item", item.LocalPath)
      
      // Add context
      syncErr.WithContext("item_index", i).
        WithContext("item_size", item.Size).
        WithContext("item_type", item.Type)
      
      batch.Add(syncErr)
      itemLog.Warn("Item sync failed", "error_type", syncErr.Type.String())
    } else {
      successCount++
    }
  }
  
  // Handle batch errors
  if batch.HasErrors() {
    opLog.Warn("Batch sync completed with errors",
      "success_count", successCount,
      "error_count", len(batch.Errors),
    )
    
    // Get recovery strategies
    strategies := csm.handler.HandleBatchErrors(ctx, batch)
    
    // Attempt recovery for retryable errors
    recoveredCount := 0
    for _, err := range batch.RetryableErrors() {
      strategy := strategies[err]
      if strategy != RecoveryStrategyNone {
        itemIndex := err.Context["item_index"].(int)
        item := files[itemIndex]
        
        retryErr := csm.handler.RecoverWithStrategy(ctx, err, strategy, func() error {
          return csm.performSync(ctx, item.LocalPath, item.RemotePath)
        })
        
        if retryErr == nil {
          recoveredCount++
        }
      }
    }
    
    opLog.Info("Batch recovery completed",
      "recovered_count", recoveredCount,
      "final_error_count", len(batch.Errors)-recoveredCount,
    )
    
    // Return error if not all items succeeded
    if recoveredCount < len(batch.Errors) {
      return fmt.Errorf("batch sync failed: %d errors remaining", len(batch.Errors)-recoveredCount)
    }
  }
  
  opLog.Info("Batch sync completed successfully")
  return nil
}

// performSync simulates the actual sync operation
func (csm *CloudSyncManager) performSync(ctx context.Context, localPath, remotePath string) error {
  // Simulate various error conditions
  
  // Check context first
  select {
  case <-ctx.Done():
    return ctx.Err()
  default:
  }
  
  // Simulate API call
  req, err := http.NewRequestWithContext(ctx, "PUT", "https://api.example.com/files"+remotePath, nil)
  if err != nil {
    return err
  }
  
  resp, err := csm.client.Do(req)
  if err != nil {
    return err
  }
  defer resp.Body.Close()
  
  // Handle various HTTP status codes
  switch resp.StatusCode {
  case http.StatusOK, http.StatusCreated:
    return nil
  case http.StatusTooManyRequests:
    return &httpError{StatusCode: resp.StatusCode, Message: "Rate limit exceeded"}
  case http.StatusUnauthorized:
    return &httpError{StatusCode: resp.StatusCode, Message: "Authentication failed"}
  case http.StatusInternalServerError:
    return &httpError{StatusCode: resp.StatusCode, Message: "Server error"}
  default:
    body, _ := io.ReadAll(resp.Body)
    return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
  }
}

// categorizeError determines the error type from a generic error
func (csm *CloudSyncManager) categorizeError(err error, op, path string) *Error {
  // Context errors
  if IsContextError(err) {
    return New(ErrorTypeContext, op, path, err)
  }
  
  // HTTP errors
  if httpErr, ok := err.(*httpError); ok {
    switch httpErr.StatusCode {
    case http.StatusTooManyRequests:
      return New(ErrorTypeAPIQuota, op, path, err).
        WithCode(httpErr.StatusCode).
        WithContext("retry_after", "60s")
    case http.StatusUnauthorized, http.StatusForbidden:
      return New(ErrorTypePermission, op, path, err).
        WithCode(httpErr.StatusCode)
    case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
      return New(ErrorTypeNetwork, op, path, err).
        WithCode(httpErr.StatusCode)
    }
  }
  
  // Network errors
  if contains(err.Error(), "connection refused", "timeout", "no such host") {
    return New(ErrorTypeNetwork, op, path, err)
  }
  
  // Storage errors
  if contains(err.Error(), "no space", "disk full", "permission denied") {
    return New(ErrorTypeStorage, op, path, err)
  }
  
  // Default to network error for unknown errors
  return New(ErrorTypeNetwork, op, path, err)
}

// SyncItem represents a file to sync
type SyncItem struct {
  LocalPath  string
  RemotePath string
  Size       int64
  Type       string
}

// Example usage of the integrated system
func ExampleIntegratedErrorHandling() {
  ctx := context.Background()
  
  // Create sync manager
  manager := NewCloudSyncManager()
  
  // Example 1: Single file sync with retry
  fmt.Println("=== Single File Sync ===")
  err := manager.SyncFile(ctx, "/local/file.txt", "/remote/file.txt")
  if err != nil {
    fmt.Printf("Sync failed: %v\n", err)
  }
  
  // Example 2: Batch sync with error recovery
  fmt.Println("\n=== Batch Sync ===")
  files := []SyncItem{
    {LocalPath: "/local/doc1.pdf", RemotePath: "/remote/doc1.pdf", Size: 1024000, Type: "pdf"},
    {LocalPath: "/local/image.jpg", RemotePath: "/remote/image.jpg", Size: 512000, Type: "image"},
    {LocalPath: "/local/data.csv", RemotePath: "/remote/data.csv", Size: 2048000, Type: "csv"},
  }
  
  err = manager.SyncBatch(ctx, files)
  if err != nil {
    fmt.Printf("Batch sync failed: %v\n", err)
  }
  
  // Example 3: Context cancellation handling
  fmt.Println("\n=== Context Cancellation ===")
  timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
  defer cancel()
  
  err = manager.SyncFile(timeoutCtx, "/local/large.zip", "/remote/large.zip")
  if err != nil {
    if IsContextError(err) {
      fmt.Println("Sync cancelled or timed out")
    } else {
      fmt.Printf("Sync failed: %v\n", err)
    }
  }
}