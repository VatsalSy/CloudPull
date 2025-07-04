/**
 * Main Sync Engine Orchestrator for CloudPull
 *
 * Features:
 * - Coordinates folder walking and file downloading
 * - Manages sync sessions and state persistence
 * - Handles pause/resume functionality
 * - Provides real-time progress monitoring
 * - Implements graceful shutdown
 *
 * Author: CloudPull Team
 * Updated: 2025-01-29
 */

package sync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/VatsalSy/CloudPull/internal/api"
	"github.com/VatsalSy/CloudPull/internal/errors"
	"github.com/VatsalSy/CloudPull/internal/logger"
	"github.com/VatsalSy/CloudPull/internal/state"
)

// Engine is the main sync orchestrator.
type Engine struct {
	ctx             context.Context
	progressTracker *ProgressTracker
	config          *EngineConfig
	stateManager    *state.Manager
	errorHandler    *errors.Handler
	logger          *logger.Logger
	walker          *FolderWalker
	downloader      *DownloadManager
	doneChan        chan struct{}
	client          *api.DriveClient
	currentSession  *state.Session
	errorChan       chan error
	cancel          context.CancelFunc
	sessionID       string
	wg              sync.WaitGroup
	mu              sync.RWMutex
	isPaused        bool
	isRunning       bool
	walkingComplete bool
}

// EngineConfig contains configuration for the sync engine.
type EngineConfig struct {
	// Folder walker configuration
	WalkerConfig *WalkerConfig

	// Download manager configuration
	DownloadConfig *DownloadManagerConfig

	// Worker pool configuration
	WorkerConfig *WorkerPoolConfig

	// Progress update interval
	ProgressInterval time.Duration

	// Session checkpoint interval
	CheckpointInterval time.Duration

	// Maximum errors before stopping
	MaxErrors int
}

// DefaultEngineConfig returns default engine configuration.
func DefaultEngineConfig() *EngineConfig {
	return &EngineConfig{
		WalkerConfig:       DefaultWalkerConfig(),
		DownloadConfig:     DefaultDownloadManagerConfig(),
		WorkerConfig:       DefaultWorkerPoolConfig(),
		ProgressInterval:   time.Second,
		CheckpointInterval: 30 * time.Second,
		MaxErrors:          100,
	}
}

// NewEngine creates a new sync engine.
func NewEngine(
	client *api.DriveClient,
	stateManager *state.Manager,
	errorHandler *errors.Handler,
	logger *logger.Logger,
	config *EngineConfig,
) (*Engine, error) {

	if config == nil {
		config = DefaultEngineConfig()
	}

	engine := &Engine{
		config:       config,
		client:       client,
		stateManager: stateManager,
		errorHandler: errorHandler,
		logger:       logger,
		errorChan:    make(chan error, config.MaxErrors),
		doneChan:     make(chan struct{}),
	}

	return engine, nil
}

// StartNewSession starts a new sync session.
func (e *Engine) StartNewSession(ctx context.Context, rootFolderID, destinationPath string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.isRunning {
		return errors.Errorf("sync engine is already running")
	}

	// Create new session
	session, err := e.createSession(ctx, rootFolderID, destinationPath)
	if err != nil {
		return errors.Wrap(err, "failed to create session")
	}

	e.currentSession = session
	e.sessionID = session.ID

	// Start sync
	return e.startSync(ctx)
}

// StartNewSessionWithID starts a new sync session and returns the session ID.
func (e *Engine) StartNewSessionWithID(ctx context.Context, rootFolderID, destinationPath string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.isRunning {
		return "", errors.Errorf("sync engine is already running")
	}

	// Create new session
	session, err := e.createSession(ctx, rootFolderID, destinationPath)
	if err != nil {
		return "", errors.Wrap(err, "failed to create session")
	}

	e.currentSession = session
	e.sessionID = session.ID

	// Start sync
	if err := e.startSync(ctx); err != nil {
		return "", err
	}

	return session.ID, nil
}

// ResumeSession resumes an existing sync session.
func (e *Engine) ResumeSession(ctx context.Context, sessionID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.isRunning {
		return errors.Errorf("sync engine is already running")
	}

	// Load session
	session, err := e.stateManager.GetSession(ctx, sessionID)
	if err != nil {
		return errors.Wrap(err, "failed to load session")
	}

	if session == nil {
		return errors.Errorf("session not found: %s", sessionID)
	}

	// Check if session can be resumed
	if session.Status == state.SessionStatusCompleted {
		return errors.Errorf("session is already completed")
	}

	if session.Status == state.SessionStatusFailed || session.Status == state.SessionStatusCancelled {
		return errors.Errorf("session cannot be resumed: status=%s", session.Status)
	}

	e.currentSession = session
	e.sessionID = session.ID

	// Start sync
	return e.startSync(ctx)
}

// Pause pauses the sync engine.
func (e *Engine) Pause() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.isRunning {
		return errors.Errorf("sync engine is not running")
	}

	if e.isPaused {
		return errors.Errorf("sync engine is already paused")
	}

	e.isPaused = true
	e.logger.Info("Sync engine paused")

	// Update session status
	if e.currentSession != nil {
		e.currentSession.Status = state.SessionStatusPaused
		if err := e.stateManager.UpdateSessionStatus(e.ctx, e.sessionID, state.SessionStatusPaused); err != nil {
			e.logger.Error(err, "Failed to update session status")
		}
	}

	return nil
}

// Resume resumes a paused sync engine.
func (e *Engine) Resume() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.isRunning {
		return errors.Errorf("sync engine is not running")
	}

	if !e.isPaused {
		return errors.Errorf("sync engine is not paused")
	}

	e.isPaused = false
	e.logger.Info("Sync engine resumed")

	// Update session status
	if e.currentSession != nil {
		e.currentSession.Status = state.SessionStatusActive
		if err := e.stateManager.UpdateSessionStatus(e.ctx, e.sessionID, state.SessionStatusActive); err != nil {
			e.logger.Error(err, "Failed to update session status")
		}
	}

	return nil
}

// Stop stops the sync engine.
func (e *Engine) Stop() error {
	e.mu.Lock()
	if !e.isRunning {
		e.mu.Unlock()
		return nil
	}
	e.mu.Unlock()

	e.logger.Info("Stopping sync engine...")

	// Cancel context
	if e.cancel != nil {
		e.cancel()
	}

	// Wait for completion
	select {
	case <-e.doneChan:
		e.logger.Info("Sync engine stopped")
	case <-time.After(60 * time.Second):
		e.logger.Warn("Sync engine stop timeout")
	}

	return nil
}

// GetProgress returns current sync progress.
func (e *Engine) GetProgress() *SyncProgress {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.progressTracker == nil {
		return nil
	}

	stats := e.progressTracker.GetStats()
	walkerStats := &WalkerStats{}
	if e.walker != nil {
		walkerStats = e.walker.GetStats()
	}

	downloadStats := &DownloadManagerStats{}
	if e.downloader != nil {
		downloadStats = e.downloader.GetStats()
	}

	return &SyncProgress{
		SessionID:       e.sessionID,
		Status:          e.getStatus(),
		StartTime:       stats.StartTime,
		ElapsedTime:     stats.ElapsedTime,
		RemainingTime:   stats.RemainingTime,
		TotalFiles:      stats.TotalFiles,
		CompletedFiles:  stats.CompletedFiles,
		FailedFiles:     stats.FailedFiles,
		SkippedFiles:    stats.SkippedFiles,
		TotalBytes:      stats.TotalBytes,
		CompletedBytes:  stats.CompletedBytes,
		CurrentSpeed:    stats.CurrentSpeed,
		AverageSpeed:    stats.AverageSpeed,
		FoldersScanned:  walkerStats.FoldersScanned,
		ActiveDownloads: downloadStats.ActiveDownloads,
		QueuedDownloads: downloadStats.WorkerPoolStats.QueuedTasks,
	}
}

// WaitForCompletion waits until the sync engine completes.
func (e *Engine) WaitForCompletion() <-chan struct{} {
	return e.doneChan
}

// startSync starts the sync process.
func (e *Engine) startSync(ctx context.Context) error {
	// Create cancellable context
	e.ctx, e.cancel = context.WithCancel(ctx)

	// Create progress tracker
	e.progressTracker = NewProgressTracker(e.sessionID)

	// Register progress event handler
	e.progressTracker.OnEvent(func(event *ProgressEvent) {
		// Log significant events
		switch event.Type {
		case ProgressEventFileFailed:
			e.logger.Error(event.Error, "File download failed",
				"file", event.ItemName,
				"path", event.ItemPath,
			)
		case ProgressEventSessionUpdate:
			if event.FilesCompleted%100 == 0 {
				e.logger.Info("Sync progress",
					"completed", event.FilesCompleted,
					"total", event.TotalFiles,
					"speed", formatBytes(event.CurrentSpeed)+"/s",
				)
			}
		}
	})

	// Create folder walker
	walker, err := NewFolderWalker(
		e.client,
		e.stateManager,
		e.progressTracker,
		e.logger,
		e.config.WalkerConfig,
	)
	if err != nil {
		return errors.Wrap(err, "failed to create folder walker")
	}
	e.walker = walker

	// Create download manager
	downloader, err := NewDownloadManager(
		e.client,
		e.stateManager,
		e.progressTracker,
		e.errorHandler,
		e.logger,
		e.config.DownloadConfig,
	)
	if err != nil {
		return errors.Wrap(err, "failed to create download manager")
	}
	e.downloader = downloader

	// Start download manager
	if err := e.downloader.Start(e.ctx); err != nil {
		return errors.Wrap(err, "failed to start download manager")
	}

	// Mark as running
	e.isRunning = true
	e.walkingComplete = false

	// Update session status
	e.currentSession.Status = state.SessionStatusActive
	if err := e.stateManager.UpdateSessionStatus(e.ctx, e.sessionID, state.SessionStatusActive); err != nil {
		e.logger.Error(err, "Failed to update session status")
	}

	// Start main sync loop
	e.wg.Add(1)
	go e.runSync()

	// Start checkpoint saver
	e.wg.Add(1)
	go e.runCheckpointSaver()

	// Start error monitor
	e.wg.Add(1)
	go e.runErrorMonitor()

	// Start completion checker
	e.wg.Add(1)
	go e.runCompletionChecker()

	e.logger.Info("Sync engine started",
		"session_id", e.sessionID,
		"root_folder", e.currentSession.RootFolderID,
		"destination", e.currentSession.DestinationPath,
	)

	return nil
}

// runSync is the main sync loop.
func (e *Engine) runSync() {
	defer e.wg.Done()
	defer e.cleanup()

	// Check if resuming
	if e.isResuming() {
		e.logger.Info("Resuming sync session",
			"completed_files", e.currentSession.CompletedFiles,
			"total_files", e.currentSession.TotalFiles,
		)

		// When resuming, walking is already complete
		e.walkingComplete = true

		// Schedule pending downloads
		if err := e.schedulePendingDownloads(); err != nil {
			e.logger.Error(err, "Failed to schedule pending downloads")
			e.handleFatalError(err)
			return
		}
	} else {
		// Start folder walking
		e.logger.Info("Starting folder scan")
		e.logger.Debug("About to call startFolderWalk", "rootFolderID", e.currentSession.RootFolderID)
		if err := e.startFolderWalk(); err != nil {
			e.logger.Error(err, "Failed to start folder walk")
			e.handleFatalError(err)
			return
		}
		e.logger.Debug("startFolderWalk completed successfully")
	}

	// Wait for completion or cancellation
	<-e.ctx.Done()

	// Determine final status
	if e.ctx.Err() == context.Canceled {
		e.updateFinalStatus(state.SessionStatusCancelled)
	} else {
		stats := e.progressTracker.GetStats()
		if stats.FailedFiles > 0 {
			e.updateFinalStatus(state.SessionStatusFailed)
		} else {
			e.updateFinalStatus(state.SessionStatusCompleted)
		}
	}
}

// startFolderWalk starts the folder walking process.
func (e *Engine) startFolderWalk() error {
	e.logger.Debug("startFolderWalk called", "rootFolderID", e.currentSession.RootFolderID, "sessionID", e.sessionID)

	// Start walking from root folder
	resultChan, err := e.walker.Walk(e.ctx, e.currentSession.RootFolderID, e.sessionID)
	if err != nil {
		e.logger.Error(err, "Failed to start walker")
		return err
	}
	e.logger.Debug("Walker started successfully")

	// Process walk results
	go func() {
		totalFiles := int64(0)
		totalBytes := int64(0)
		batchSize := 100
		fileBatch := make([]*state.File, 0, batchSize)

		for result := range resultChan {
			if e.ctx.Err() != nil {
				return
			}

			// Check if paused
			for e.isPaused {
				select {
				case <-e.ctx.Done():
					return
				case <-time.After(time.Second):
					continue
				}
			}

			// Handle errors
			if result.Error != nil {
				e.errorChan <- result.Error
				continue
			}

			// Process files
			if len(result.Files) > 0 {
				e.logger.Debug("Processing walk result",
					"folder", result.Folder.Name,
					"files_count", len(result.Files),
					"total_files_so_far", totalFiles,
				)

				totalFiles += int64(len(result.Files))
				for _, file := range result.Files {
					totalBytes += file.Size
					fileBatch = append(fileBatch, file)

					// Schedule batch when full
					if len(fileBatch) >= batchSize {
						e.logger.Debug("Scheduling file batch",
							"batch_size", len(fileBatch),
							"total_scheduled", totalFiles,
						)
						e.downloader.ScheduleBatch(fileBatch)
						fileBatch = make([]*state.File, 0, batchSize)
					}
				}
			}

			// Update totals immediately when we have files
			if totalFiles > 0 && (totalFiles <= 100 || totalFiles%1000 == 0) {
				e.progressTracker.SetTotals(totalFiles, totalBytes)
				e.updateSessionTotals(totalFiles, totalBytes)
			}
		}

		// Schedule remaining files
		if len(fileBatch) > 0 {
			e.downloader.ScheduleBatch(fileBatch)
		}

		// Final update
		e.progressTracker.SetTotals(totalFiles, totalBytes)
		e.updateSessionTotals(totalFiles, totalBytes)

		e.logger.Info("Folder scan completed",
			"folders", e.walker.GetStats().FoldersScanned,
			"files", totalFiles,
			"size", formatBytes(totalBytes),
		)

		// Signal that walking is complete
		e.walkingComplete = true
		e.checkIfSyncComplete()
	}()

	return nil
}

// schedulePendingDownloads schedules pending downloads when resuming.
func (e *Engine) schedulePendingDownloads() error {
	// Get pending files
	files, err := e.stateManager.GetPendingFiles(e.ctx, e.sessionID, 1000)
	if err != nil {
		return errors.Wrap(err, "failed to get pending files")
	}

	e.logger.Info("Scheduling pending downloads",
		"count", len(files),
	)

	// Schedule downloads
	return e.downloader.ScheduleBatch(files)
}

// runCheckpointSaver periodically saves session state.
func (e *Engine) runCheckpointSaver() {
	defer e.wg.Done()

	ticker := time.NewTicker(e.config.CheckpointInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.saveCheckpoint()
		}
	}
}

// saveCheckpoint saves current session state.
func (e *Engine) saveCheckpoint() {
	stats := e.progressTracker.GetStats()

	// Update session
	e.mu.Lock()
	e.currentSession.CompletedFiles = stats.CompletedFiles
	e.currentSession.FailedFiles = stats.FailedFiles
	e.currentSession.SkippedFiles = stats.SkippedFiles
	e.currentSession.CompletedBytes = stats.CompletedBytes
	session := *e.currentSession
	e.mu.Unlock()

	// Save to database
	if err := e.stateManager.UpdateSession(e.ctx, &session); err != nil {
		e.logger.Error(err, "Failed to save checkpoint")
	}
}

// runErrorMonitor monitors errors and stops if threshold exceeded.
func (e *Engine) runErrorMonitor() {
	defer e.wg.Done()

	errorCount := 0

	for {
		select {
		case <-e.ctx.Done():
			return
		case err := <-e.errorChan:
			errorCount++
			e.logger.Error(err, "Sync error",
				"count", errorCount,
				"max", e.config.MaxErrors,
			)

			if errorCount >= e.config.MaxErrors {
				e.logger.Error(nil, "Maximum errors exceeded, stopping sync")
				e.cancel()
				return
			}
		}
	}
}

// runCompletionChecker periodically checks if the sync is complete.
func (e *Engine) runCompletionChecker() {
	defer e.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.checkIfSyncComplete()
		}
	}
}

// cleanup performs cleanup after sync stops.
func (e *Engine) cleanup() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.isRunning = false
	e.isPaused = false

	// Stop components
	if e.walker != nil {
		e.walker.Stop()
	}

	if e.downloader != nil {
		e.downloader.Stop()
	}

	// Save final checkpoint
	e.saveCheckpoint()

	// Close done channel to signal completion
	close(e.doneChan)
}

// Helper methods

// createSession creates a new sync session.
func (e *Engine) createSession(ctx context.Context, rootFolderID, destinationPath string) (*state.Session, error) {
	// Get root folder name
	var rootFolderName string
	if rootFolderID == "root" {
		rootFolderName = "My Drive"
	} else {
		info, err := e.client.GetFile(ctx, rootFolderID)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get root folder info")
		}
		rootFolderName = info.Name
	}

	// Create session via state manager
	session, err := e.stateManager.CreateSession(ctx, rootFolderID, rootFolderName, destinationPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create session")
	}

	return session, nil
}

// isResuming checks if this is a resume operation.
func (e *Engine) isResuming() bool {
	return e.currentSession.CompletedFiles > 0 || e.currentSession.TotalFiles > 0
}

// updateSessionTotals updates session total counts.
func (e *Engine) updateSessionTotals(totalFiles, totalBytes int64) {
	e.mu.Lock()
	e.currentSession.TotalFiles = totalFiles
	e.currentSession.TotalBytes = totalBytes
	e.mu.Unlock()

	if err := e.stateManager.UpdateSessionTotals(e.ctx, e.sessionID, totalFiles, totalBytes); err != nil {
		e.logger.Error(err, "Failed to update session totals")
	}
}

// updateFinalStatus updates the final session status.
func (e *Engine) updateFinalStatus(status string) {
	e.mu.Lock()
	e.currentSession.Status = status
	e.currentSession.EndTime = state.NewNullTime(time.Now())
	e.mu.Unlock()

	if err := e.stateManager.UpdateSessionStatus(e.ctx, e.sessionID, status); err != nil {
		e.logger.Error(err, "Failed to update final session status")
	}
}

// handleFatalError handles fatal errors.
func (e *Engine) handleFatalError(err error) {
	e.logger.Error(err, "Fatal error occurred")
	e.updateFinalStatus(state.SessionStatusFailed)
	e.cancel()
}

// getStatus returns the current engine status.
func (e *Engine) getStatus() string {
	if !e.isRunning {
		return "stopped"
	}
	if e.isPaused {
		return "paused"
	}

	// Check if sync is complete
	if e.walkingComplete {
		stats := e.progressTracker.GetStats()
		totalProcessed := stats.CompletedFiles + stats.FailedFiles + stats.SkippedFiles
		if totalProcessed >= stats.TotalFiles && stats.TotalFiles > 0 {
			if e.downloader != nil {
				downloaderStats := e.downloader.GetStats()
				if downloaderStats.ActiveDownloads == 0 && downloaderStats.WorkerPoolStats.QueuedTasks == 0 {
					return "completed"
				}
			}
		}
	}

	return "running"
}

// checkIfSyncComplete checks if the sync is complete and cancels the context if so.
func (e *Engine) checkIfSyncComplete() {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Check if walking is complete
	if !e.walkingComplete {
		return
	}

	// Check if all downloads are complete
	stats := e.progressTracker.GetStats()
	totalProcessed := stats.CompletedFiles + stats.FailedFiles + stats.SkippedFiles

	if totalProcessed >= stats.TotalFiles && stats.TotalFiles > 0 {
		// Check worker pool status
		if e.downloader != nil {
			downloaderStats := e.downloader.GetStats()
			if downloaderStats.ActiveDownloads == 0 && downloaderStats.WorkerPoolStats.QueuedTasks == 0 {
				e.logger.Info("All downloads complete, stopping sync engine",
					"total_files", stats.TotalFiles,
					"completed", stats.CompletedFiles,
					"failed", stats.FailedFiles,
					"skipped", stats.SkippedFiles,
				)
				// Cancel context to trigger shutdown
				if e.cancel != nil {
					e.cancel()
				}
			}
		}
	}
}

// SyncProgress represents the current sync progress.
type SyncProgress struct {
	StartTime       time.Time
	SessionID       string
	Status          string
	SkippedFiles    int64
	RemainingTime   time.Duration
	TotalFiles      int64
	CompletedFiles  int64
	FailedFiles     int64
	ElapsedTime     time.Duration
	TotalBytes      int64
	CompletedBytes  int64
	CurrentSpeed    int64
	AverageSpeed    int64
	FoldersScanned  int64
	ActiveDownloads int64
	QueuedDownloads int
}

// formatBytes formats bytes to human-readable string.
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
