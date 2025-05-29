/**
 * Progress Reporter
 * Provides different output formats for progress reporting
 * 
 * Features:
 * - Terminal output with progress bars
 * - JSON output for programmatic consumption
 * - Quiet mode for minimal output
 * - Human-readable formatting
 * - Adaptive refresh rates
 * 
 * Author: CloudPull Team
 * Update History:
 * - 2025-01-29: Initial implementation
 */

package progress

import (
  "encoding/json"
  "fmt"
  "io"
  "os"
  "strings"
  "sync"
  "time"
  
  "github.com/schollz/progressbar/v3"
)

// OutputFormat defines the output format for progress reporting
type OutputFormat string

const (
  OutputFormatTerminal OutputFormat = "terminal"
  OutputFormatJSON     OutputFormat = "json"
  OutputFormatQuiet    OutputFormat = "quiet"
)

// Reporter handles progress reporting in various formats
type Reporter struct {
  format       OutputFormat
  output       io.Writer
  tracker      *Tracker
  progressBar  *progressbar.ProgressBar
  lastUpdate   time.Time
  updateMu     sync.Mutex
  done         chan struct{}
  wg           sync.WaitGroup
}

// ReporterConfig configures a progress reporter
type ReporterConfig struct {
  Format       OutputFormat
  Output       io.Writer
  RefreshRate  time.Duration
  ShowETA      bool
  ShowSpeed    bool
}

// NewReporter creates a new progress reporter
func NewReporter(tracker *Tracker, config ReporterConfig) *Reporter {
  if config.Output == nil {
    config.Output = os.Stdout
  }
  if config.RefreshRate == 0 {
    config.RefreshRate = 100 * time.Millisecond
  }
  
  r := &Reporter{
    format:  config.Format,
    output:  config.Output,
    tracker: tracker,
    done:    make(chan struct{}),
  }
  
  // Initialize progress bar for terminal format
  if config.Format == OutputFormatTerminal {
    r.progressBar = progressbar.NewOptions64(
      100,
      progressbar.OptionSetWriter(config.Output),
      progressbar.OptionEnableColorCodes(true),
      progressbar.OptionShowBytes(true),
      progressbar.OptionSetWidth(15),
      progressbar.OptionSetDescription("[cyan]Syncing files...[reset]"),
      progressbar.OptionSetTheme(progressbar.Theme{
        Saucer:        "[green]=[reset]",
        SaucerHead:    "[green]>[reset]",
        SaucerPadding: " ",
        BarStart:      "[",
        BarEnd:        "]",
      }),
      progressbar.OptionShowCount(),
      progressbar.OptionClearOnFinish(),
      progressbar.OptionSetPredictTime(config.ShowETA),
      progressbar.OptionShowIts(),
      progressbar.OptionSetItsString("files"),
      progressbar.OptionThrottle(65*time.Millisecond),
      progressbar.OptionOnCompletion(func() {
        fmt.Fprint(config.Output, "\n")
      }),
    )
  }
  
  return r
}

// Start begins reporting progress
func (r *Reporter) Start() {
  updates := r.tracker.Subscribe()
  
  r.wg.Add(1)
  go r.processUpdates(updates)
  
  if r.format == OutputFormatTerminal {
    r.wg.Add(1)
    go r.refreshTerminal()
  }
}

// Stop stops reporting progress
func (r *Reporter) Stop() {
  close(r.done)
  r.wg.Wait()
  
  // Final update
  r.reportProgress(r.tracker.GetSnapshot())
  
  if r.format == OutputFormatTerminal && r.progressBar != nil {
    r.progressBar.Finish()
  }
}

// processUpdates processes incoming progress updates
func (r *Reporter) processUpdates(updates <-chan Update) {
  defer r.wg.Done()
  
  for {
    select {
    case update, ok := <-updates:
      if !ok {
        return
      }
      
      // Handle different update types
      switch update.Type {
      case UpdateTypeError:
        r.reportError(update.Error)
      case UpdateTypeState:
        r.reportStateChange()
      default:
        // Batch file/byte updates for performance
        if r.format != OutputFormatTerminal {
          r.updateMu.Lock()
          if time.Since(r.lastUpdate) > 100*time.Millisecond {
            r.reportProgress(r.tracker.GetSnapshot())
            r.lastUpdate = time.Now()
          }
          r.updateMu.Unlock()
        }
      }
      
    case <-r.done:
      return
    }
  }
}

// refreshTerminal refreshes terminal display periodically
func (r *Reporter) refreshTerminal() {
  defer r.wg.Done()
  
  ticker := time.NewTicker(100 * time.Millisecond)
  defer ticker.Stop()
  
  for {
    select {
    case <-ticker.C:
      snapshot := r.tracker.GetSnapshot()
      r.updateProgressBar(snapshot)
      
    case <-r.done:
      return
    }
  }
}

// reportProgress reports progress based on the configured format
func (r *Reporter) reportProgress(snapshot ProgressSnapshot) {
  switch r.format {
  case OutputFormatTerminal:
    r.updateProgressBar(snapshot)
  case OutputFormatJSON:
    r.reportJSON(snapshot)
  case OutputFormatQuiet:
    // Quiet mode only reports on completion
    if snapshot.State == StateCompleted {
      r.reportQuiet(snapshot)
    }
  }
}

// updateProgressBar updates the terminal progress bar
func (r *Reporter) updateProgressBar(snapshot ProgressSnapshot) {
  if r.progressBar == nil {
    return
  }
  
  // Update description with current status
  description := r.formatDescription(snapshot)
  r.progressBar.Describe(description)
  
  // Update progress
  if snapshot.TotalBytes > 0 {
    r.progressBar.ChangeMax64(snapshot.TotalBytes)
    r.progressBar.Set64(snapshot.ProcessedBytes)
  } else if snapshot.TotalFiles > 0 {
    r.progressBar.ChangeMax64(snapshot.TotalFiles)
    r.progressBar.Set64(snapshot.ProcessedFiles)
  }
}

// formatDescription formats the progress bar description
func (r *Reporter) formatDescription(snapshot ProgressSnapshot) string {
  var parts []string
  
  // State indicator
  switch snapshot.State {
  case StateRunning:
    parts = append(parts, "[cyan]Syncing[reset]")
  case StatePaused:
    parts = append(parts, "[yellow]Paused[reset]")
  case StateCompleted:
    parts = append(parts, "[green]Completed[reset]")
  case StateError:
    parts = append(parts, "[red]Error[reset]")
  }
  
  // File progress
  parts = append(parts, fmt.Sprintf("%d/%d files",
    snapshot.ProcessedFiles, snapshot.TotalFiles))
  
  // Byte progress with human-readable sizes
  if snapshot.TotalBytes > 0 {
    parts = append(parts, fmt.Sprintf("%s/%s",
      formatBytes(snapshot.ProcessedBytes),
      formatBytes(snapshot.TotalBytes)))
  }
  
  // Speed
  speed := snapshot.BytesPerSecond()
  if speed > 0 {
    parts = append(parts, fmt.Sprintf("%s/s", formatBytes(int64(speed))))
  }
  
  // ETA
  eta := snapshot.ETA()
  if eta > 0 {
    parts = append(parts, fmt.Sprintf("ETA: %s", formatDuration(eta)))
  }
  
  // Error count
  if snapshot.ErrorCount > 0 {
    parts = append(parts, fmt.Sprintf("[red]%d errors[reset]", 
      snapshot.ErrorCount))
  }
  
  return strings.Join(parts, " | ")
}

// reportJSON outputs progress in JSON format
func (r *Reporter) reportJSON(snapshot ProgressSnapshot) {
  output := map[string]interface{}{
    "timestamp":       time.Now().Unix(),
    "state":           snapshot.State.String(),
    "total_files":     snapshot.TotalFiles,
    "processed_files": snapshot.ProcessedFiles,
    "total_bytes":     snapshot.TotalBytes,
    "processed_bytes": snapshot.ProcessedBytes,
    "percent":         snapshot.PercentComplete(),
    "speed_bps":       snapshot.BytesPerSecond(),
    "eta_seconds":     snapshot.ETA().Seconds(),
    "elapsed_seconds": snapshot.ElapsedTime.Seconds(),
    "error_count":     snapshot.ErrorCount,
  }
  
  if snapshot.CurrentFile != "" {
    output["current_file"] = snapshot.CurrentFile
  }
  
  data, _ := json.Marshal(output)
  fmt.Fprintln(r.output, string(data))
}

// reportQuiet outputs minimal progress information
func (r *Reporter) reportQuiet(snapshot ProgressSnapshot) {
  fmt.Fprintf(r.output, "Completed: %d files, %s in %s\n",
    snapshot.ProcessedFiles,
    formatBytes(snapshot.ProcessedBytes),
    formatDuration(snapshot.ElapsedTime))
  
  if snapshot.ErrorCount > 0 {
    fmt.Fprintf(r.output, "Errors: %d\n", snapshot.ErrorCount)
  }
}

// reportError reports an error
func (r *Reporter) reportError(err error) {
  switch r.format {
  case OutputFormatTerminal:
    fmt.Fprintf(r.output, "\n[red]Error:[reset] %v\n", err)
  case OutputFormatJSON:
    output := map[string]interface{}{
      "timestamp": time.Now().Unix(),
      "type":      "error",
      "error":     err.Error(),
    }
    data, _ := json.Marshal(output)
    fmt.Fprintln(r.output, string(data))
  case OutputFormatQuiet:
    // Quiet mode suppresses error output
  }
}

// reportStateChange reports a state change
func (r *Reporter) reportStateChange() {
  snapshot := r.tracker.GetSnapshot()
  
  switch r.format {
  case OutputFormatTerminal:
    // Terminal updates are handled by progress bar
  case OutputFormatJSON:
    output := map[string]interface{}{
      "timestamp": time.Now().Unix(),
      "type":      "state_change",
      "state":     snapshot.State.String(),
    }
    data, _ := json.Marshal(output)
    fmt.Fprintln(r.output, string(data))
  case OutputFormatQuiet:
    // Quiet mode suppresses state changes
  }
}

// formatBytes formats bytes into human-readable format
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
  
  return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), 
    "KMGTPE"[exp])
}

// formatDuration formats duration into human-readable format
func formatDuration(d time.Duration) string {
  if d < time.Minute {
    return fmt.Sprintf("%ds", int(d.Seconds()))
  }
  if d < time.Hour {
    return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
  }
  hours := int(d.Hours())
  minutes := int(d.Minutes()) % 60
  return fmt.Sprintf("%dh%dm", hours, minutes)
}

// String returns the string representation of a State
func (s State) String() string {
  switch s {
  case StateIdle:
    return "idle"
  case StateRunning:
    return "running"
  case StatePaused:
    return "paused"
  case StateCompleted:
    return "completed"
  case StateError:
    return "error"
  default:
    return "unknown"
  }
}