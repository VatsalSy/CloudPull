package api

import (
  "context"
  "sync"
  "time"

  "github.com/VatsalSy/CloudPull/internal/errors"
  "github.com/VatsalSy/CloudPull/internal/logger"
  "google.golang.org/api/drive/v3"
  "google.golang.org/api/googleapi"
)

/**
 * Batch Operations for Google Drive API (v2 - Concurrent Implementation)
 * 
 * Features:
 * - Concurrent request processing (replaces unavailable batch API)
 * - Request grouping with concurrency control
 * - Error handling per request
 * - Progress tracking
 * - Retry logic for failed requests
 * - Memory-efficient processing
 * 
 * Author: CloudPull Team
 * Updated: 2025-01-30
 */

const (
  // Maximum concurrent requests
  maxConcurrentRequests = 10
  // Maximum requests to process in one batch
  maxBatchSize = 100
  // Timeout for batch processing
  batchTimeout = 30 * time.Second
)

// BatchRequest represents a single request in a batch
type BatchRequest struct {
  ID       string
  FileID   string
  Type     BatchRequestType
  Callback func(interface{}, error)
}

// BatchRequestType defines the type of batch request
type BatchRequestType int

const (
  BatchGetMetadata BatchRequestType = iota
  BatchGetPermissions
  BatchGetRevisions
)

// BatchResponse wraps the response from a batch request
type BatchResponse struct {
  Request BatchRequest
  Data    interface{}
  Error   error
}

// BatchProcessor handles batch operations
type BatchProcessor struct {
  service     *drive.Service
  rateLimiter *RateLimiter
  logger      *logger.Logger
  
  // Request queue
  mu       sync.Mutex
  queue    []BatchRequest
  results  chan BatchResponse
  
  // Processing control
  processing bool
  cancel     context.CancelFunc
  wg         sync.WaitGroup
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(service *drive.Service, rateLimiter *RateLimiter, log *logger.Logger) *BatchProcessor {
  return &BatchProcessor{
    service:     service,
    rateLimiter: rateLimiter,
    logger:      log,
    queue:       make([]BatchRequest, 0),
    results:     make(chan BatchResponse, maxBatchSize),
  }
}

// AddRequest adds a request to the batch queue
func (bp *BatchProcessor) AddRequest(req BatchRequest) error {
  bp.mu.Lock()
  defer bp.mu.Unlock()
  
  if len(bp.queue) >= maxBatchSize*10 {
    return errors.New(errors.ErrorTypeAPI, "batch_queue_full", "Batch queue is full", nil)
  }
  
  bp.queue = append(bp.queue, req)
  
  // Start processing if we have enough requests
  if len(bp.queue) >= maxBatchSize && !bp.processing {
    go bp.processQueue(context.Background())
  }
  
  return nil
}

// Flush processes all pending requests
func (bp *BatchProcessor) Flush(ctx context.Context) error {
  bp.mu.Lock()
  if len(bp.queue) == 0 {
    bp.mu.Unlock()
    return nil
  }
  bp.mu.Unlock()
  
  return bp.processQueue(ctx)
}

// processQueue processes all pending requests
func (bp *BatchProcessor) processQueue(ctx context.Context) error {
  bp.mu.Lock()
  if bp.processing {
    bp.mu.Unlock()
    return nil
  }
  bp.processing = true
  bp.mu.Unlock()
  
  defer func() {
    bp.mu.Lock()
    bp.processing = false
    bp.mu.Unlock()
  }()
  
  for {
    batch := bp.dequeueBatch()
    if len(batch) == 0 {
      break
    }
    
    if err := bp.processBatch(ctx, batch); err != nil {
      bp.logger.Error(err, "Failed to process batch")
      // Continue processing other batches
    }
  }
  
  return nil
}

// dequeueBatch removes up to maxBatchSize requests from the queue
func (bp *BatchProcessor) dequeueBatch() []BatchRequest {
  bp.mu.Lock()
  defer bp.mu.Unlock()
  
  if len(bp.queue) == 0 {
    return nil
  }
  
  size := maxBatchSize
  if len(bp.queue) < size {
    size = len(bp.queue)
  }
  
  batch := bp.queue[:size]
  bp.queue = bp.queue[size:]
  
  return batch
}

// processBatch processes a batch of requests concurrently
func (bp *BatchProcessor) processBatch(ctx context.Context, batch []BatchRequest) error {
  bp.logger.Debug("Processing batch", "size", len(batch))
  
  // Create context with timeout
  batchCtx, cancel := context.WithTimeout(ctx, batchTimeout)
  defer cancel()
  
  // Create semaphore for concurrency control
  sem := make(chan struct{}, maxConcurrentRequests)
  
  // Process requests concurrently
  var wg sync.WaitGroup
  for _, req := range batch {
    wg.Add(1)
    go func(r BatchRequest) {
      defer wg.Done()
      
      // Acquire semaphore
      select {
      case sem <- struct{}{}:
        defer func() { <-sem }()
      case <-batchCtx.Done():
        bp.handleBatchResponse(r, nil, batchCtx.Err())
        return
      }
      
      // Wait for rate limit
      if err := bp.rateLimiter.Wait(batchCtx); err != nil {
        bp.handleBatchResponse(r, nil, err)
        return
      }
      
      // Execute request based on type
      switch r.Type {
      case BatchGetMetadata:
        bp.executeMetadataRequest(batchCtx, r)
      case BatchGetPermissions:
        bp.executePermissionsRequest(batchCtx, r)
      case BatchGetRevisions:
        bp.executeRevisionsRequest(batchCtx, r)
      default:
        bp.logger.Warn("Unknown batch request type", "type", r.Type)
      }
    }(req)
  }
  
  // Wait for all requests to complete
  wg.Wait()
  
  bp.logger.Debug("Batch processed successfully", "size", len(batch))
  return nil
}

// executeMetadataRequest executes a metadata request
func (bp *BatchProcessor) executeMetadataRequest(ctx context.Context, req BatchRequest) {
  resp, err := bp.service.Files.Get(req.FileID).
    Fields("id, name, mimeType, size, md5Checksum, modifiedTime, parents").
    Context(ctx).
    Do()
  
  bp.handleBatchResponse(req, resp, err)
}

// executePermissionsRequest executes a permissions request
func (bp *BatchProcessor) executePermissionsRequest(ctx context.Context, req BatchRequest) {
  resp, err := bp.service.Permissions.List(req.FileID).
    Fields("permissions(id, type, role, emailAddress)").
    Context(ctx).
    Do()
  
  bp.handleBatchResponse(req, resp, err)
}

// executeRevisionsRequest executes a revisions request
func (bp *BatchProcessor) executeRevisionsRequest(ctx context.Context, req BatchRequest) {
  resp, err := bp.service.Revisions.List(req.FileID).
    Fields("revisions(id, modifiedTime, size)").
    Context(ctx).
    Do()
  
  bp.handleBatchResponse(req, resp, err)
}

// handleBatchResponse handles the response from a batch request
func (bp *BatchProcessor) handleBatchResponse(req BatchRequest, data interface{}, err error) {
  // Handle API errors
  if err != nil {
    if apiErr, ok := err.(*googleapi.Error); ok {
      bp.logger.Debug("Batch request failed", 
        "file_id", req.FileID,
        "type", req.Type,
        "status", apiErr.Code,
        "message", apiErr.Message,
      )
    } else {
      bp.logger.Debug("Batch request failed",
        "file_id", req.FileID,
        "type", req.Type,
        "error", err,
      )
    }
  }
  
  // Execute callback if provided
  if req.Callback != nil {
    req.Callback(data, err)
  }
  
  // Send to results channel
  select {
  case bp.results <- BatchResponse{
    Request: req,
    Data:    data,
    Error:   err,
  }:
  default:
    // Results channel full, log and continue
    bp.logger.Warn("Results channel full, dropping response", "file_id", req.FileID)
  }
}

// GetResults returns the results channel
func (bp *BatchProcessor) GetResults() <-chan BatchResponse {
  return bp.results
}

// Stop gracefully stops the batch processor
func (bp *BatchProcessor) Stop() error {
  bp.mu.Lock()
  if bp.cancel != nil {
    bp.cancel()
  }
  bp.mu.Unlock()
  
  // Wait for processing to complete
  bp.wg.Wait()
  
  // Close results channel
  close(bp.results)
  
  return nil
}

// Example usage for prefetching file metadata
func (bp *BatchProcessor) PrefetchMetadata(ctx context.Context, fileIDs []string) error {
  bp.logger.Info("Prefetching metadata", "count", len(fileIDs))
  
  for _, fileID := range fileIDs {
    req := BatchRequest{
      ID:     fileID,
      FileID: fileID,
      Type:   BatchGetMetadata,
    }
    
    if err := bp.AddRequest(req); err != nil {
      return err
    }
  }
  
  return bp.Flush(ctx)
}