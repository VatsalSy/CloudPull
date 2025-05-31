package api

import (
	"context"
	"sync"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"

	"github.com/VatsalSy/CloudPull/internal/errors"
	"github.com/VatsalSy/CloudPull/internal/logger"
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

// BatchRequest represents a single request in a batch.
type BatchRequest struct {
	Callback func(interface{}, error)
	ID       string
	FileID   string
	Type     BatchRequestType
}

// BatchRequestType defines the type of batch request.
type BatchRequestType int

const (
	BatchGetMetadata BatchRequestType = iota
	BatchGetPermissions
	BatchGetRevisions
)

// BatchResponse wraps the response from a batch request.
type BatchResponse struct {
	Request BatchRequest
	Data    interface{}
	Error   error
}

// BatchProcessor handles batch operations.
type BatchProcessor struct {
	service     *drive.Service
	rateLimiter *RateLimiter
	logger      *logger.Logger
	results     chan BatchResponse
	cancel      context.CancelFunc
	queue       []BatchRequest
	wg          sync.WaitGroup
	mu          sync.Mutex
	processing  bool
	workers     chan struct{}
	jobs        chan BatchRequest
}

// NewBatchProcessor creates a new batch processor.
func NewBatchProcessor(service *drive.Service, rateLimiter *RateLimiter, log *logger.Logger) *BatchProcessor {
	bp := &BatchProcessor{
		service:     service,
		rateLimiter: rateLimiter,
		logger:      log,
		queue:       make([]BatchRequest, 0),
		results:     make(chan BatchResponse, maxBatchSize),
		jobs:        make(chan BatchRequest, maxBatchSize),
	}
	
	// Start worker pool
	ctx, cancel := context.WithCancel(context.Background())
	bp.cancel = cancel
	bp.startWorkerPool(ctx)
	
	return bp
}

// AddRequest adds a request to the batch queue.
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

// Flush processes all pending requests.
func (bp *BatchProcessor) Flush(ctx context.Context) error {
	bp.mu.Lock()
	if len(bp.queue) == 0 {
		bp.mu.Unlock()
		return nil
	}
	bp.mu.Unlock()

	return bp.processQueue(ctx)
}

// processQueue processes all pending requests.
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
		select {
		case <-ctx.Done():
			bp.logger.Info("Context cancelled, stopping queue processing")
			return ctx.Err()
		default:
			batch := bp.dequeueBatch()
			if len(batch) == 0 {
				return nil
			}

			if err := bp.processBatch(ctx, batch); err != nil {
				bp.logger.Error(err, "Failed to process batch")
				// Continue processing other batches
			}
		}
	}
}

// dequeueBatch removes up to maxBatchSize requests from the queue.
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

// startWorkerPool starts the worker pool for processing batch requests.
func (bp *BatchProcessor) startWorkerPool(ctx context.Context) {
	for i := 0; i < maxConcurrentRequests; i++ {
		bp.wg.Add(1)
		go bp.worker(ctx)
	}
}

// worker processes batch requests from the jobs channel.
func (bp *BatchProcessor) worker(ctx context.Context) {
	defer bp.wg.Done()
	
	for {
		select {
		case <-ctx.Done():
			return
		case req, ok := <-bp.jobs:
			if !ok {
				return
			}
			
			// Create context with timeout for this request
			reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			
			// Wait for rate limit
			if err := bp.rateLimiter.Wait(reqCtx); err != nil {
				bp.handleBatchResponse(req, nil, err)
				cancel()
				continue
			}
			
			// Execute request based on type
			switch req.Type {
			case BatchGetMetadata:
				bp.executeMetadataRequest(reqCtx, req)
			case BatchGetPermissions:
				bp.executePermissionsRequest(reqCtx, req)
			case BatchGetRevisions:
				bp.executeRevisionsRequest(reqCtx, req)
			default:
				bp.logger.Warn("Unknown batch request type", "type", req.Type)
			}
			
			cancel()
		}
	}
}

// processBatch processes a batch of requests using the worker pool.
func (bp *BatchProcessor) processBatch(ctx context.Context, batch []BatchRequest) error {
	bp.logger.Debug("Processing batch", "size", len(batch))

	// Create context with timeout
	batchCtx, cancel := context.WithTimeout(ctx, batchTimeout)
	defer cancel()

	// Send requests to worker pool
	for _, req := range batch {
		select {
		case <-batchCtx.Done():
			return batchCtx.Err()
		case bp.jobs <- req:
			// Request sent to worker
		}
	}

	bp.logger.Debug("Batch dispatched to workers", "size", len(batch))
	return nil
}

// executeMetadataRequest executes a metadata request.
func (bp *BatchProcessor) executeMetadataRequest(ctx context.Context, req BatchRequest) {
	resp, err := bp.service.Files.Get(req.FileID).
		Fields("id, name, mimeType, size, md5Checksum, modifiedTime, parents").
		Context(ctx).
		Do()

	bp.handleBatchResponse(req, resp, err)
}

// executePermissionsRequest executes a permissions request.
func (bp *BatchProcessor) executePermissionsRequest(ctx context.Context, req BatchRequest) {
	resp, err := bp.service.Permissions.List(req.FileID).
		Fields("permissions(id, type, role, emailAddress)").
		Context(ctx).
		Do()

	bp.handleBatchResponse(req, resp, err)
}

// executeRevisionsRequest executes a revisions request.
func (bp *BatchProcessor) executeRevisionsRequest(ctx context.Context, req BatchRequest) {
	resp, err := bp.service.Revisions.List(req.FileID).
		Fields("revisions(id, modifiedTime, size)").
		Context(ctx).
		Do()

	bp.handleBatchResponse(req, resp, err)
}

// handleBatchResponse handles the response from a batch request.
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

// GetResults returns the results channel.
func (bp *BatchProcessor) GetResults() <-chan BatchResponse {
	return bp.results
}

// Stop gracefully stops the batch processor.
func (bp *BatchProcessor) Stop() error {
	bp.mu.Lock()
	if bp.cancel != nil {
		bp.cancel()
	}
	bp.mu.Unlock()

	// Close jobs channel to signal workers to stop
	close(bp.jobs)
	
	// Wait for all workers to complete
	bp.wg.Wait()

	// Close results channel
	close(bp.results)

	return nil
}

// Example usage for prefetching file metadata.
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
