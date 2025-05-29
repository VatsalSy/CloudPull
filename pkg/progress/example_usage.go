/**
 * Progress System Example Usage
 * Demonstrates how to integrate the progress tracking system
 * 
 * Author: CloudPull Team
 * Update History:
 * - 2025-01-29: Initial implementation
 */

package progress

import (
  "context"
  "fmt"
  "log"
  "time"
  
  "github.com/VatsalSy/CloudPull/internal/sync"
)

// Example demonstrates how to use the progress tracking system
func Example() {
  // Create progress tracker with batch size of 100
  tracker := NewTracker(100)
  
  // Create metrics collector with 30s window
  metrics := NewMetricsCollector(30*time.Second, 100*time.Millisecond)
  
  // Create reporter for terminal output
  reporter := NewReporter(tracker, ReporterConfig{
    Format:    OutputFormatTerminal,
    ShowETA:   true,
    ShowSpeed: true,
  })
  
  // Start tracking
  tracker.Start()
  reporter.Start()
  defer func() {
    tracker.Stop()
    reporter.Stop()
  }()
  
  // Set totals (usually from file scanning)
  tracker.SetTotals(1000, 1024*1024*1024) // 1000 files, 1GB total
  
  // Simulate file sync
  for i := 0; i < 100; i++ {
    filename := fmt.Sprintf("file_%d.txt", i)
    fileSize := int64(1024 * 1024 * 10) // 10MB
    
    // Start file
    tracker.AddFile(filename, 0)
    metrics.StartFile(filename, fileSize)
    
    // Simulate progress
    for transferred := int64(0); transferred < fileSize; {
      chunk := int64(1024 * 1024) // 1MB chunks
      if transferred+chunk > fileSize {
        chunk = fileSize - transferred
      }
      
      tracker.AddBytes(chunk)
      metrics.UpdateFile(filename, transferred+chunk)
      metrics.AddSample(chunk, 0)
      
      transferred += chunk
      time.Sleep(10 * time.Millisecond) // Simulate transfer time
    }
    
    // Complete file
    tracker.AddFile(filename, fileSize)
    metrics.CompleteFile(filename)
  }
  
  // Get final stats
  snapshot := tracker.GetSnapshot()
  stats := metrics.GetStats()
  
  fmt.Printf("\nSync completed:\n")
  fmt.Printf("- Files: %d/%d\n", snapshot.ProcessedFiles, snapshot.TotalFiles)
  fmt.Printf("- Data: %.2f GB\n", float64(snapshot.ProcessedBytes)/(1024*1024*1024))
  fmt.Printf("- Average speed: %.2f MB/s\n", stats.AverageSpeed/(1024*1024))
  fmt.Printf("- Duration: %v\n", snapshot.ElapsedTime)
}

// ExampleWithEvents demonstrates integration with the event system
func ExampleWithEvents() {
  // Create event bus
  eventBus := sync.NewEventBus(1000)
  defer eventBus.Close()
  
  // Create progress components
  tracker := NewTracker(100)
  metrics := NewMetricsCollector(30*time.Second, 100*time.Millisecond)
  
  // Subscribe to sync events
  eventBus.Subscribe(sync.EventTypeFileStart, func(event sync.Event) {
    if fileEvent, ok := event.Data.(sync.FileEvent); ok {
      tracker.AddFile(fileEvent.FileName, 0)
      metrics.StartFile(fileEvent.FileID, fileEvent.FileSize)
    }
  }, nil, sync.EventPriorityNormal)
  
  eventBus.Subscribe(sync.EventTypeFileProgress, func(event sync.Event) {
    if fileEvent, ok := event.Data.(sync.FileEvent); ok {
      tracker.AddBytes(fileEvent.BytesTransferred)
      metrics.UpdateFile(fileEvent.FileID, fileEvent.BytesTransferred)
    }
  }, nil, sync.EventPriorityNormal)
  
  eventBus.Subscribe(sync.EventTypeFileComplete, func(event sync.Event) {
    if fileEvent, ok := event.Data.(sync.FileEvent); ok {
      tracker.AddFile(fileEvent.FileName, fileEvent.BytesTransferred)
      metrics.CompleteFile(fileEvent.FileID)
    }
  }, nil, sync.EventPriorityNormal)
  
  eventBus.Subscribe(sync.EventTypeFileError, func(event sync.Event) {
    if fileEvent, ok := event.Data.(sync.FileEvent); ok {
      tracker.AddError(fileEvent.Error)
      metrics.ErrorFile(fileEvent.FileID, fileEvent.Error)
    }
  }, nil, sync.EventPriorityHigh)
  
  // Start sync operation
  sessionID := "sync_12345"
  eventBus.PublishSyncStart(sessionID, 100, 1024*1024*100)
  
  tracker.Start()
  tracker.SetTotals(100, 1024*1024*100)
  
  // Simulate file transfers
  ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
  defer cancel()
  
  for i := 0; i < 10; i++ {
    select {
    case <-ctx.Done():
      return
    default:
      fileID := fmt.Sprintf("file_%d", i)
      fileName := fmt.Sprintf("document_%d.pdf", i)
      fileSize := int64(1024 * 1024 * 10) // 10MB
      
      // Publish file start
      eventBus.PublishFileStart(fileID, fileName, "/path/to/file", fileSize)
      
      // Simulate progress
      for progress := int64(0); progress < fileSize; progress += 1024*1024 {
        eventBus.PublishFileProgress(fileID, progress)
        time.Sleep(50 * time.Millisecond)
      }
      
      // Complete file
      eventBus.PublishFileComplete(fileID, fileName, fileSize)
    }
  }
  
  // Complete sync
  snapshot := tracker.GetSnapshot()
  eventBus.PublishSyncComplete(sessionID, snapshot.ProcessedFiles, 
    snapshot.ProcessedBytes)
  
  tracker.Stop()
}

// ExampleJSONReporter demonstrates JSON output for programmatic consumption
func ExampleJSONReporter() {
  tracker := NewTracker(100)
  
  // Create JSON reporter
  reporter := NewReporter(tracker, ReporterConfig{
    Format: OutputFormatJSON,
  })
  
  tracker.Start()
  reporter.Start()
  
  // Simulate some progress
  tracker.SetTotals(10, 1024*1024*100)
  
  for i := 0; i < 5; i++ {
    tracker.AddFile(fmt.Sprintf("file%d.txt", i), 1024*1024*10)
    time.Sleep(100 * time.Millisecond)
  }
  
  // Output will be JSON lines like:
  // {"timestamp":1234567890,"state":"running","total_files":10,...}
  // {"timestamp":1234567891,"state":"running","total_files":10,...}
  
  tracker.Stop()
  reporter.Stop()
}

// ExamplePauseResume demonstrates pause/resume functionality
func ExamplePauseResume() {
  tracker := NewTracker(100)
  metrics := NewMetricsCollector(30*time.Second, 100*time.Millisecond)
  
  tracker.Start()
  
  // Process some files
  for i := 0; i < 5; i++ {
    tracker.AddFile(fmt.Sprintf("file%d.txt", i), 1024*1024)
    metrics.AddSample(1024*1024, 1)
    time.Sleep(100 * time.Millisecond)
  }
  
  // Pause operation
  log.Println("Pausing sync...")
  tracker.Pause()
  
  // Simulate pause duration
  time.Sleep(2 * time.Second)
  
  // Resume operation
  log.Println("Resuming sync...")
  tracker.Resume()
  
  // Continue processing
  for i := 5; i < 10; i++ {
    tracker.AddFile(fmt.Sprintf("file%d.txt", i), 1024*1024)
    metrics.AddSample(1024*1024, 1)
    time.Sleep(100 * time.Millisecond)
  }
  
  tracker.Stop()
  
  // Get final snapshot
  snapshot := tracker.GetSnapshot()
  log.Printf("Total elapsed time (excluding pause): %v\n", snapshot.ElapsedTime)
}

// ExampleCustomReporter demonstrates creating a custom reporter
func ExampleCustomReporter() {
  tracker := NewTracker(100)
  
  // Subscribe to updates
  updates := tracker.Subscribe()
  defer tracker.Unsubscribe(updates)
  
  // Start tracking
  tracker.Start()
  defer tracker.Stop()
  
  // Custom reporter goroutine
  go func() {
    for update := range updates {
      switch update.Type {
      case UpdateTypeFile:
        log.Printf("File completed: %s (%d bytes)\n", 
          update.FileName, update.Bytes)
      case UpdateTypeError:
        log.Printf("Error: %v\n", update.Error)
      }
    }
  }()
  
  // Simulate file operations
  tracker.SetTotals(5, 1024*1024*50)
  
  for i := 0; i < 5; i++ {
    filename := fmt.Sprintf("custom_file_%d.dat", i)
    tracker.AddFile(filename, 1024*1024*10)
    time.Sleep(200 * time.Millisecond)
  }
}