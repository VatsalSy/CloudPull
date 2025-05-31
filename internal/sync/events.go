/**
 * Sync Events
 * Event system for progress updates during sync operations
 *
 * Features:
 * - Type-safe event definitions
 * - Event bus for decoupled communication
 * - Buffered channels for non-blocking events
 * - Priority-based event handling
 * - Event filtering and routing
 *
 * Author: CloudPull Team
 * Update History:
 * - 2025-01-29: Initial implementation
 */

package sync

import (
	"context"
	"fmt"
	"sync"
	"time"
	
	"github.com/VatsalSy/CloudPull/internal/logger"
)

// EventType defines the type of sync event.
type EventType int

const (
	EventTypeFileStart EventType = iota
	EventTypeFileProgress
	EventTypeFileComplete
	EventTypeFileError
	EventTypeFolderStart
	EventTypeFolderComplete
	EventTypeSyncStart
	EventTypeSyncComplete
	EventTypeSyncPaused
	EventTypeSyncResumed
	EventTypeRateLimit
	EventTypeRetry
)

// EventPriority defines the priority of an event.
type EventPriority int

const (
	EventPriorityLow EventPriority = iota
	EventPriorityNormal
	EventPriorityHigh
	EventPriorityCritical
)

// Event represents a sync event.
type Event struct {
	Timestamp time.Time
	Data      interface{}
	Type      EventType
	Priority  EventPriority
}

// FileEvent contains data for file-related events.
type FileEvent struct {
	Error            error
	Metadata         map[string]interface{}
	FileID           string
	FileName         string
	FilePath         string
	FileSize         int64
	BytesTransferred int64
}

// FolderEvent contains data for folder-related events.
type FolderEvent struct {
	FolderID   string
	FolderName string
	FolderPath string
	FileCount  int
	TotalSize  int64
}

// SyncEvent contains data for sync-related events.
type SyncEvent struct {
	StartTime      time.Time
	SessionID      string
	Message        string
	TotalFiles     int64
	TotalBytes     int64
	ProcessedFiles int64
	ProcessedBytes int64
}

// RetryEvent contains data for retry events.
type RetryEvent struct {
	NextRetryAt time.Time
	Error       error
	FileID      string
	FileName    string
	RetryCount  int
	MaxRetries  int
}

// EventHandler processes events.
type EventHandler func(event Event)

// EventFilter determines if an event should be processed.
type EventFilter func(event Event) bool

// EventBus manages event distribution.
type EventBus struct {
	ctx            context.Context
	handlers       map[EventType][]HandlerInfo
	channels       map[string]chan Event
	cancel         context.CancelFunc
	globalHandlers []HandlerInfo
	wg             sync.WaitGroup
	bufferSize     int
	mu             sync.RWMutex
	logger         *logger.Logger
}

// HandlerInfo contains handler metadata.
type HandlerInfo struct {
	Handler  EventHandler
	Filter   EventFilter
	Priority EventPriority
}

// NewEventBus creates a new event bus.
func NewEventBus(bufferSize int) *EventBus {
	if bufferSize <= 0 {
		bufferSize = 1000
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &EventBus{
		handlers:       make(map[EventType][]HandlerInfo),
		globalHandlers: make([]HandlerInfo, 0),
		channels:       make(map[string]chan Event),
		bufferSize:     bufferSize,
		ctx:            ctx,
		cancel:         cancel,
		logger:         logger.Global(),
	}
}

// Subscribe adds a handler for specific event types.
func (eb *EventBus) Subscribe(eventType EventType, handler EventHandler,
	filter EventFilter, priority EventPriority) {

	eb.mu.Lock()
	defer eb.mu.Unlock()

	info := HandlerInfo{
		Handler:  handler,
		Filter:   filter,
		Priority: priority,
	}

	eb.handlers[eventType] = append(eb.handlers[eventType], info)

	// Sort handlers by priority
	eb.sortHandlers(eventType)
}

// SubscribeAll adds a global handler for all events.
func (eb *EventBus) SubscribeAll(handler EventHandler, filter EventFilter,
	priority EventPriority) {

	eb.mu.Lock()
	defer eb.mu.Unlock()

	info := HandlerInfo{
		Handler:  handler,
		Filter:   filter,
		Priority: priority,
	}

	eb.globalHandlers = append(eb.globalHandlers, info)

	// Sort global handlers by priority
	eb.sortGlobalHandlers()
}

// CreateChannel creates a named channel for event streaming.
func (eb *EventBus) CreateChannel(name string) <-chan Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if _, exists := eb.channels[name]; exists {
		return eb.channels[name]
	}

	ch := make(chan Event, eb.bufferSize)
	eb.channels[name] = ch

	return ch
}

// CloseChannel closes a named channel.
func (eb *EventBus) CloseChannel(name string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if ch, exists := eb.channels[name]; exists {
		close(ch)
		delete(eb.channels, name)
	}
}

// Publish publishes an event to all subscribers.
func (eb *EventBus) Publish(event Event) {
	event.Timestamp = time.Now()

	// Send to channels
	eb.sendToChannels(event)

	// Process handlers asynchronously
	eb.wg.Add(1)
	go func() {
		defer eb.wg.Done()
		eb.processHandlers(event)
	}()
}

// PublishFileStart publishes a file start event.
func (eb *EventBus) PublishFileStart(fileID, fileName, filePath string,
	fileSize int64) {

	eb.Publish(Event{
		Type:     EventTypeFileStart,
		Priority: EventPriorityNormal,
		Data: FileEvent{
			FileID:   fileID,
			FileName: fileName,
			FilePath: filePath,
			FileSize: fileSize,
		},
	})
}

// PublishFileProgress publishes a file progress event.
func (eb *EventBus) PublishFileProgress(fileID string, bytesTransferred int64) {
	eb.Publish(Event{
		Type:     EventTypeFileProgress,
		Priority: EventPriorityLow,
		Data: FileEvent{
			FileID:           fileID,
			BytesTransferred: bytesTransferred,
		},
	})
}

// PublishFileComplete publishes a file completion event.
func (eb *EventBus) PublishFileComplete(fileID, fileName string,
	bytesTransferred int64) {

	eb.Publish(Event{
		Type:     EventTypeFileComplete,
		Priority: EventPriorityNormal,
		Data: FileEvent{
			FileID:           fileID,
			FileName:         fileName,
			BytesTransferred: bytesTransferred,
		},
	})
}

// PublishFileError publishes a file error event.
func (eb *EventBus) PublishFileError(fileID, fileName string, err error) {
	eb.Publish(Event{
		Type:     EventTypeFileError,
		Priority: EventPriorityHigh,
		Data: FileEvent{
			FileID:   fileID,
			FileName: fileName,
			Error:    err,
		},
	})
}

// PublishSyncStart publishes a sync start event.
func (eb *EventBus) PublishSyncStart(sessionID string, totalFiles,
	totalBytes int64) {

	eb.Publish(Event{
		Type:     EventTypeSyncStart,
		Priority: EventPriorityHigh,
		Data: SyncEvent{
			SessionID:  sessionID,
			TotalFiles: totalFiles,
			TotalBytes: totalBytes,
			StartTime:  time.Now(),
		},
	})
}

// PublishSyncComplete publishes a sync completion event.
func (eb *EventBus) PublishSyncComplete(sessionID string, processedFiles,
	processedBytes int64) {

	eb.Publish(Event{
		Type:     EventTypeSyncComplete,
		Priority: EventPriorityHigh,
		Data: SyncEvent{
			SessionID:      sessionID,
			ProcessedFiles: processedFiles,
			ProcessedBytes: processedBytes,
		},
	})
}

// Close shuts down the event bus.
func (eb *EventBus) Close() {
	eb.cancel()
	eb.wg.Wait()

	eb.mu.Lock()
	defer eb.mu.Unlock()

	// Close all channels
	for name, ch := range eb.channels {
		close(ch)
		delete(eb.channels, name)
	}
}

// sendToChannels sends event to all registered channels.
func (eb *EventBus) sendToChannels(event Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	for _, ch := range eb.channels {
		select {
		case ch <- event:
		default:
			// Channel full, skip to avoid blocking
		}
	}
}

// processHandlers processes event handlers.
func (eb *EventBus) processHandlers(event Event) {
	// Copy handlers to avoid holding lock during execution
	var handlersToCall []HandlerInfo
	
	eb.mu.RLock()
	// Process type-specific handlers
	if handlers, exists := eb.handlers[event.Type]; exists {
		for _, info := range handlers {
			if info.Filter == nil || info.Filter(event) {
				// Make a copy to avoid race conditions
				handlersToCall = append(handlersToCall, info)
			}
		}
	}

	// Process global handlers
	for _, info := range eb.globalHandlers {
		if info.Filter == nil || info.Filter(event) {
			// Make a copy to avoid race conditions
			handlersToCall = append(handlersToCall, info)
		}
	}
	eb.mu.RUnlock()
	
	// Call handlers without holding the lock
	for _, info := range handlersToCall {
		eb.callHandler(info.Handler, event)
	}
}

// callHandler safely calls a handler.
func (eb *EventBus) callHandler(handler EventHandler, event Event) {
	defer func() {
		if r := recover(); r != nil {
			// Log panic but don't crash
			if eb.logger != nil {
				eb.logger.Error(fmt.Errorf("panic in event handler: %v", r), 
					"Event handler panicked",
					"event_type", event.Type.String(),
					"panic", r,
				)
			}
		}
	}()

	handler(event)
}

// sortHandlers sorts handlers by priority (higher priority first).
func (eb *EventBus) sortHandlers(eventType EventType) {
	handlers := eb.handlers[eventType]
	for i := 0; i < len(handlers)-1; i++ {
		for j := i + 1; j < len(handlers); j++ {
			if handlers[j].Priority > handlers[i].Priority {
				handlers[i], handlers[j] = handlers[j], handlers[i]
			}
		}
	}
}

// sortGlobalHandlers sorts global handlers by priority.
func (eb *EventBus) sortGlobalHandlers() {
	for i := 0; i < len(eb.globalHandlers)-1; i++ {
		for j := i + 1; j < len(eb.globalHandlers); j++ {
			if eb.globalHandlers[j].Priority > eb.globalHandlers[i].Priority {
				eb.globalHandlers[i], eb.globalHandlers[j] =
					eb.globalHandlers[j], eb.globalHandlers[i]
			}
		}
	}
}

// String returns string representation of event type.
func (et EventType) String() string {
	switch et {
	case EventTypeFileStart:
		return "file_start"
	case EventTypeFileProgress:
		return "file_progress"
	case EventTypeFileComplete:
		return "file_complete"
	case EventTypeFileError:
		return "file_error"
	case EventTypeFolderStart:
		return "folder_start"
	case EventTypeFolderComplete:
		return "folder_complete"
	case EventTypeSyncStart:
		return "sync_start"
	case EventTypeSyncComplete:
		return "sync_complete"
	case EventTypeSyncPaused:
		return "sync_paused"
	case EventTypeSyncResumed:
		return "sync_resumed"
	case EventTypeRateLimit:
		return "rate_limit"
	case EventTypeRetry:
		return "retry"
	default:
		return "unknown"
	}
}
