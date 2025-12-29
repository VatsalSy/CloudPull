package sync

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VatsalSy/CloudPull/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// ProgressTracker Tests
// =============================================================================

func TestNewProgressTracker(t *testing.T) {
	sessionID := "test-session-123"
	pt := NewProgressTracker(sessionID)

	assert.NotNil(t, pt)
	assert.Equal(t, sessionID, pt.sessionID)
	assert.NotNil(t, pt.activeDownloads)
	assert.NotNil(t, pt.speedSamples)
	assert.Equal(t, 10, pt.maxSpeedSamples)
	assert.False(t, pt.startTime.IsZero())
}

func TestProgressTracker_SetTotals(t *testing.T) {
	pt := NewProgressTracker("session-1")

	pt.SetTotals(100, 1024000)

	pt.mu.RLock()
	defer pt.mu.RUnlock()
	assert.Equal(t, int64(100), pt.totalFiles)
	assert.Equal(t, int64(1024000), pt.totalBytes)
}

func TestProgressTracker_SetBandwidthLimit(t *testing.T) {
	pt := NewProgressTracker("session-1")

	pt.SetBandwidthLimit(1000000) // 1 MB/s

	pt.mu.RLock()
	defer pt.mu.RUnlock()
	assert.Equal(t, int64(1000000), pt.bandwidthLimit)
}

func TestProgressTracker_OnEvent(t *testing.T) {
	pt := NewProgressTracker("session-1")

	var receivedEvents []*ProgressEvent
	var mu sync.Mutex
	var wg sync.WaitGroup

	wg.Add(1) // Expect 1 event from SetTotals
	pt.OnEvent(func(event *ProgressEvent) {
		mu.Lock()
		receivedEvents = append(receivedEvents, event)
		mu.Unlock()
		wg.Done()
	})

	// Trigger an event
	pt.SetTotals(10, 1000)

	// Wait for async event handlers
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	assert.Greater(t, len(receivedEvents), 0)
}

func TestProgressTracker_FileLifecycle(t *testing.T) {
	pt := NewProgressTracker("session-1")
	pt.SetTotals(1, 1000)

	// Start file download
	pt.FileStarted("file-1", "test.txt", "/path/to/test.txt", 1000)

	pt.mu.RLock()
	fp, exists := pt.activeDownloads["file-1"]
	pt.mu.RUnlock()

	assert.True(t, exists)
	assert.Equal(t, "test.txt", fp.FileName)
	assert.Equal(t, int64(1000), fp.TotalBytes)

	// Update progress
	pt.FileProgress("file-1", 500)

	pt.mu.RLock()
	fp = pt.activeDownloads["file-1"]
	pt.mu.RUnlock()
	assert.Equal(t, int64(500), fp.BytesDownloaded)

	// Complete file
	pt.FileCompleted("file-1")

	pt.mu.RLock()
	_, exists = pt.activeDownloads["file-1"]
	pt.mu.RUnlock()
	assert.False(t, exists)

	assert.Equal(t, int64(1), atomic.LoadInt64(&pt.completedFiles))
}

func TestProgressTracker_FileFailed(t *testing.T) {
	pt := NewProgressTracker("session-1")

	// Start and fail a file
	pt.FileStarted("file-1", "test.txt", "/path/test.txt", 1000)
	testErr := errors.New("download failed")
	pt.FileFailed("file-1", testErr)

	pt.mu.RLock()
	_, exists := pt.activeDownloads["file-1"]
	pt.mu.RUnlock()

	assert.False(t, exists)
	assert.Equal(t, int64(1), atomic.LoadInt64(&pt.failedFiles))
}

func TestProgressTracker_FileSkipped(t *testing.T) {
	pt := NewProgressTracker("session-1")

	pt.FileSkipped("file-1", "test.txt", "/path/test.txt", "already exists")

	assert.Equal(t, int64(1), atomic.LoadInt64(&pt.skippedFiles))
}

func TestProgressTracker_FolderEvents(t *testing.T) {
	pt := NewProgressTracker("session-1")

	var receivedEvents []*ProgressEvent
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Expect 2 events: FolderStarted and FolderCompleted
	wg.Add(2)
	pt.OnEvent(func(event *ProgressEvent) {
		mu.Lock()
		receivedEvents = append(receivedEvents, event)
		mu.Unlock()
		wg.Done()
	})

	pt.FolderStarted("folder-1", "Documents", "/Documents")
	pt.FolderCompleted("folder-1", "Documents", "/Documents", 10)

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	var hasStarted, hasCompleted bool
	for _, e := range receivedEvents {
		if e.Type == ProgressEventFolderStarted {
			hasStarted = true
		}
		if e.Type == ProgressEventFolderCompleted {
			hasCompleted = true
		}
	}
	assert.True(t, hasStarted)
	assert.True(t, hasCompleted)
}

func TestProgressTracker_GetStats(t *testing.T) {
	pt := NewProgressTracker("session-1")
	pt.SetTotals(10, 10000)
	pt.SetBandwidthLimit(1000000)

	// Simulate some progress
	pt.FileStarted("file-1", "test.txt", "/test.txt", 1000)
	pt.FileProgress("file-1", 500)
	pt.FileCompleted("file-1")

	stats := pt.GetStats()

	assert.Equal(t, "session-1", stats.SessionID)
	assert.Equal(t, int64(10), stats.TotalFiles)
	assert.Equal(t, int64(10000), stats.TotalBytes)
	assert.Equal(t, int64(1), stats.CompletedFiles)
	assert.Equal(t, int64(1000000), stats.BandwidthLimit)
	assert.True(t, stats.ElapsedTime > 0)
}

func TestProgressTracker_FileProgressNonExistent(t *testing.T) {
	pt := NewProgressTracker("session-1")

	// Should not panic when updating non-existent file
	pt.FileProgress("non-existent", 500)

	pt.mu.RLock()
	defer pt.mu.RUnlock()
	assert.Empty(t, pt.activeDownloads)
}

func TestProgressTracker_FileCompletedNonExistent(t *testing.T) {
	pt := NewProgressTracker("session-1")

	// Should not panic when completing non-existent file
	pt.FileCompleted("non-existent")

	// completedFiles should NOT increment for non-existent files (returns early)
	assert.Equal(t, int64(0), atomic.LoadInt64(&pt.completedFiles))
}

func TestProgressTracker_MultipleHandlers(t *testing.T) {
	pt := NewProgressTracker("session-1")

	var count1, count2 int32
	var wg sync.WaitGroup

	// Expect each handler to be called once (2 handlers total)
	wg.Add(2)

	pt.OnEvent(func(event *ProgressEvent) {
		atomic.AddInt32(&count1, 1)
		wg.Done()
	})

	pt.OnEvent(func(event *ProgressEvent) {
		atomic.AddInt32(&count2, 1)
		wg.Done()
	})

	pt.SetTotals(10, 1000)

	wg.Wait()

	assert.True(t, atomic.LoadInt32(&count1) > 0)
	assert.True(t, atomic.LoadInt32(&count2) > 0)
}

// =============================================================================
// ProgressStats Tests
// =============================================================================

func TestProgressStats_Progress(t *testing.T) {
	tests := []struct {
		name           string
		totalFiles     int64
		completedFiles int64
		expected       float64
	}{
		{"no files", 0, 0, 0},
		{"no progress", 100, 0, 0},
		{"half done", 100, 50, 50},
		{"all done", 100, 100, 100},
		{"quarter done", 100, 25, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := &ProgressStats{
				TotalFiles:     tt.totalFiles,
				CompletedFiles: tt.completedFiles,
			}
			assert.Equal(t, tt.expected, ps.Progress())
		})
	}
}

func TestProgressStats_BytesProgress(t *testing.T) {
	tests := []struct {
		name           string
		totalBytes     int64
		completedBytes int64
		expected       float64
	}{
		{"no bytes", 0, 0, 0},
		{"no progress", 1000, 0, 0},
		{"half done", 1000, 500, 50},
		{"all done", 1000, 1000, 100},
		{"ten percent", 1000, 100, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := &ProgressStats{
				TotalBytes:     tt.totalBytes,
				CompletedBytes: tt.completedBytes,
			}
			assert.Equal(t, tt.expected, ps.BytesProgress())
		})
	}
}

// =============================================================================
// PriorityQueue Tests
// =============================================================================

func TestNewPriorityQueue(t *testing.T) {
	pq := NewPriorityQueue()

	assert.NotNil(t, pq)
	assert.Equal(t, 0, pq.Len())
}

func TestPriorityQueue_PushPop(t *testing.T) {
	pq := NewPriorityQueue()

	// Push tasks with different priorities
	task1 := &DownloadTask{
		File:     &state.File{ID: "file-1"},
		Priority: 10,
	}
	task2 := &DownloadTask{
		File:     &state.File{ID: "file-2"},
		Priority: 5,
	}
	task3 := &DownloadTask{
		File:     &state.File{ID: "file-3"},
		Priority: 15,
	}

	pq.Push(task1)
	pq.Push(task2)
	pq.Push(task3)

	assert.Equal(t, 3, pq.Len())

	// Pop should return in priority order (lowest first)
	popped := pq.Pop()
	assert.Equal(t, "file-2", popped.File.ID) // Priority 5

	popped = pq.Pop()
	assert.Equal(t, "file-1", popped.File.ID) // Priority 10

	popped = pq.Pop()
	assert.Equal(t, "file-3", popped.File.ID) // Priority 15

	assert.Equal(t, 0, pq.Len())
}

func TestPriorityQueue_PopEmpty(t *testing.T) {
	pq := NewPriorityQueue()

	result := pq.Pop()
	assert.Nil(t, result)
}

func TestPriorityQueue_ConcurrentAccess(t *testing.T) {
	pq := NewPriorityQueue()

	var wg sync.WaitGroup
	numGoroutines := 10
	tasksPerGoroutine := 100

	// Concurrent pushes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for j := 0; j < tasksPerGoroutine; j++ {
				task := &DownloadTask{
					File:     &state.File{ID: "file"},
					Priority: base*tasksPerGoroutine + j,
				}
				pq.Push(task)
			}
		}(i)
	}

	wg.Wait()
	assert.Equal(t, numGoroutines*tasksPerGoroutine, pq.Len())

	// Concurrent pops
	var popCount int32
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < tasksPerGoroutine; j++ {
				if pq.Pop() != nil {
					atomic.AddInt32(&popCount, 1)
				}
			}
		}()
	}

	wg.Wait()
	assert.Equal(t, int32(numGoroutines*tasksPerGoroutine), popCount)
	assert.Equal(t, 0, pq.Len())
}

// =============================================================================
// Configuration Tests
// =============================================================================

func TestDefaultWalkerConfig(t *testing.T) {
	cfg := DefaultWalkerConfig()

	assert.NotNil(t, cfg)
	assert.Equal(t, TraversalBFS, cfg.Strategy)
	assert.Equal(t, 0, cfg.MaxDepth)
	assert.False(t, cfg.FollowShortcuts)
	assert.Equal(t, 3, cfg.Concurrency)
	assert.Equal(t, 100, cfg.ChannelBufferSize)
}

func TestDefaultDownloadManagerConfig(t *testing.T) {
	cfg := DefaultDownloadManagerConfig()

	assert.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.TempDir)
	assert.Equal(t, int64(10*1024*1024), cfg.ChunkSize) // 10MB
	assert.Equal(t, 3, cfg.MaxConcurrent)
	assert.True(t, cfg.VerifyChecksums)
}

func TestDefaultWorkerPoolConfig(t *testing.T) {
	cfg := DefaultWorkerPoolConfig()

	assert.NotNil(t, cfg)
	assert.Equal(t, 3, cfg.WorkerCount)
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, 30*time.Second, cfg.ShutdownTimeout)
}

func TestDefaultEngineConfig(t *testing.T) {
	cfg := DefaultEngineConfig()

	assert.NotNil(t, cfg)
	assert.NotNil(t, cfg.WalkerConfig)
	assert.NotNil(t, cfg.DownloadConfig)
	assert.NotNil(t, cfg.WorkerConfig)
	assert.Equal(t, time.Second, cfg.ProgressInterval)
	assert.Equal(t, 30*time.Second, cfg.CheckpointInterval)
	assert.Equal(t, 100, cfg.MaxErrors)
}

// =============================================================================
// Helper Function Tests
// =============================================================================

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{"zero bytes", 0, "0 B"},
		{"bytes", 512, "512 B"},
		{"kilobytes", 1024, "1.0 KB"},
		{"megabytes", 1024 * 1024, "1.0 MB"},
		{"gigabytes", 1024 * 1024 * 1024, "1.0 GB"},
		{"terabytes", 1024 * 1024 * 1024 * 1024, "1.0 TB"},
		{"fractional KB", 1536, "1.5 KB"},
		{"fractional MB", 1572864, "1.5 MB"},
		{"large GB", 5 * 1024 * 1024 * 1024, "5.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBytes(tt.bytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateID(t *testing.T) {
	// Test that IDs are generated with the required prefix
	id1 := generateID()
	assert.NotEmpty(t, id1)
	assert.True(t, strings.HasPrefix(id1, "session_"), "ID should have 'session_' prefix")

	// Test uniqueness - two consecutive calls should produce different IDs
	id2 := generateID()
	assert.NotEqual(t, id1, id2, "consecutive IDs should be unique")
}

func TestGenerateID_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	numIDs := 1000

	for i := 0; i < numIDs; i++ {
		id := generateID()
		require.False(t, ids[id], "Duplicate ID generated: %s", id)
		ids[id] = true
	}

	assert.Equal(t, numIDs, len(ids))
}

// =============================================================================
// TraversalStrategy Tests
// =============================================================================

func TestTraversalStrategy(t *testing.T) {
	// Verify strategies are distinct values (don't rely on specific integer values)
	assert.NotEqual(t, TraversalBFS, TraversalDFS, "traversal strategies must be distinct")

	// Verify default config uses BFS (the documented default)
	cfg := DefaultWalkerConfig()
	assert.Equal(t, TraversalBFS, cfg.Strategy, "default strategy should be BFS")

	// Verify both strategies are valid TraversalStrategy values
	strategies := []TraversalStrategy{TraversalBFS, TraversalDFS}
	for _, s := range strategies {
		assert.IsType(t, TraversalStrategy(0), s)
	}
}

// =============================================================================
// ProgressEventType Tests
// =============================================================================

func TestProgressEventTypes(t *testing.T) {
	assert.Equal(t, ProgressEventType("file_started"), ProgressEventFileStarted)
	assert.Equal(t, ProgressEventType("file_progress"), ProgressEventFileProgress)
	assert.Equal(t, ProgressEventType("file_completed"), ProgressEventFileCompleted)
	assert.Equal(t, ProgressEventType("file_failed"), ProgressEventFileFailed)
	assert.Equal(t, ProgressEventType("folder_started"), ProgressEventFolderStarted)
	assert.Equal(t, ProgressEventType("folder_completed"), ProgressEventFolderCompleted)
	assert.Equal(t, ProgressEventType("session_update"), ProgressEventSessionUpdate)
	assert.Equal(t, ProgressEventType("bandwidth_update"), ProgressEventBandwidthUpdate)
}

// =============================================================================
// WalkerStats Tests (via struct usage)
// =============================================================================

func TestWalkerStats(t *testing.T) {
	stats := &WalkerStats{
		FoldersScanned: 10,
		FilesFound:     100,
		TotalSize:      1024 * 1024,
		ErrorCount:     1,
	}

	assert.Equal(t, int64(10), stats.FoldersScanned)
	assert.Equal(t, int64(100), stats.FilesFound)
	assert.Equal(t, int64(1024*1024), stats.TotalSize)
	assert.Equal(t, 1, stats.ErrorCount)
}

// =============================================================================
// DownloadStats Tests
// =============================================================================

func TestDownloadStats(t *testing.T) {
	stats := &DownloadStats{}

	// Test concurrent access using thread-safe methods
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stats.IncrementDownloads()
			stats.AddBytes(1000)
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(100), stats.TotalDownloads)
	assert.Equal(t, int64(100000), stats.BytesDownloaded)
}

// =============================================================================
// WorkerPoolStats Tests
// =============================================================================

func TestWorkerPoolStats(t *testing.T) {
	stats := &WorkerPoolStats{
		WorkerCount:     5,
		ActiveWorkers:   3,
		QueuedTasks:     10,
		TasksProcessed:  100,
		TasksSucceeded:  95,
		TasksFailed:     5,
		BytesDownloaded: 1024 * 1024 * 100,
	}

	assert.Equal(t, 5, stats.WorkerCount)
	assert.Equal(t, 3, stats.ActiveWorkers)
	assert.Equal(t, 10, stats.QueuedTasks)
	assert.Equal(t, int64(100), stats.TasksProcessed)
	assert.Equal(t, int64(95), stats.TasksSucceeded)
	assert.Equal(t, int64(5), stats.TasksFailed)
	assert.Equal(t, int64(1024*1024*100), stats.BytesDownloaded)
}

// =============================================================================
// DownloadTask Tests
// =============================================================================

func TestDownloadTask(t *testing.T) {
	now := time.Now()
	file := &state.File{
		ID:   "file-1",
		Name: "test.txt",
		Size: 1024,
	}

	task := &DownloadTask{
		File:      file,
		Priority:  1,
		CreatedAt: now,
		Retries:   0,
	}

	assert.Equal(t, file, task.File)
	assert.Equal(t, 1, task.Priority)
	assert.Equal(t, now, task.CreatedAt)
	assert.Equal(t, 0, task.Retries)
	assert.Nil(t, task.StartedAt)
	assert.Nil(t, task.CompletedAt)
	assert.Nil(t, task.LastError)
}

// =============================================================================
// TaskResult Tests
// =============================================================================

func TestTaskResult(t *testing.T) {
	task := &DownloadTask{
		File: &state.File{ID: "file-1"},
	}

	result := &TaskResult{
		Task:         task,
		Success:      true,
		BytesWritten: 1024,
		Duration:     5 * time.Second,
		WorkerID:     1,
	}

	assert.Equal(t, task, result.Task)
	assert.True(t, result.Success)
	assert.Nil(t, result.Error)
	assert.Equal(t, int64(1024), result.BytesWritten)
	assert.Equal(t, 5*time.Second, result.Duration)
	assert.Equal(t, 1, result.WorkerID)
}

// =============================================================================
// SyncProgress Tests
// =============================================================================

func TestSyncProgress(t *testing.T) {
	progress := &SyncProgress{
		SessionID:       "session-1",
		Status:          "running",
		StartTime:       time.Now(),
		ElapsedTime:     5 * time.Minute,
		RemainingTime:   10 * time.Minute,
		TotalFiles:      100,
		CompletedFiles:  50,
		FailedFiles:     5,
		SkippedFiles:    3,
		TotalBytes:      1024 * 1024 * 1024,
		CompletedBytes:  512 * 1024 * 1024,
		CurrentSpeed:    10 * 1024 * 1024,
		AverageSpeed:    8 * 1024 * 1024,
		FoldersScanned:  20,
		ActiveDownloads: 3,
		QueuedDownloads: 42,
	}

	assert.Equal(t, "session-1", progress.SessionID)
	assert.Equal(t, "running", progress.Status)
	assert.Equal(t, int64(100), progress.TotalFiles)
	assert.Equal(t, int64(50), progress.CompletedFiles)
	assert.Equal(t, int64(5), progress.FailedFiles)
	assert.Equal(t, int64(3), progress.SkippedFiles)
	assert.Equal(t, 5*time.Minute, progress.ElapsedTime)
	assert.Equal(t, 10*time.Minute, progress.RemainingTime)
}

// =============================================================================
// FileProgress Tests
// =============================================================================

func TestFileProgress_Struct(t *testing.T) {
	fp := &FileProgress{
		FileID:          "file-1",
		FileName:        "test.txt",
		FilePath:        "/path/to/test.txt",
		TotalBytes:      1024,
		BytesDownloaded: 512,
		Speed:           100,
		StartTime:       time.Now(),
		LastUpdate:      time.Now(),
	}

	assert.Equal(t, "file-1", fp.FileID)
	assert.Equal(t, "test.txt", fp.FileName)
	assert.Equal(t, "/path/to/test.txt", fp.FilePath)
	assert.Equal(t, int64(1024), fp.TotalBytes)
	assert.Equal(t, int64(512), fp.BytesDownloaded)
	assert.Equal(t, int64(100), fp.Speed)
}

// =============================================================================
// ProgressEvent Tests
// =============================================================================

func TestProgressEvent(t *testing.T) {
	testErr := errors.New("test error")
	event := &ProgressEvent{
		Type:             ProgressEventFileFailed,
		Timestamp:        time.Now(),
		SessionID:        "session-1",
		ItemID:           "file-1",
		ItemName:         "test.txt",
		ItemPath:         "/path/test.txt",
		Error:            testErr,
		ErrorMessage:     testErr.Error(),
		BytesTransferred: 500,
		TotalBytes:       1000,
		TotalFiles:       10,
		FilesCompleted:   5,
		CurrentSpeed:     1024,
		AverageSpeed:     2048,
		RemainingTime:    5 * time.Minute,
		Context: map[string]interface{}{
			"retry": 1,
		},
	}

	assert.Equal(t, ProgressEventFileFailed, event.Type)
	assert.Equal(t, "session-1", event.SessionID)
	assert.Equal(t, "file-1", event.ItemID)
	assert.Equal(t, testErr, event.Error)
	assert.Equal(t, "test error", event.ErrorMessage)
	assert.Equal(t, 1, event.Context["retry"])
}

// =============================================================================
// WalkResult Tests
// =============================================================================

func TestWalkResult(t *testing.T) {
	folder := &state.Folder{
		ID:   "folder-1",
		Name: "Documents",
	}
	files := []*state.File{
		{ID: "file-1", Name: "doc1.txt"},
		{ID: "file-2", Name: "doc2.txt"},
	}

	result := &WalkResult{
		Folder:     folder,
		Files:      files,
		Depth:      2,
		IsSkipped:  false,
		SkipReason: "",
		Error:      nil,
	}

	assert.Equal(t, folder, result.Folder)
	assert.Len(t, result.Files, 2)
	assert.Equal(t, 2, result.Depth)
	assert.False(t, result.IsSkipped)
	assert.Nil(t, result.Error)
}

func TestWalkResult_Skipped(t *testing.T) {
	result := &WalkResult{
		Folder:     &state.Folder{ID: "folder-1"},
		IsSkipped:  true,
		SkipReason: "excluded by pattern",
	}

	assert.True(t, result.IsSkipped)
	assert.Equal(t, "excluded by pattern", result.SkipReason)
}

func TestWalkResult_Error(t *testing.T) {
	testErr := errors.New("folder access denied")
	result := &WalkResult{
		Error: testErr,
	}

	assert.Equal(t, testErr, result.Error)
}

// =============================================================================
// DownloadInfo Tests
// =============================================================================

func TestDownloadInfo(t *testing.T) {
	info := &DownloadInfo{
		FileID:          "file-1",
		FileName:        "test.pdf",
		TempPath:        "/tmp/download-123",
		FinalPath:       "/downloads/test.pdf",
		Size:            1024 * 1024,
		BytesDownloaded: 512 * 1024,
		IsGoogleDoc:     false,
		ExportFormat:    "",
		Checksum:        "abc123",
		StartTime:       time.Now(),
	}

	assert.Equal(t, "file-1", info.FileID)
	assert.Equal(t, "test.pdf", info.FileName)
	assert.Equal(t, "/tmp/download-123", info.TempPath)
	assert.Equal(t, "/downloads/test.pdf", info.FinalPath)
	assert.Equal(t, int64(1024*1024), info.Size)
	assert.Equal(t, int64(512*1024), info.BytesDownloaded)
	assert.False(t, info.IsGoogleDoc)
	assert.Equal(t, "abc123", info.Checksum)
}

func TestDownloadInfo_GoogleDoc(t *testing.T) {
	info := &DownloadInfo{
		FileID:       "doc-1",
		FileName:     "My Document",
		IsGoogleDoc:  true,
		ExportFormat: "application/pdf",
	}

	assert.True(t, info.IsGoogleDoc)
	assert.Equal(t, "application/pdf", info.ExportFormat)
}

// =============================================================================
// DownloadManagerStats Tests
// =============================================================================

func TestDownloadManagerStats(t *testing.T) {
	stats := &DownloadManagerStats{
		ActiveDownloads: 3,
		WorkerPoolStats: &WorkerPoolStats{
			WorkerCount:    5,
			QueuedTasks:    10,
			TasksProcessed: 100,
		},
	}

	assert.Equal(t, int64(3), stats.ActiveDownloads)
	assert.Equal(t, 5, stats.WorkerPoolStats.WorkerCount)
	assert.Equal(t, 10, stats.WorkerPoolStats.QueuedTasks)
}
