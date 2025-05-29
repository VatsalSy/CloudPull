/**
 * Download Manager for CloudPull Sync Engine
 * 
 * Features:
 * - Resume partial downloads using byte ranges
 * - Checksum verification after download
 * - Atomic file operations (download to temp, then move)
 * - Google Docs export handling
 * - Bandwidth throttling support
 * - Priority-based download scheduling
 * 
 * Author: CloudPull Team
 * Updated: 2025-01-29
 */

package sync

import (
  "context"
  "crypto/md5"
  "encoding/hex"
  "fmt"
  "io"
  "os"
  "path/filepath"
  "strings"
  "sync"
  "time"
  
  "github.com/cloudpull/cloudpull/internal/api"
  "github.com/cloudpull/cloudpull/internal/errors"
  "github.com/cloudpull/cloudpull/internal/logger"
  "github.com/cloudpull/cloudpull/internal/state"
)

// DownloadManager manages file downloads with advanced features
type DownloadManager struct {
  mu sync.RWMutex
  
  // Configuration
  tempDir          string
  chunkSize        int64
  maxConcurrent    int
  verifyChecksums  bool
  
  // Dependencies
  client          *api.DriveClient
  stateManager    *state.Manager
  progressTracker *ProgressTracker
  errorHandler    *errors.Handler
  logger          logger.Logger
  
  // Worker pool
  workerPool      *WorkerPool
  
  // State
  ctx             context.Context
  cancel          context.CancelFunc
  
  // Metrics
  activeDownloads sync.Map // map[string]*DownloadInfo
  downloadStats   *DownloadStats
}

// DownloadInfo tracks active download information
type DownloadInfo struct {
  FileID          string
  FileName        string
  TempPath        string
  FinalPath       string
  Size            int64
  BytesDownloaded int64
  StartTime       time.Time
  Checksum        string
  IsGoogleDoc     bool
  ExportFormat    string
}

// DownloadStats tracks download statistics
type DownloadStats struct {
  mu              sync.RWMutex
  TotalDownloads  int64
  ActiveDownloads int64
  CompletedDownloads int64
  FailedDownloads int64
  BytesDownloaded int64
  TotalDuration   time.Duration
}

// DownloadManagerConfig contains configuration for the download manager
type DownloadManagerConfig struct {
  TempDir         string
  ChunkSize       int64
  MaxConcurrent   int
  VerifyChecksums bool
}

// DefaultDownloadManagerConfig returns default configuration
func DefaultDownloadManagerConfig() *DownloadManagerConfig {
  return &DownloadManagerConfig{
    TempDir:         os.TempDir(),
    ChunkSize:       10 * 1024 * 1024, // 10MB
    MaxConcurrent:   3,
    VerifyChecksums: true,
  }
}

// NewDownloadManager creates a new download manager
func NewDownloadManager(
  client *api.DriveClient,
  stateManager *state.Manager,
  progressTracker *ProgressTracker,
  errorHandler *errors.Handler,
  logger logger.Logger,
  config *DownloadManagerConfig,
) (*DownloadManager, error) {
  if config == nil {
    config = DefaultDownloadManagerConfig()
  }
  
  // Create temp directory
  tempDir := filepath.Join(config.TempDir, "cloudpull-downloads")
  if err := os.MkdirAll(tempDir, 0755); err != nil {
    return nil, errors.Wrap(err, "failed to create temp directory")
  }
  
  // Create worker pool
  workerPoolConfig := &WorkerPoolConfig{
    WorkerCount:     config.MaxConcurrent,
    MaxRetries:      3,
    ShutdownTimeout: 30 * time.Second,
  }
  
  workerPool := NewWorkerPool(
    client,
    stateManager,
    progressTracker,
    errorHandler,
    logger,
    workerPoolConfig,
  )
  
  dm := &DownloadManager{
    tempDir:         tempDir,
    chunkSize:       config.ChunkSize,
    maxConcurrent:   config.MaxConcurrent,
    verifyChecksums: config.VerifyChecksums,
    client:          client,
    stateManager:    stateManager,
    progressTracker: progressTracker,
    errorHandler:    errorHandler,
    logger:          logger,
    workerPool:      workerPool,
    downloadStats:   &DownloadStats{},
  }
  
  return dm, nil
}

// Start starts the download manager
func (dm *DownloadManager) Start(ctx context.Context) error {
  dm.ctx, dm.cancel = context.WithCancel(ctx)
  
  // Start worker pool
  if err := dm.workerPool.Start(dm.ctx); err != nil {
    return errors.Wrap(err, "failed to start worker pool")
  }
  
  dm.logger.Info("Download manager started",
    "temp_dir", dm.tempDir,
    "chunk_size", dm.chunkSize,
    "max_concurrent", dm.maxConcurrent,
  )
  
  return nil
}

// Stop stops the download manager
func (dm *DownloadManager) Stop() error {
  dm.logger.Info("Stopping download manager...")
  
  if dm.cancel != nil {
    dm.cancel()
  }
  
  // Stop worker pool
  if err := dm.workerPool.Stop(); err != nil {
    dm.logger.Error(err, "Failed to stop worker pool")
  }
  
  // Clean up temp files
  dm.cleanupTempFiles()
  
  return nil
}

// ScheduleDownload schedules a file for download
func (dm *DownloadManager) ScheduleDownload(file *state.File, priority int) error {
  // Check if already downloading
  if _, exists := dm.activeDownloads.Load(file.ID); exists {
    return errors.Errorf("file %s is already being downloaded", file.ID)
  }
  
  // Submit to worker pool
  return dm.workerPool.SubmitTask(file, priority)
}

// ScheduleBatch schedules a batch of files for download
func (dm *DownloadManager) ScheduleBatch(files []*state.File) error {
  // Sort by size (smallest first) for better throughput
  priorityMap := dm.calculatePriorities(files)
  
  for _, file := range files {
    priority := priorityMap[file.ID]
    if err := dm.ScheduleDownload(file, priority); err != nil {
      dm.logger.Error(err, "Failed to schedule file",
        "file_id", file.ID,
        "file_name", file.Name,
      )
    }
  }
  
  return nil
}

// DownloadFile downloads a single file with resume support
func (dm *DownloadManager) DownloadFile(ctx context.Context, file *state.File) error {
  // Create download info
  downloadInfo := &DownloadInfo{
    FileID:       file.ID,
    FileName:     file.Name,
    Size:         file.Size,
    StartTime:    time.Now(),
    IsGoogleDoc:  file.IsGoogleDoc,
  }
  
  if file.IsGoogleDoc && file.ExportMimeType.Valid {
    downloadInfo.ExportFormat = file.ExportMimeType.String
  }
  
  // Generate paths
  downloadInfo.TempPath = dm.getTempPath(file)
  downloadInfo.FinalPath = file.Path
  
  // Store in active downloads
  dm.activeDownloads.Store(file.ID, downloadInfo)
  defer dm.activeDownloads.Delete(file.ID)
  
  // Update stats
  dm.downloadStats.mu.Lock()
  dm.downloadStats.TotalDownloads++
  dm.downloadStats.ActiveDownloads++
  dm.downloadStats.mu.Unlock()
  
  defer func() {
    dm.downloadStats.mu.Lock()
    dm.downloadStats.ActiveDownloads--
    dm.downloadStats.mu.Unlock()
  }()
  
  // Perform download
  var err error
  if file.IsGoogleDoc {
    err = dm.downloadGoogleDoc(ctx, file, downloadInfo)
  } else {
    err = dm.downloadRegularFile(ctx, file, downloadInfo)
  }
  
  if err != nil {
    dm.downloadStats.mu.Lock()
    dm.downloadStats.FailedDownloads++
    dm.downloadStats.mu.Unlock()
    return err
  }
  
  // Verify checksum if enabled
  if dm.verifyChecksums && file.MD5Checksum.Valid && file.MD5Checksum.String != "" {
    if err := dm.verifyChecksum(downloadInfo.TempPath, file.MD5Checksum.String); err != nil {
      os.Remove(downloadInfo.TempPath)
      return errors.Wrap(err, "checksum verification failed")
    }
  }
  
  // Move to final destination
  if err := dm.moveToFinal(downloadInfo.TempPath, downloadInfo.FinalPath); err != nil {
    os.Remove(downloadInfo.TempPath)
    return errors.Wrap(err, "failed to move file to final destination")
  }
  
  // Update stats
  dm.downloadStats.mu.Lock()
  dm.downloadStats.CompletedDownloads++
  dm.downloadStats.BytesDownloaded += file.Size
  dm.downloadStats.TotalDuration += time.Since(downloadInfo.StartTime)
  dm.downloadStats.mu.Unlock()
  
  return nil
}

// downloadRegularFile downloads a regular (non-Google Docs) file
func (dm *DownloadManager) downloadRegularFile(ctx context.Context, file *state.File, info *DownloadInfo) error {
  // Check if partial download exists
  startOffset := int64(0)
  if stat, err := os.Stat(info.TempPath); err == nil {
    startOffset = stat.Size()
    info.BytesDownloaded = startOffset
    
    // Check if already complete
    if startOffset >= file.Size {
      dm.logger.Info("File already downloaded",
        "file", file.Name,
        "size", file.Size,
      )
      return nil
    }
    
    dm.logger.Info("Resuming partial download",
      "file", file.Name,
      "offset", startOffset,
      "total", file.Size,
    )
  }
  
  // Progress callback with bandwidth limiting
  progressFn := func(downloaded, total int64) {
    // Check bandwidth limit
    delta := downloaded - info.BytesDownloaded
    if delta > 0 {
      if err := dm.progressTracker.CheckBandwidthLimit(ctx, delta); err != nil {
        dm.logger.Debug("Bandwidth limit check failed", "error", err)
      }
    }
    
    info.BytesDownloaded = startOffset + downloaded
    dm.progressTracker.FileProgress(file.ID, info.BytesDownloaded)
  }
  
  // Download file
  err := dm.downloadWithResume(ctx, file.DriveID, info.TempPath, startOffset, file.Size, progressFn)
  if err != nil {
    return errors.Wrap(err, "download failed")
  }
  
  return nil
}

// downloadGoogleDoc exports and downloads a Google Docs file
func (dm *DownloadManager) downloadGoogleDoc(ctx context.Context, file *state.File, info *DownloadInfo) error {
  // Add appropriate extension
  if !strings.Contains(info.FinalPath, ".") {
    ext := dm.getExportExtension(info.ExportFormat)
    info.FinalPath += ext
    info.TempPath += ext
  }
  
  // Progress callback
  progressFn := func(downloaded, total int64) {
    info.BytesDownloaded = downloaded
    dm.progressTracker.FileProgress(file.ID, downloaded)
  }
  
  // Export file
  err := dm.client.ExportFile(ctx, file.DriveID, info.ExportFormat, info.TempPath, progressFn)
  if err != nil {
    return errors.Wrap(err, "export failed")
  }
  
  // Get actual file size
  if stat, err := os.Stat(info.TempPath); err == nil {
    info.Size = stat.Size()
  }
  
  return nil
}

// downloadWithResume performs resumable download
func (dm *DownloadManager) downloadWithResume(
  ctx context.Context,
  fileID string,
  destPath string,
  startOffset int64,
  totalSize int64,
  progressFn func(downloaded, total int64),
) error {
  // Ensure directory exists
  if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
    return errors.Wrap(err, "failed to create directory")
  }
  
  // Open file for writing
  file, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY, 0644)
  if err != nil {
    return errors.Wrap(err, "failed to open file")
  }
  defer file.Close()
  
  // Seek to resume position
  if startOffset > 0 {
    if _, err := file.Seek(startOffset, 0); err != nil {
      return errors.Wrap(err, "failed to seek in file")
    }
  }
  
  // Custom download with manual retry and resume
  currentOffset := startOffset
  retries := 0
  maxRetries := 3
  
  for currentOffset < totalSize && retries < maxRetries {
    // Calculate chunk boundaries
    endOffset := currentOffset + dm.chunkSize - 1
    if endOffset >= totalSize {
      endOffset = totalSize - 1
    }
    
    // Download chunk
    resp, err := dm.client.GetFileContent(ctx, fileID, currentOffset, endOffset)
    if err != nil {
      retries++
      dm.logger.Warn("Chunk download failed, retrying",
        "file_id", fileID,
        "offset", currentOffset,
        "retry", retries,
        "error", err,
      )
      
      // Wait before retry
      select {
      case <-time.After(time.Duration(retries) * time.Second):
        continue
      case <-ctx.Done():
        return ctx.Err()
      }
    }
    
    // Write chunk
    written, err := io.Copy(file, resp.Body)
    resp.Body.Close()
    
    if err != nil {
      return errors.Wrap(err, "failed to write chunk")
    }
    
    currentOffset += written
    retries = 0 // Reset retries on success
    
    // Report progress
    if progressFn != nil {
      progressFn(currentOffset-startOffset, totalSize-startOffset)
    }
  }
  
  if currentOffset < totalSize {
    return errors.Errorf("download incomplete: %d/%d bytes", currentOffset, totalSize)
  }
  
  return nil
}

// verifyChecksum verifies file checksum
func (dm *DownloadManager) verifyChecksum(filePath string, expectedMD5 string) error {
  file, err := os.Open(filePath)
  if err != nil {
    return errors.Wrap(err, "failed to open file")
  }
  defer file.Close()
  
  hash := md5.New()
  if _, err := io.Copy(hash, file); err != nil {
    return errors.Wrap(err, "failed to calculate checksum")
  }
  
  actualMD5 := hex.EncodeToString(hash.Sum(nil))
  if actualMD5 != expectedMD5 {
    return errors.Errorf("checksum mismatch: expected %s, got %s", expectedMD5, actualMD5)
  }
  
  dm.logger.Debug("Checksum verified",
    "file", filePath,
    "md5", actualMD5,
  )
  
  return nil
}

// moveToFinal moves file from temp to final location atomically
func (dm *DownloadManager) moveToFinal(tempPath, finalPath string) error {
  // Ensure destination directory exists
  if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
    return errors.Wrap(err, "failed to create destination directory")
  }
  
  // Try atomic rename first
  if err := os.Rename(tempPath, finalPath); err == nil {
    return nil
  }
  
  // Fall back to copy and delete (for cross-device moves)
  src, err := os.Open(tempPath)
  if err != nil {
    return errors.Wrap(err, "failed to open source file")
  }
  defer src.Close()
  
  dst, err := os.Create(finalPath)
  if err != nil {
    return errors.Wrap(err, "failed to create destination file")
  }
  defer dst.Close()
  
  if _, err := io.Copy(dst, src); err != nil {
    os.Remove(finalPath)
    return errors.Wrap(err, "failed to copy file")
  }
  
  // Remove temp file
  os.Remove(tempPath)
  
  return nil
}

// calculatePriorities calculates download priorities based on file size
func (dm *DownloadManager) calculatePriorities(files []*state.File) map[string]int {
  priorities := make(map[string]int)
  
  // Sort by size (smallest first gets higher priority = lower number)
  for i, file := range files {
    if file.Size < 1024*1024 { // < 1MB
      priorities[file.ID] = i
    } else if file.Size < 10*1024*1024 { // < 10MB
      priorities[file.ID] = i + 1000
    } else if file.Size < 100*1024*1024 { // < 100MB
      priorities[file.ID] = i + 2000
    } else {
      priorities[file.ID] = i + 3000
    }
  }
  
  return priorities
}

// getTempPath generates a temporary file path
func (dm *DownloadManager) getTempPath(file *state.File) string {
  // Use file ID to ensure uniqueness
  filename := fmt.Sprintf("%s_%s", file.ID, file.Name)
  return filepath.Join(dm.tempDir, filename)
}

// getExportExtension returns the file extension for an export format
func (dm *DownloadManager) getExportExtension(mimeType string) string {
  extensions := map[string]string{
    "application/vnd.openxmlformats-officedocument.wordprocessingml.document":    ".docx",
    "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         ".xlsx",
    "application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
    "application/pdf": ".pdf",
    "text/plain":      ".txt",
    "text/html":       ".html",
    "text/csv":        ".csv",
  }
  
  if ext, ok := extensions[mimeType]; ok {
    return ext
  }
  return ""
}

// cleanupTempFiles removes all temporary files
func (dm *DownloadManager) cleanupTempFiles() {
  dm.activeDownloads.Range(func(key, value interface{}) bool {
    if info, ok := value.(*DownloadInfo); ok {
      if _, err := os.Stat(info.TempPath); err == nil {
        dm.logger.Debug("Removing temp file", "path", info.TempPath)
        os.Remove(info.TempPath)
      }
    }
    return true
  })
}

// GetStats returns download manager statistics
func (dm *DownloadManager) GetStats() *DownloadManagerStats {
  dm.downloadStats.mu.RLock()
  defer dm.downloadStats.mu.RUnlock()
  
  avgDuration := time.Duration(0)
  if dm.downloadStats.CompletedDownloads > 0 {
    avgDuration = dm.downloadStats.TotalDuration / time.Duration(dm.downloadStats.CompletedDownloads)
  }
  
  avgSpeed := int64(0)
  if dm.downloadStats.TotalDuration > 0 {
    avgSpeed = int64(float64(dm.downloadStats.BytesDownloaded) / dm.downloadStats.TotalDuration.Seconds())
  }
  
  return &DownloadManagerStats{
    TotalDownloads:     dm.downloadStats.TotalDownloads,
    ActiveDownloads:    dm.downloadStats.ActiveDownloads,
    CompletedDownloads: dm.downloadStats.CompletedDownloads,
    FailedDownloads:    dm.downloadStats.FailedDownloads,
    BytesDownloaded:    dm.downloadStats.BytesDownloaded,
    AverageSpeed:       avgSpeed,
    AverageDuration:    avgDuration,
    WorkerPoolStats:    dm.workerPool.GetStats(),
  }
}

// DownloadManagerStats contains download manager statistics
type DownloadManagerStats struct {
  TotalDownloads     int64
  ActiveDownloads    int64
  CompletedDownloads int64
  FailedDownloads    int64
  BytesDownloaded    int64
  AverageSpeed       int64 // bytes per second
  AverageDuration    time.Duration
  WorkerPoolStats    *WorkerPoolStats
}