/**
 * Worker Pool for Concurrent Downloads in CloudPull Sync Engine
 * 
 * Features:
 * - Configurable worker pool size
 * - Priority queue support
 * - Graceful shutdown and restart
 * - Worker health monitoring
 * - Task distribution and load balancing
 * 
 * Author: CloudPull Team
 * Updated: 2025-01-29
 */

package sync

import (
  "container/heap"
  "context"
  "fmt"
  "sync"
  "sync/atomic"
  "time"
  
  "github.com/VatsalSy/CloudPull/internal/api"
  "github.com/VatsalSy/CloudPull/internal/errors"
  "github.com/VatsalSy/CloudPull/internal/logger"
  "github.com/VatsalSy/CloudPull/internal/state"
)

// WorkerPool manages concurrent download workers
type WorkerPool struct {
  mu sync.RWMutex
  
  // Configuration
  workerCount      int
  maxRetries       int
  shutdownTimeout  time.Duration
  
  // Dependencies
  client          *api.DriveClient
  stateManager    *state.Manager
  progressTracker *ProgressTracker
  errorHandler    *errors.Handler
  logger          *logger.Logger
  downloadManager *DownloadManager // Reference to parent download manager
  
  // Worker management
  workers         []*Worker
  taskQueue       *PriorityQueue
  taskChan        chan *DownloadTask
  resultChan      chan *TaskResult
  
  // Control
  ctx             context.Context
  cancel          context.CancelFunc
  wg              sync.WaitGroup
  
  // Metrics
  tasksProcessed  int64
  tasksSucceeded  int64
  tasksFailed     int64
  bytesDownloaded int64
}

// Worker represents a download worker
type Worker struct {
  id              int
  pool            *WorkerPool
  tasksProcessed  int64
  bytesDownloaded int64
  lastActivity    time.Time
  isActive        atomic.Bool
}

// DownloadTask represents a file download task
type DownloadTask struct {
  File         *state.File
  Priority     int // Lower number = higher priority
  Retries      int
  LastError    error
  CreatedAt    time.Time
  StartedAt    *time.Time
  CompletedAt  *time.Time
}

// TaskResult represents the result of a download task
type TaskResult struct {
  Task         *DownloadTask
  Success      bool
  Error        error
  BytesWritten int64
  Duration     time.Duration
  WorkerID     int
}

// PriorityQueue implements a priority queue for download tasks
type PriorityQueue struct {
  mu    sync.Mutex
  items taskHeap
}

// WorkerPoolConfig contains configuration for the worker pool
type WorkerPoolConfig struct {
  WorkerCount     int
  MaxRetries      int
  ShutdownTimeout time.Duration
}

// DefaultWorkerPoolConfig returns default configuration
func DefaultWorkerPoolConfig() *WorkerPoolConfig {
  return &WorkerPoolConfig{
    WorkerCount:     3,
    MaxRetries:      3,
    ShutdownTimeout: 30 * time.Second,
  }
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(
  client *api.DriveClient,
  stateManager *state.Manager,
  progressTracker *ProgressTracker,
  errorHandler *errors.Handler,
  logger *logger.Logger,
  config *WorkerPoolConfig,
) *WorkerPool {
  if config == nil {
    config = DefaultWorkerPoolConfig()
  }
  
  ctx, cancel := context.WithCancel(context.Background())
  
  return &WorkerPool{
    workerCount:     config.WorkerCount,
    maxRetries:      config.MaxRetries,
    shutdownTimeout: config.ShutdownTimeout,
    client:          client,
    stateManager:    stateManager,
    progressTracker: progressTracker,
    errorHandler:    errorHandler,
    logger:          logger,
    taskQueue:       NewPriorityQueue(),
    taskChan:        make(chan *DownloadTask, config.WorkerCount*2),
    resultChan:      make(chan *TaskResult, config.WorkerCount*2),
    ctx:             ctx,
    cancel:          cancel,
  }
}

// SetDownloadManager sets the download manager reference
func (wp *WorkerPool) SetDownloadManager(dm *DownloadManager) {
  wp.mu.Lock()
  defer wp.mu.Unlock()
  wp.downloadManager = dm
}

// Start starts the worker pool
func (wp *WorkerPool) Start(ctx context.Context) error {
  wp.mu.Lock()
  defer wp.mu.Unlock()
  
  // Create context with cancellation
  wp.ctx, wp.cancel = context.WithCancel(ctx)
  
  // Start workers
  wp.workers = make([]*Worker, wp.workerCount)
  for i := 0; i < wp.workerCount; i++ {
    worker := &Worker{
      id:           i + 1,
      pool:         wp,
      lastActivity: time.Now(),
    }
    wp.workers[i] = worker
    wp.wg.Add(1)
    go worker.run()
  }
  
  // Start task dispatcher
  wp.wg.Add(1)
  go wp.dispatchTasks()
  
  // Start result processor
  wp.wg.Add(1)
  go wp.processResults()
  
  wp.logger.Info("Worker pool started",
    "worker_count", wp.workerCount,
    "max_retries", wp.maxRetries,
  )
  
  return nil
}

// Stop stops the worker pool gracefully
func (wp *WorkerPool) Stop() error {
  wp.logger.Info("Stopping worker pool...")
  
  // Signal shutdown
  wp.cancel()
  
  // Wait for workers with timeout
  done := make(chan struct{})
  go func() {
    wp.wg.Wait()
    close(done)
  }()
  
  select {
  case <-done:
    wp.logger.Info("Worker pool stopped gracefully")
    return nil
  case <-time.After(wp.shutdownTimeout):
    wp.logger.Warn("Worker pool shutdown timeout exceeded")
    return fmt.Errorf("shutdown timeout exceeded")
  }
}

// SubmitTask submits a download task to the pool
func (wp *WorkerPool) SubmitTask(file *state.File, priority int) error {
  select {
  case <-wp.ctx.Done():
    return wp.ctx.Err()
  default:
  }
  
  task := &DownloadTask{
    File:      file,
    Priority:  priority,
    CreatedAt: time.Now(),
  }
  
  // Add to priority queue
  wp.taskQueue.Push(task)
  
  wp.logger.Debug("Task submitted to queue",
    "file_id", file.ID,
    "file_name", file.Name,
    "priority", priority,
    "queue_size", wp.taskQueue.Len(),
  )
  
  return nil
}

// GetStats returns worker pool statistics
func (wp *WorkerPool) GetStats() *WorkerPoolStats {
  wp.mu.RLock()
  defer wp.mu.RUnlock()
  
  activeWorkers := 0
  for _, worker := range wp.workers {
    if worker.isActive.Load() {
      activeWorkers++
    }
  }
  
  return &WorkerPoolStats{
    WorkerCount:     wp.workerCount,
    ActiveWorkers:   activeWorkers,
    QueuedTasks:     wp.taskQueue.Len(),
    TasksProcessed:  atomic.LoadInt64(&wp.tasksProcessed),
    TasksSucceeded:  atomic.LoadInt64(&wp.tasksSucceeded),
    TasksFailed:     atomic.LoadInt64(&wp.tasksFailed),
    BytesDownloaded: atomic.LoadInt64(&wp.bytesDownloaded),
  }
}

// dispatchTasks dispatches tasks from the priority queue to workers
func (wp *WorkerPool) dispatchTasks() {
  defer wp.wg.Done()
  
  ticker := time.NewTicker(100 * time.Millisecond)
  defer ticker.Stop()
  
  for {
    select {
    case <-wp.ctx.Done():
      return
      
    case <-ticker.C:
      // Process all available tasks in the queue
      for {
        // Check if there are tasks in the queue
        task := wp.taskQueue.Pop()
        if task == nil {
          break
        }
        
        // Send task to workers
        select {
        case wp.taskChan <- task:
          // Task dispatched
          wp.logger.Debug("Task dispatched to worker",
            "file_id", task.File.ID,
            "file_name", task.File.Name,
            "priority", task.Priority,
          )
        case <-wp.ctx.Done():
          // Put task back in queue
          wp.taskQueue.Push(task)
          return
        default:
          // Channel is full, put task back and wait
          wp.taskQueue.Push(task)
          goto waitForNextTick
        }
      }
      waitForNextTick:
    }
  }
}

// processResults processes task results
func (wp *WorkerPool) processResults() {
  defer wp.wg.Done()
  
  for {
    select {
    case <-wp.ctx.Done():
      return
      
    case result := <-wp.resultChan:
      atomic.AddInt64(&wp.tasksProcessed, 1)
      
      if result.Success {
        atomic.AddInt64(&wp.tasksSucceeded, 1)
        atomic.AddInt64(&wp.bytesDownloaded, result.BytesWritten)
        
        // Update file status in database
        result.Task.File.Status = state.FileStatusCompleted
        result.Task.File.BytesDownloaded = result.Task.File.Size
        if err := wp.stateManager.UpdateFileStatus(wp.ctx, result.Task.File); err != nil {
          wp.logger.Error(err, "Failed to update file status",
            "file_id", result.Task.File.ID,
            "status", result.Task.File.Status,
          )
        }
        
        // Notify progress tracker
        wp.progressTracker.FileCompleted(result.Task.File.ID)
        
      } else {
        atomic.AddInt64(&wp.tasksFailed, 1)
        
        // Handle retry logic
        if result.Task.Retries < wp.maxRetries {
          result.Task.Retries++
          result.Task.LastError = result.Error
          
          // Calculate retry priority (lower priority for retries)
          result.Task.Priority += 1000 * result.Task.Retries
          
          // Re-queue the task
          wp.taskQueue.Push(result.Task)
          
          wp.logger.Warn("Retrying download task",
            "file_id", result.Task.File.ID,
            "attempt", result.Task.Retries,
            "error", result.Error,
          )
        } else {
          // Max retries exceeded
          result.Task.File.Status = state.FileStatusFailed
          result.Task.File.ErrorMessage.Valid = true
          result.Task.File.ErrorMessage.String = result.Error.Error()
          
          if err := wp.stateManager.UpdateFileStatus(wp.ctx, result.Task.File); err != nil {
            wp.logger.Error(err, "Failed to update file status",
              "file_id", result.Task.File.ID,
              "status", result.Task.File.Status,
            )
          }
          
          // Notify progress tracker
          wp.progressTracker.FileFailed(result.Task.File.ID, result.Error)
          
          wp.logger.Error(result.Error, "Download task failed after max retries",
            "file_id", result.Task.File.ID,
            "attempts", result.Task.Retries,
          )
        }
      }
    }
  }
}

// Worker methods

// run is the main worker loop
func (w *Worker) run() {
  defer w.pool.wg.Done()
  
  w.pool.logger.Debug("Worker started", "worker_id", w.id)
  
  for {
    select {
    case <-w.pool.ctx.Done():
      w.pool.logger.Debug("Worker stopping", "worker_id", w.id)
      return
      
    case task := <-w.pool.taskChan:
      w.processTask(task)
    }
  }
}

// processTask processes a single download task
func (w *Worker) processTask(task *DownloadTask) {
  w.isActive.Store(true)
  w.lastActivity = time.Now()
  defer w.isActive.Store(false)
  
  startTime := time.Now()
  task.StartedAt = &startTime
  
  w.pool.logger.Info("Worker processing task",
    "worker_id", w.id,
    "file_id", task.File.ID,
    "file_name", task.File.Name,
    "file_size", task.File.Size,
    "priority", task.Priority,
  )
  
  // Notify progress tracker
  w.pool.progressTracker.FileStarted(
    task.File.ID,
    task.File.Name,
    task.File.Path,
    task.File.Size,
  )
  
  // Update file status
  task.File.Status = state.FileStatusDownloading
  task.File.DownloadAttempts++
  if err := w.pool.stateManager.UpdateFileStatus(w.pool.ctx, task.File); err != nil {
    w.pool.logger.Error(err, "Failed to update file status",
      "file_id", task.File.ID,
      "status", task.File.Status,
    )
  }
  
  // Download the file
  var bytesWritten int64
  err := w.downloadFile(task, &bytesWritten)
  
  completedTime := time.Now()
  task.CompletedAt = &completedTime
  duration := completedTime.Sub(startTime)
  
  if err != nil {
    w.pool.logger.Error(err, "Download failed",
      "worker_id", w.id,
      "file_id", task.File.ID,
      "file_name", task.File.Name,
      "duration", duration,
    )
  } else {
    w.pool.logger.Info("Download completed",
      "worker_id", w.id,
      "file_id", task.File.ID,
      "file_name", task.File.Name,
      "bytes_written", bytesWritten,
      "duration", duration,
    )
  }
  
  // Send result
  result := &TaskResult{
    Task:         task,
    Success:      err == nil,
    Error:        err,
    BytesWritten: bytesWritten,
    Duration:     duration,
    WorkerID:     w.id,
  }
  
  select {
  case w.pool.resultChan <- result:
    // Result sent
  case <-w.pool.ctx.Done():
    // Shutting down
  }
  
  atomic.AddInt64(&w.tasksProcessed, 1)
  if err == nil {
    atomic.AddInt64(&w.bytesDownloaded, bytesWritten)
  }
}

// downloadFile performs the actual file download
func (w *Worker) downloadFile(task *DownloadTask, bytesWritten *int64) error {
  // Use download manager if available (for advanced features like resume, checksum, etc)
  if w.pool.downloadManager != nil {
    err := w.pool.downloadManager.DownloadFile(w.pool.ctx, task.File)
    if err != nil {
      return errors.Wrap(err, "download failed")
    }
    *bytesWritten = task.File.Size
    return nil
  }
  
  // Fallback to direct client download
  // Progress callback
  progressFn := func(downloaded, total int64) {
    *bytesWritten = downloaded
    w.pool.progressTracker.FileProgress(task.File.ID, downloaded)
  }
  
  // Download the file
  err := w.pool.client.DownloadFile(
    w.pool.ctx,
    task.File.DriveID,
    task.File.Path,
    progressFn,
  )
  
  if err != nil {
    return errors.Wrap(err, "download failed")
  }
  
  return nil
}

// Priority queue implementation

// NewPriorityQueue creates a new priority queue
func NewPriorityQueue() *PriorityQueue {
  pq := &PriorityQueue{
    items: make(taskHeap, 0),
  }
  heap.Init(&pq.items)
  return pq
}

// Push adds a task to the queue
func (pq *PriorityQueue) Push(task *DownloadTask) {
  pq.mu.Lock()
  defer pq.mu.Unlock()
  
  heap.Push(&pq.items, task)
}

// Pop removes and returns the highest priority task
func (pq *PriorityQueue) Pop() *DownloadTask {
  pq.mu.Lock()
  defer pq.mu.Unlock()
  
  if len(pq.items) == 0 {
    return nil
  }
  
  return heap.Pop(&pq.items).(*DownloadTask)
}

// Len returns the number of tasks in the queue
func (pq *PriorityQueue) Len() int {
  pq.mu.Lock()
  defer pq.mu.Unlock()
  
  return len(pq.items)
}

// Heap interface implementation for priority queue
type taskHeap []*DownloadTask

func (h taskHeap) Len() int           { return len(h) }
func (h taskHeap) Less(i, j int) bool { return h[i].Priority < h[j].Priority }
func (h taskHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *taskHeap) Push(x interface{}) {
  *h = append(*h, x.(*DownloadTask))
}

func (h *taskHeap) Pop() interface{} {
  old := *h
  n := len(old)
  x := old[n-1]
  *h = old[0 : n-1]
  return x
}

// WorkerPoolStats contains worker pool statistics
type WorkerPoolStats struct {
  WorkerCount     int
  ActiveWorkers   int
  QueuedTasks     int
  TasksProcessed  int64
  TasksSucceeded  int64
  TasksFailed     int64
  BytesDownloaded int64
}