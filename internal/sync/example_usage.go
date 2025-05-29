/**
 * Example Usage of CloudPull Sync Engine
 * 
 * This file demonstrates how to use the sync engine components
 * for implementing file synchronization from Google Drive.
 * 
 * Author: CloudPull Team
 * Updated: 2025-01-29
 */

package sync

import (
  "context"
  "fmt"
  "log"
  "time"
  
  "github.com/VatsalSy/CloudPull/internal/api"
  "github.com/VatsalSy/CloudPull/internal/errors"
  "github.com/VatsalSy/CloudPull/internal/logger"
  "github.com/VatsalSy/CloudPull/internal/state"
  "google.golang.org/api/drive/v3"
)

// ExampleBasicSync demonstrates basic sync usage
func ExampleBasicSync() {
  ctx := context.Background()
  
  // Initialize dependencies
  driveService := &drive.Service{} // Initialize with OAuth2
  rateLimiter := api.NewRateLimiter(10, 100) // 10 req/s, burst 100
  logger := logger.New(logger.Config{Level: "info"})
  
  // Create API client
  apiClient := api.NewDriveClient(driveService, rateLimiter, logger)
  
  // Create state manager
  stateManager, err := state.NewManager(state.Config{
    DatabasePath: "cloudpull.db",
  })
  if err != nil {
    log.Fatal("Failed to create state manager:", err)
  }
  defer stateManager.Close()
  
  // Create error handler
  errorHandler := errors.NewHandler(logger)
  
  // Create sync engine
  engine, err := NewEngine(apiClient, stateManager, errorHandler, logger, nil)
  if err != nil {
    log.Fatal("Failed to create sync engine:", err)
  }
  
  // Start new sync session
  rootFolderID := "root" // or specific folder ID
  destinationPath := "/path/to/local/folder"
  
  if err := engine.StartNewSession(ctx, rootFolderID, destinationPath); err != nil {
    log.Fatal("Failed to start sync:", err)
  }
  
  // Monitor progress
  go monitorProgress(engine)
  
  // Wait for completion or user interrupt
  // In a real application, you would handle signals
  time.Sleep(24 * time.Hour)
  
  // Stop engine
  engine.Stop()
}

// ExampleResumeSync demonstrates resuming a sync session
func ExampleResumeSync() {
  ctx := context.Background()
  
  // ... initialize dependencies as above ...
  
  // Resume existing session
  sessionID := "previous-session-id"
  
  engine, _ := NewEngine(nil, nil, nil, nil, nil) // with proper dependencies
  
  if err := engine.ResumeSession(ctx, sessionID); err != nil {
    log.Fatal("Failed to resume sync:", err)
  }
  
  // Monitor and control as before
}

// ExampleAdvancedSync demonstrates advanced configuration
func ExampleAdvancedSync() {
  // Custom configuration
  config := &EngineConfig{
    WalkerConfig: &WalkerConfig{
      Strategy:        TraversalBFS,
      MaxDepth:        0, // unlimited
      IncludePatterns: []string{".*\\.pdf$", ".*\\.docx$"}, // Only PDFs and Word docs
      ExcludePatterns: []string{"^\\.", "~\\$"}, // Skip hidden files and temp files
      FollowShortcuts: false,
      Concurrency:     5,
    },
    DownloadConfig: &DownloadManagerConfig{
      TempDir:         "/tmp/cloudpull",
      ChunkSize:       20 * 1024 * 1024, // 20MB chunks
      MaxConcurrent:   5,
      VerifyChecksums: true,
    },
    WorkerConfig: &WorkerPoolConfig{
      WorkerCount:     5,
      MaxRetries:      3,
      ShutdownTimeout: 60 * time.Second,
    },
    ProgressInterval:   time.Second,
    CheckpointInterval: 30 * time.Second,
    MaxErrors:          100,
  }
  
  // Create engine with custom config
  engine, _ := NewEngine(nil, nil, nil, nil, config) // with proper dependencies
  
  // Set bandwidth limit (1MB/s)
  engine.progressTracker.SetBandwidthLimit(1024 * 1024)
  
  // Start sync
  _ = engine
}

// ExampleProgressHandling demonstrates progress event handling
func ExampleProgressHandling() {
  progressTracker := NewProgressTracker("session-123")
  
  // Register event handlers
  progressTracker.OnEvent(func(event *ProgressEvent) {
    switch event.Type {
    case EventTypeFileStarted:
      fmt.Printf("Starting download: %s\n", event.ItemName)
      
    case EventTypeFileProgress:
      percent := float64(event.BytesTransferred) / float64(event.TotalBytes) * 100
      fmt.Printf("Progress: %s - %.1f%%\n", event.ItemName, percent)
      
    case EventTypeFileCompleted:
      fmt.Printf("Completed: %s\n", event.ItemName)
      
    case EventTypeFileFailed:
      fmt.Printf("Failed: %s - %s\n", event.ItemName, event.ErrorMessage)
      
    case EventTypeSessionUpdate:
      fmt.Printf("Overall progress: %d/%d files (%.1f%%)\n",
        event.FilesCompleted, event.TotalFiles,
        float64(event.FilesCompleted)/float64(event.TotalFiles)*100)
    }
  })
}

// monitorProgress monitors and displays sync progress
func monitorProgress(engine *Engine) {
  ticker := time.NewTicker(5 * time.Second)
  defer ticker.Stop()
  
  for range ticker.C {
    progress := engine.GetProgress()
    if progress == nil {
      continue
    }
    
    fmt.Printf("\n=== Sync Progress ===\n")
    fmt.Printf("Status: %s\n", progress.Status)
    fmt.Printf("Files: %d/%d (%.1f%%)\n", 
      progress.CompletedFiles, progress.TotalFiles,
      float64(progress.CompletedFiles)/float64(progress.TotalFiles)*100)
    fmt.Printf("Data: %s/%s (%.1f%%)\n",
      formatBytes(progress.CompletedBytes),
      formatBytes(progress.TotalBytes),
      float64(progress.CompletedBytes)/float64(progress.TotalBytes)*100)
    fmt.Printf("Speed: %s/s\n", formatBytes(progress.CurrentSpeed))
    fmt.Printf("ETA: %s\n", progress.RemainingTime)
    fmt.Printf("Active Downloads: %d\n", progress.ActiveDownloads)
    fmt.Printf("Queued: %d\n", progress.QueuedDownloads)
    fmt.Printf("Failed: %d\n", progress.FailedFiles)
    fmt.Printf("==================\n")
  }
}

// ExampleCustomWalker demonstrates using the folder walker directly
func ExampleCustomWalker() {
  ctx := context.Background()
  
  // Create walker with custom config
  config := &WalkerConfig{
    Strategy:        TraversalDFS,
    MaxDepth:        3, // Only go 3 levels deep
    ExcludePatterns: []string{"node_modules", "__pycache__"},
  }
  
  walker, _ := NewFolderWalker(nil, nil, nil, nil, config) // with proper dependencies
  
  // Start walking
  resultChan, err := walker.Walk(ctx, "root-folder-id", "session-id")
  if err != nil {
    log.Fatal("Failed to start walker:", err)
  }
  
  // Process results
  for result := range resultChan {
    if result.Error != nil {
      log.Printf("Error scanning folder: %v", result.Error)
      continue
    }
    
    fmt.Printf("Scanned folder: %s (%d files)\n", 
      result.Folder.Path, len(result.Files))
    
    // Process files as needed
    for _, file := range result.Files {
      fmt.Printf("  - %s (%s)\n", file.Name, formatBytes(file.Size))
    }
  }
}

// ExampleDownloadManager demonstrates direct download manager usage
func ExampleDownloadManager() {
  ctx := context.Background()
  
  // Create download manager
  config := &DownloadManagerConfig{
    TempDir:         "/tmp/cloudpull",
    ChunkSize:       10 * 1024 * 1024, // 10MB
    MaxConcurrent:   3,
    VerifyChecksums: true,
  }
  
  manager, _ := NewDownloadManager(nil, nil, nil, nil, nil, config) // with proper dependencies
  
  // Start manager
  if err := manager.Start(ctx); err != nil {
    log.Fatal("Failed to start download manager:", err)
  }
  defer manager.Stop()
  
  // Schedule downloads
  files := []*state.File{
    {ID: "1", DriveID: "drive-id-1", Name: "file1.pdf", Size: 1024 * 1024},
    {ID: "2", DriveID: "drive-id-2", Name: "file2.docx", Size: 2048 * 1024},
  }
  
  if err := manager.ScheduleBatch(files); err != nil {
    log.Fatal("Failed to schedule downloads:", err)
  }
  
  // Monitor stats
  ticker := time.NewTicker(time.Second)
  defer ticker.Stop()
  
  for range ticker.C {
    stats := manager.GetStats()
    fmt.Printf("Downloads: %d active, %d completed, %d failed\n",
      stats.ActiveDownloads,
      stats.CompletedDownloads,
      stats.FailedDownloads)
  }
}