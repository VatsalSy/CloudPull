/**
 * Full Sync Example for CloudPull
 * 
 * This example demonstrates a complete sync workflow using CloudPull,
 * including authentication, starting a sync, monitoring progress,
 * and handling interruptions.
 * 
 * Author: CloudPull Team
 * Updated: 2025-01-29
 */

package main

import (
  "context"
  "flag"
  "fmt"
  "log"
  "os"
  "os/signal"
  "syscall"
  "time"

  "github.com/VatsalSy/CloudPull/internal/app"
  cloudsync "github.com/VatsalSy/CloudPull/internal/sync"
  "github.com/fatih/color"
)

func main() {
  // Parse command line flags
  var (
    folderID   = flag.String("folder", "root", "Google Drive folder ID to sync")
    outputDir  = flag.String("output", "~/CloudPull/Example", "Output directory")
    resume     = flag.String("resume", "", "Resume session ID")
    maxDepth   = flag.Int("depth", -1, "Maximum folder depth (-1 for unlimited)")
    include    = flag.String("include", "", "Include pattern (e.g., *.pdf)")
    exclude    = flag.String("exclude", "", "Exclude pattern (e.g., temp/*)")
  )
  flag.Parse()

  // Create and initialize application
  application, err := app.New()
  if err != nil {
    log.Fatal("Failed to create application:", err)
  }

  fmt.Println(color.CyanString("🚀 CloudPull Full Sync Example"))
  fmt.Println()

  // Initialize components
  fmt.Println("Initializing CloudPull...")
  if err := application.Initialize(); err != nil {
    log.Fatal("Failed to initialize:", err)
  }

  if err := application.InitializeAuth(); err != nil {
    log.Fatal("Failed to initialize auth:", err)
  }

  if err := application.InitializeSyncEngine(); err != nil {
    log.Fatal("Failed to initialize sync engine:", err)
  }

  fmt.Println(color.GreenString("✓ Initialization complete"))
  fmt.Println()

  // Create context with cancellation
  ctx, cancel := context.WithCancel(context.Background())
  defer cancel()

  // Handle signals for graceful shutdown
  sigChan := make(chan os.Signal, 1)
  signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
  
  go func() {
    sig := <-sigChan
    fmt.Printf("\n%s Received signal: %v\n", color.YellowString("⚠"), sig)
    fmt.Println("Stopping sync gracefully...")
    cancel()
  }()

  // Start sync or resume
  errChan := make(chan error, 1)
  
  if *resume != "" {
    // Resume existing session
    fmt.Printf("Resuming session: %s\n", *resume)
    go func() {
      errChan <- application.ResumeSync(ctx, *resume)
    }()
  } else {
    // Start new sync
    fmt.Printf("Starting sync from folder: %s\n", *folderID)
    fmt.Printf("Output directory: %s\n", *outputDir)
    
    // Build sync options
    options := &app.SyncOptions{
      MaxDepth: *maxDepth,
    }
    
    if *include != "" {
      options.IncludePatterns = []string{*include}
      fmt.Printf("Include pattern: %s\n", *include)
    }
    
    if *exclude != "" {
      options.ExcludePatterns = []string{*exclude}
      fmt.Printf("Exclude pattern: %s\n", *exclude)
    }
    
    fmt.Println()
    
    go func() {
      errChan <- application.StartSync(ctx, *folderID, *outputDir, options)
    }()
  }

  // Monitor progress
  progressDone := make(chan struct{})
  go monitorProgress(ctx, application, progressDone)

  // Wait for completion or error
  select {
  case err := <-errChan:
    if err != nil {
      fmt.Printf("\n%s Sync failed: %v\n", color.RedString("✗"), err)
      os.Exit(1)
    }
    fmt.Printf("\n%s Sync completed successfully!\n", color.GreenString("✓"))
  case <-ctx.Done():
    // Wait for progress monitor to finish
    <-progressDone
    fmt.Println(color.YellowString("Sync interrupted by user"))
  }

  // Cleanup
  fmt.Println("\nShutting down...")
  if err := application.Stop(); err != nil {
    log.Printf("Warning: shutdown error: %v", err)
  }

  fmt.Println("Goodbye! 👋")
}

func monitorProgress(ctx context.Context, app *app.App, done chan struct{}) {
  defer close(done)
  
  ticker := time.NewTicker(500 * time.Millisecond)
  defer ticker.Stop()

  var lastProgress *cloudsync.SyncProgress
  startTime := time.Now()

  for {
    select {
    case <-ctx.Done():
      return
    case <-ticker.C:
      progress := app.GetProgress()
      if progress == nil {
        continue
      }

      // Only update if progress changed
      if lastProgress == nil || 
         progress.CompletedFiles != lastProgress.CompletedFiles ||
         progress.FailedFiles != lastProgress.FailedFiles {
        
        displayProgress(progress, startTime)
        lastProgress = progress
      }

      // Check if complete
      if progress.Status == "stopped" {
        return
      }
    }
  }
}

func displayProgress(p *cloudsync.SyncProgress, startTime time.Time) {
  // Clear line and move cursor to beginning
  fmt.Print("\r\033[K")
  
  // Build progress string
  var status string
  switch p.Status {
  case "running":
    status = color.GreenString("↻")
  case "paused":
    status = color.YellowString("⏸")
  default:
    status = "•"
  }
  
  elapsed := time.Since(startTime).Round(time.Second)
  
  if p.TotalFiles > 0 {
    percentage := float64(p.CompletedFiles) / float64(p.TotalFiles) * 100
    
    fmt.Printf("%s Progress: %d/%d files (%.1f%%) | %s | %s/s | %s elapsed",
      status,
      p.CompletedFiles,
      p.TotalFiles,
      percentage,
      formatBytes(p.CompletedBytes),
      formatBytes(p.CurrentSpeed),
      elapsed,
    )
    
    if p.FailedFiles > 0 {
      fmt.Printf(" | %s", color.RedString("%d failed", p.FailedFiles))
    }
  } else {
    // Still scanning
    fmt.Printf("%s Scanning... %d files found | %s | %s elapsed",
      status,
      p.CompletedFiles,
      formatBytes(p.CompletedBytes),
      elapsed,
    )
  }
}

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