/**
 * Progress Tracking and Reporting for CloudPull Sync Engine
 *
 * Features:
 * - Real-time progress tracking for sync operations
 * - Per-file and overall session progress
 * - Event emission for UI updates
 * - Bandwidth calculation and throttling stats
 * - ETA estimation
 *
 * Author: CloudPull Team
 * Updated: 2025-01-29
 */

package sync

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// ProgressEventType defines types of progress events.
type ProgressEventType string

const (
	// Progress event types as strings for better readability
	ProgressEventFileStarted     ProgressEventType = "file_started"
	ProgressEventFileProgress    ProgressEventType = "file_progress"
	ProgressEventFileCompleted   ProgressEventType = "file_completed"
	ProgressEventFileFailed      ProgressEventType = "file_failed"
	ProgressEventFolderStarted   ProgressEventType = "folder_started"
	ProgressEventFolderCompleted ProgressEventType = "folder_completed"
	ProgressEventSessionUpdate   ProgressEventType = "session_update"
	ProgressEventBandwidthUpdate ProgressEventType = "bandwidth_update"
)

// ProgressEvent represents a progress update event.
type ProgressEvent struct {
	Timestamp        time.Time
	Error            error
	Context          map[string]interface{}
	SessionID        string
	ItemID           string
	ItemName         string
	ItemPath         string
	ErrorMessage     string
	Type             ProgressEventType
	FilesCompleted   int64
	CurrentSpeed     int64
	AverageSpeed     int64
	RemainingTime    time.Duration
	TotalFiles       int64
	TotalBytes       int64
	BytesTransferred int64
}

// ProgressTracker tracks sync progress and emits events.
type ProgressTracker struct {
	lastUpdate      time.Time
	startTime       time.Time
	periodStart     time.Time
	activeDownloads map[string]*FileProgress
	sessionID       string
	eventHandlers   []func(event *ProgressEvent)
	speedSamples    []int64
	totalFiles      int64
	skippedFiles    int64
	failedFiles     int64
	lastBytes       int64
	currentSpeed    int64
	completedFiles  int64
	maxSpeedSamples int
	completedBytes  int64
	bandwidthLimit  int64
	bytesThisPeriod int64
	totalBytes      int64
	mu              sync.RWMutex
}

// FileProgress tracks individual file download progress.
type FileProgress struct {
	StartTime       time.Time
	LastUpdate      time.Time
	FileID          string
	FileName        string
	FilePath        string
	TotalBytes      int64
	BytesDownloaded int64
	Speed           int64
}

// NewProgressTracker creates a new progress tracker.
func NewProgressTracker(sessionID string) *ProgressTracker {
	return &ProgressTracker{
		sessionID:       sessionID,
		startTime:       time.Now(),
		lastUpdate:      time.Now(),
		activeDownloads: make(map[string]*FileProgress),
		speedSamples:    make([]int64, 0, 10),
		maxSpeedSamples: 10,
		periodStart:     time.Now(),
	}
}

// SetTotals sets the total files and bytes for the session.
func (pt *ProgressTracker) SetTotals(totalFiles, totalBytes int64) {
	pt.mu.Lock()
	pt.totalFiles = totalFiles
	pt.totalBytes = totalBytes
	pt.mu.Unlock()

	pt.emitSessionUpdate()
}

// SetBandwidthLimit sets the bandwidth limit in bytes per second.
func (pt *ProgressTracker) SetBandwidthLimit(bytesPerSecond int64) {
	pt.mu.Lock()
	pt.bandwidthLimit = bytesPerSecond
	pt.mu.Unlock()
}

// OnEvent registers an event handler.
func (pt *ProgressTracker) OnEvent(handler func(event *ProgressEvent)) {
	pt.mu.Lock()
	pt.eventHandlers = append(pt.eventHandlers, handler)
	pt.mu.Unlock()
}

// FileStarted notifies that a file download has started.
func (pt *ProgressTracker) FileStarted(fileID, fileName, filePath string, totalBytes int64) {
	pt.mu.Lock()
	pt.activeDownloads[fileID] = &FileProgress{
		FileID:     fileID,
		FileName:   fileName,
		FilePath:   filePath,
		TotalBytes: totalBytes,
		StartTime:  time.Now(),
		LastUpdate: time.Now(),
	}
	pt.mu.Unlock()

	pt.emit(&ProgressEvent{
		Type:       ProgressEventFileStarted,
		Timestamp:  time.Now(),
		SessionID:  pt.sessionID,
		ItemID:     fileID,
		ItemName:   fileName,
		ItemPath:   filePath,
		TotalBytes: totalBytes,
	})
}

// FileProgress updates file download progress.
func (pt *ProgressTracker) FileProgress(fileID string, bytesDownloaded int64) {
	pt.mu.Lock()
	fp, exists := pt.activeDownloads[fileID]
	if !exists {
		pt.mu.Unlock()
		return
	}

	// Calculate speed for this file
	now := time.Now()
	deltaTime := now.Sub(fp.LastUpdate).Seconds()
	deltaBytes := bytesDownloaded - fp.BytesDownloaded

	if deltaTime > 0 {
		fp.Speed = int64(float64(deltaBytes) / deltaTime)
	}

	fp.BytesDownloaded = bytesDownloaded
	fp.LastUpdate = now

	// Update session totals
	atomic.AddInt64(&pt.completedBytes, deltaBytes)

	pt.mu.Unlock()

	// Update speed tracking
	pt.updateSpeed(deltaBytes)

	pt.emit(&ProgressEvent{
		Type:             ProgressEventFileProgress,
		Timestamp:        now,
		SessionID:        pt.sessionID,
		ItemID:           fileID,
		ItemName:         fp.FileName,
		ItemPath:         fp.FilePath,
		BytesTransferred: bytesDownloaded,
		TotalBytes:       fp.TotalBytes,
		CurrentSpeed:     fp.Speed,
	})

	// Emit session update periodically
	if now.Sub(pt.lastUpdate) > time.Second {
		pt.emitSessionUpdate()
	}
}

// FileCompleted notifies that a file download completed.
func (pt *ProgressTracker) FileCompleted(fileID string) {
	pt.mu.Lock()
	fp, exists := pt.activeDownloads[fileID]
	if !exists {
		pt.mu.Unlock()
		return
	}

	delete(pt.activeDownloads, fileID)
	atomic.AddInt64(&pt.completedFiles, 1)
	fileName := fp.FileName
	filePath := fp.FilePath
	totalBytes := fp.TotalBytes
	pt.mu.Unlock()

	pt.emit(&ProgressEvent{
		Type:             ProgressEventFileCompleted,
		Timestamp:        time.Now(),
		SessionID:        pt.sessionID,
		ItemID:           fileID,
		ItemName:         fileName,
		ItemPath:         filePath,
		BytesTransferred: totalBytes,
		TotalBytes:       totalBytes,
	})

	pt.emitSessionUpdate()
}

// FileFailed notifies that a file download failed.
func (pt *ProgressTracker) FileFailed(fileID string, err error) {
	pt.mu.Lock()
	fp, exists := pt.activeDownloads[fileID]
	if exists {
		delete(pt.activeDownloads, fileID)
	}
	atomic.AddInt64(&pt.failedFiles, 1)

	fileName := ""
	filePath := ""
	if fp != nil {
		fileName = fp.FileName
		filePath = fp.FilePath
	}
	pt.mu.Unlock()

	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}

	pt.emit(&ProgressEvent{
		Type:         ProgressEventFileFailed,
		Timestamp:    time.Now(),
		SessionID:    pt.sessionID,
		ItemID:       fileID,
		ItemName:     fileName,
		ItemPath:     filePath,
		Error:        err,
		ErrorMessage: errorMsg,
	})

	pt.emitSessionUpdate()
}

// FileSkipped notifies that a file was skipped.
func (pt *ProgressTracker) FileSkipped(fileID, fileName, filePath string, reason string) {
	atomic.AddInt64(&pt.skippedFiles, 1)

	pt.emit(&ProgressEvent{
		Type:      ProgressEventFileCompleted,
		Timestamp: time.Now(),
		SessionID: pt.sessionID,
		ItemID:    fileID,
		ItemName:  fileName,
		ItemPath:  filePath,
		Context: map[string]interface{}{
			"skipped": true,
			"reason":  reason,
		},
	})

	pt.emitSessionUpdate()
}

// FolderStarted notifies that folder scanning started.
func (pt *ProgressTracker) FolderStarted(folderID, folderName, folderPath string) {
	pt.emit(&ProgressEvent{
		Type:      ProgressEventFolderStarted,
		Timestamp: time.Now(),
		SessionID: pt.sessionID,
		ItemID:    folderID,
		ItemName:  folderName,
		ItemPath:  folderPath,
	})
}

// FolderCompleted notifies that folder scanning completed.
func (pt *ProgressTracker) FolderCompleted(folderID, folderName, folderPath string, fileCount int64) {
	pt.emit(&ProgressEvent{
		Type:      ProgressEventFolderCompleted,
		Timestamp: time.Now(),
		SessionID: pt.sessionID,
		ItemID:    folderID,
		ItemName:  folderName,
		ItemPath:  folderPath,
		Context: map[string]interface{}{
			"file_count": fileCount,
		},
	})
}

// GetStats returns current progress statistics.
func (pt *ProgressTracker) GetStats() *ProgressStats {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	elapsed := time.Since(pt.startTime)
	remaining := pt.calculateRemainingTime()

	return &ProgressStats{
		SessionID:       pt.sessionID,
		StartTime:       pt.startTime,
		ElapsedTime:     elapsed,
		RemainingTime:   remaining,
		TotalFiles:      pt.totalFiles,
		CompletedFiles:  pt.completedFiles,
		FailedFiles:     pt.failedFiles,
		SkippedFiles:    pt.skippedFiles,
		TotalBytes:      pt.totalBytes,
		CompletedBytes:  pt.completedBytes,
		CurrentSpeed:    pt.currentSpeed,
		AverageSpeed:    pt.calculateAverageSpeed(),
		ActiveDownloads: len(pt.activeDownloads),
		BandwidthLimit:  pt.bandwidthLimit,
	}
}

// CheckBandwidthLimit checks if we're within bandwidth limits.
func (pt *ProgressTracker) CheckBandwidthLimit(ctx context.Context, bytesRequested int64) error {
	if pt.bandwidthLimit <= 0 {
		return nil // No limit
	}

	pt.mu.Lock()
	defer pt.mu.Unlock()

	now := time.Now()
	periodDuration := now.Sub(pt.periodStart)

	// Reset period if it's been more than a second
	if periodDuration >= time.Second {
		pt.bytesThisPeriod = 0
		pt.periodStart = now
		periodDuration = 0
	}

	// Check if adding these bytes would exceed the limit
	if pt.bytesThisPeriod+bytesRequested > pt.bandwidthLimit {
		// Calculate wait time
		remainingPeriod := time.Second - periodDuration

		// Wait for the next period
		timer := time.NewTimer(remainingPeriod)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			pt.bytesThisPeriod = bytesRequested
			pt.periodStart = time.Now()
		}
	} else {
		pt.bytesThisPeriod += bytesRequested
	}

	return nil
}

// updateSpeed updates the current speed calculation.
func (pt *ProgressTracker) updateSpeed(deltaBytes int64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	now := time.Now()
	deltaTime := now.Sub(pt.lastUpdate).Seconds()

	if deltaTime > 0 {
		speed := int64(float64(deltaBytes) / deltaTime)

		// Add to samples
		pt.speedSamples = append(pt.speedSamples, speed)
		if len(pt.speedSamples) > pt.maxSpeedSamples {
			pt.speedSamples = pt.speedSamples[1:]
		}

		// Calculate current speed as average of recent samples
		if len(pt.speedSamples) > 0 {
			sum := int64(0)
			for _, s := range pt.speedSamples {
				sum += s
			}
			pt.currentSpeed = sum / int64(len(pt.speedSamples))
		}
	}

	pt.lastUpdate = now
	pt.lastBytes += deltaBytes
}

// calculateAverageSpeed calculates the average speed since start.
func (pt *ProgressTracker) calculateAverageSpeed() int64 {
	elapsed := time.Since(pt.startTime).Seconds()
	if elapsed > 0 {
		return int64(float64(pt.completedBytes) / elapsed)
	}
	return 0
}

// calculateRemainingTime estimates time remaining.
func (pt *ProgressTracker) calculateRemainingTime() time.Duration {
	if pt.currentSpeed <= 0 || pt.completedBytes >= pt.totalBytes {
		return 0
	}

	remainingBytes := pt.totalBytes - pt.completedBytes
	seconds := float64(remainingBytes) / float64(pt.currentSpeed)
	return time.Duration(seconds) * time.Second
}

// emitSessionUpdate emits a session progress update.
func (pt *ProgressTracker) emitSessionUpdate() {
	stats := pt.GetStats()

	pt.emit(&ProgressEvent{
		Type:             ProgressEventSessionUpdate,
		Timestamp:        time.Now(),
		SessionID:        pt.sessionID,
		FilesCompleted:   stats.CompletedFiles,
		TotalFiles:       stats.TotalFiles,
		BytesTransferred: stats.CompletedBytes,
		TotalBytes:       stats.TotalBytes,
		CurrentSpeed:     stats.CurrentSpeed,
		AverageSpeed:     stats.AverageSpeed,
		RemainingTime:    stats.RemainingTime,
		Context: map[string]interface{}{
			"failed_files":     stats.FailedFiles,
			"skipped_files":    stats.SkippedFiles,
			"active_downloads": stats.ActiveDownloads,
		},
	})
}

// emit sends an event to all registered handlers.
func (pt *ProgressTracker) emit(event *ProgressEvent) {
	pt.mu.RLock()
	handlers := make([]func(*ProgressEvent), len(pt.eventHandlers))
	copy(handlers, pt.eventHandlers)
	pt.mu.RUnlock()

	for _, handler := range handlers {
		// Call handlers in goroutines to prevent blocking
		go handler(event)
	}
}

// ProgressStats contains current progress statistics.
type ProgressStats struct {
	StartTime       time.Time
	SessionID       string
	FailedFiles     int64
	RemainingTime   time.Duration
	TotalFiles      int64
	CompletedFiles  int64
	ElapsedTime     time.Duration
	SkippedFiles    int64
	TotalBytes      int64
	CompletedBytes  int64
	CurrentSpeed    int64
	AverageSpeed    int64
	ActiveDownloads int
	BandwidthLimit  int64
}

// Progress returns completion percentage.
func (ps *ProgressStats) Progress() float64 {
	if ps.TotalFiles == 0 {
		return 0
	}
	return float64(ps.CompletedFiles) / float64(ps.TotalFiles) * 100
}

// BytesProgress returns bytes completion percentage.
func (ps *ProgressStats) BytesProgress() float64 {
	if ps.TotalBytes == 0 {
		return 0
	}
	return float64(ps.CompletedBytes) / float64(ps.TotalBytes) * 100
}
