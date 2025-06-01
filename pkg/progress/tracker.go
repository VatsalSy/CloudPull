/**
 * Progress Tracker
 * Core progress tracking functionality for CloudPull sync operations
 *
 * Features:
 * - Thread-safe progress tracking
 * - Real-time file and byte counters
 * - Pause/resume aware state management
 * - Batch update support for performance
 * - Memory-efficient design for millions of files
 *
 * Author: CloudPull Team
 * Update History:
 * - 2025-01-29: Initial implementation
 */

package progress

import (
	"sync"
	"sync/atomic"
	"time"
)

// State represents the current state of a sync operation.
type State int

const (
	StateIdle State = iota
	StateRunning
	StatePaused
	StateCompleted
	StateError
)

// Progress represents the current progress of a sync operation.
type Progress struct {
	startTime      time.Time
	lastFlush      time.Time
	lastPauseTime  time.Time
	currentFile    string
	errors         []error
	pendingUpdates []Update
	state          State
	processedBytes atomic.Int64
	pausedDuration time.Duration
	totalBytes     atomic.Int64
	totalFiles     atomic.Int64
	batchSize      int
	processedFiles atomic.Int64
	mu             sync.RWMutex
	batchMu        sync.Mutex
}

// Update represents a progress update.
type Update struct {
	Timestamp time.Time
	Error     error
	FileName  string
	Type      UpdateType
	Files     int64
	Bytes     int64
}

// UpdateType defines the type of progress update.
type UpdateType int

const (
	UpdateTypeFile UpdateType = iota
	UpdateTypeBytes
	UpdateTypeError
	UpdateTypeState
)

// Tracker manages progress tracking for sync operations.
type Tracker struct {
	progress    *Progress
	done        chan struct{}
	listeners   []chan Update
	wg          sync.WaitGroup
	listenersMu sync.RWMutex
}

// NewTracker creates a new progress tracker.
func NewTracker(batchSize int) *Tracker {
	if batchSize <= 0 {
		batchSize = 100 // Default batch size
	}

	return &Tracker{
		progress: &Progress{
			state:          StateIdle,
			batchSize:      batchSize,
			pendingUpdates: make([]Update, 0, batchSize),
		},
		listeners: make([]chan Update, 0),
		done:      make(chan struct{}),
	}
}

// Start begins tracking progress.
func (t *Tracker) Start() {
	t.progress.mu.Lock()
	defer t.progress.mu.Unlock()

	if t.progress.state != StateIdle {
		return
	}

	t.progress.state = StateRunning
	t.progress.startTime = time.Now()
	t.progress.lastFlush = time.Now()

	// Start batch processor
	t.wg.Add(1)
	go t.processBatches()
}

// Stop stops tracking progress.
func (t *Tracker) Stop() {
	t.progress.mu.Lock()
	t.progress.state = StateCompleted
	t.progress.mu.Unlock()

	close(t.done)
	t.wg.Wait()
}

// Pause pauses progress tracking.
func (t *Tracker) Pause() {
	t.progress.mu.Lock()
	defer t.progress.mu.Unlock()

	if t.progress.state != StateRunning {
		return
	}

	t.progress.state = StatePaused
	t.progress.lastPauseTime = time.Now()
}

// Resume resumes progress tracking.
func (t *Tracker) Resume() {
	t.progress.mu.Lock()
	defer t.progress.mu.Unlock()

	if t.progress.state != StatePaused {
		return
	}

	t.progress.state = StateRunning
	if !t.progress.lastPauseTime.IsZero() {
		t.progress.pausedDuration += time.Since(t.progress.lastPauseTime)
	}
}

// SetTotals sets the total files and bytes to process.
func (t *Tracker) SetTotals(files, bytes int64) {
	t.progress.totalFiles.Store(files)
	t.progress.totalBytes.Store(bytes)
}

// AddFile increments the processed file counter.
func (t *Tracker) AddFile(filename string, bytes int64) {
	t.progress.processedFiles.Add(1)
	t.progress.processedBytes.Add(bytes)

	update := Update{
		Type:      UpdateTypeFile,
		Files:     1,
		Bytes:     bytes,
		FileName:  filename,
		Timestamp: time.Now(),
	}

	t.addUpdate(update)
}

// AddBytes increments the processed bytes counter.
func (t *Tracker) AddBytes(bytes int64) {
	t.progress.processedBytes.Add(bytes)

	update := Update{
		Type:      UpdateTypeBytes,
		Bytes:     bytes,
		Timestamp: time.Now(),
	}

	t.addUpdate(update)
}

// AddError adds an error to the progress.
func (t *Tracker) AddError(err error) {
	t.progress.mu.Lock()
	t.progress.errors = append(t.progress.errors, err)
	t.progress.mu.Unlock()

	update := Update{
		Type:      UpdateTypeError,
		Error:     err,
		Timestamp: time.Now(),
	}

	t.notifyListeners([]Update{update})
}

// GetSnapshot returns a snapshot of the current progress.
func (t *Tracker) GetSnapshot() ProgressSnapshot {
	t.progress.mu.RLock()
	defer t.progress.mu.RUnlock()

	elapsed := t.calculateElapsed()

	return ProgressSnapshot{
		TotalFiles:     t.progress.totalFiles.Load(),
		ProcessedFiles: t.progress.processedFiles.Load(),
		TotalBytes:     t.progress.totalBytes.Load(),
		ProcessedBytes: t.progress.processedBytes.Load(),
		State:          t.progress.state,
		StartTime:      t.progress.startTime,
		ElapsedTime:    elapsed,
		CurrentFile:    t.progress.currentFile,
		ErrorCount:     len(t.progress.errors),
	}
}

// Subscribe creates a new listener channel for progress updates.
func (t *Tracker) Subscribe() <-chan Update {
	t.listenersMu.Lock()
	defer t.listenersMu.Unlock()

	ch := make(chan Update, 100)
	t.listeners = append(t.listeners, ch)
	return ch
}

// Unsubscribe removes a listener channel.
func (t *Tracker) Unsubscribe(ch <-chan Update) {
	t.listenersMu.Lock()
	defer t.listenersMu.Unlock()

	for i, listener := range t.listeners {
		if listener == ch {
			close(listener)
			t.listeners = append(t.listeners[:i], t.listeners[i+1:]...)
			break
		}
	}
}

// addUpdate adds an update to the batch.
func (t *Tracker) addUpdate(update Update) {
	t.progress.batchMu.Lock()
	defer t.progress.batchMu.Unlock()

	t.progress.pendingUpdates = append(t.progress.pendingUpdates, update)

	// Flush if batch is full or time threshold exceeded
	if len(t.progress.pendingUpdates) >= t.progress.batchSize ||
		time.Since(t.progress.lastFlush) > 100*time.Millisecond {

		t.flushUpdates()
	}
}

// flushUpdates sends pending updates to listeners.
func (t *Tracker) flushUpdates() {
	if len(t.progress.pendingUpdates) == 0 {
		return
	}

	updates := make([]Update, len(t.progress.pendingUpdates))
	copy(updates, t.progress.pendingUpdates)
	t.progress.pendingUpdates = t.progress.pendingUpdates[:0]
	t.progress.lastFlush = time.Now()

	go t.notifyListeners(updates)
}

// notifyListeners sends updates to all listeners.
func (t *Tracker) notifyListeners(updates []Update) {
	t.listenersMu.RLock()
	defer t.listenersMu.RUnlock()

	for _, listener := range t.listeners {
		for _, update := range updates {
			select {
			case listener <- update:
			default:
				// Skip if listener is blocked
			}
		}
	}
}

// processBatches handles batch processing in the background.
func (t *Tracker) processBatches() {
	defer t.wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.progress.batchMu.Lock()
			t.flushUpdates()
			t.progress.batchMu.Unlock()
		case <-t.done:
			// Final flush
			t.progress.batchMu.Lock()
			t.flushUpdates()
			t.progress.batchMu.Unlock()
			return
		}
	}
}

// calculateElapsed calculates elapsed time excluding paused duration.
func (t *Tracker) calculateElapsed() time.Duration {
	if t.progress.startTime.IsZero() {
		return 0
	}

	elapsed := time.Since(t.progress.startTime) - t.progress.pausedDuration

	// If currently paused, subtract time since last pause
	if t.progress.state == StatePaused && !t.progress.lastPauseTime.IsZero() {
		elapsed -= time.Since(t.progress.lastPauseTime)
	}

	return elapsed
}

// ProgressSnapshot represents a point-in-time view of progress.
type ProgressSnapshot struct {
	StartTime      time.Time
	CurrentFile    string
	TotalFiles     int64
	ProcessedFiles int64
	TotalBytes     int64
	ProcessedBytes int64
	State          State
	ElapsedTime    time.Duration
	ErrorCount     int
}

// PercentComplete returns the percentage of completion.
func (ps ProgressSnapshot) PercentComplete() float64 {
	if ps.TotalBytes == 0 {
		if ps.TotalFiles == 0 {
			return 0
		}
		return float64(ps.ProcessedFiles) / float64(ps.TotalFiles) * 100
	}
	return float64(ps.ProcessedBytes) / float64(ps.TotalBytes) * 100
}

// BytesPerSecond returns the average transfer speed.
func (ps ProgressSnapshot) BytesPerSecond() float64 {
	if ps.ElapsedTime == 0 {
		return 0
	}
	return float64(ps.ProcessedBytes) / ps.ElapsedTime.Seconds()
}

// ETA returns the estimated time to completion.
func (ps ProgressSnapshot) ETA() time.Duration {
	if ps.ProcessedBytes == 0 || ps.ElapsedTime == 0 {
		return 0
	}

	bytesPerSecond := ps.BytesPerSecond()
	if bytesPerSecond == 0 {
		return 0
	}

	remainingBytes := ps.TotalBytes - ps.ProcessedBytes
	return time.Duration(float64(remainingBytes)/bytesPerSecond) * time.Second
}
