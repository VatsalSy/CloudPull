# CloudPull Sync Engine

## Overview
The sync engine is the heart of CloudPull, orchestrating the synchronization of Google Drive folders to local storage.

## Architecture

### Components

1. **Engine** (`engine.go`)
   - Main orchestrator for sync operations
   - Manages sync sessions and lifecycle
   - Coordinates walker and downloader

2. **Walker** (`walker.go`)
   - Traverses Google Drive folder structures
   - Memory-efficient pagination
   - Supports BFS and DFS strategies

3. **Downloader** (`downloader.go`)
   - Manages file downloads with resume support
   - Handles Google Docs export
   - Checksum verification

4. **Worker** (`worker.go`)
   - Concurrent download workers
   - Priority queue processing
   - Health monitoring and recovery

5. **Progress** (`progress.go`)
   - Real-time progress tracking
   - Event emission for UI updates
   - Bandwidth calculation

## Usage Example

```go
// Initialize dependencies
logger := logger.NewLogger(logConfig)
db, _ := state.NewDatabase(dbConfig)
stateManager := state.NewManager(db, logger)
apiClient := api.NewClient(authConfig, logger)
progressTracker := progress.NewTracker()

// Create sync engine
engine := sync.NewEngine(sync.Config{
    StateManager:    stateManager,
    APIClient:       apiClient,
    Logger:          logger,
    ProgressTracker: progressTracker,
    // ... other config
})

// Start sync
ctx := context.Background()
sessionID, err := engine.StartSync(ctx, "drive-folder-id", "/local/path")

// Or resume existing sync
err = engine.ResumeSync(ctx, sessionID)
```

## Features

### Memory Efficiency
- Streams folder contents without loading entire tree
- Pagination prevents memory overflow
- Batch processing for database operations

### Reliability
- Automatic retry with exponential backoff
- Resume from exact byte offset
- Checksum verification
- Atomic file operations

### Performance
- Concurrent downloads (configurable)
- Priority queue (smallest files first)
- Bandwidth throttling
- Progress batching

### Monitoring
- Real-time progress events
- Detailed error logging
- Performance metrics
- Health checks

## Configuration

```go
type Config struct {
    MaxConcurrentDownloads int
    DownloadChunkSize      int64
    BandwidthLimit         int64
    RetryAttempts          int
    RetryBackoff           time.Duration
    TempDir                string
    VerifyChecksums        bool
}
```

## Error Handling

The sync engine uses the centralized error handler for:
- Network errors: Retry with backoff
- API quota errors: Longer backoff
- Permission errors: Skip file
- Storage errors: Pause sync
- Corruption: Re-download

## Events

Subscribe to sync events:

```go
progressTracker.Subscribe(func(snapshot progress.Snapshot) {
    fmt.Printf("Progress: %.2f%% (%.2f MB/s)\n", 
        snapshot.Percentage, 
        snapshot.BytesPerSecond/1024/1024)
})
```

Event types:
- FileStarted
- FileProgress
- FileCompleted
- FileFailed
- FolderStarted
- FolderCompleted
- SyncPaused
- SyncResumed