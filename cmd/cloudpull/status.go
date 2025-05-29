package main

import (
  "fmt"
  "os"
  "strings"
  "time"

  "github.com/fatih/color"
  "github.com/jedib0t/go-pretty/v6/table"
  "github.com/schollz/progressbar/v3"
  "github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
  Use:   "status [session-id]",
  Short: "Show sync progress and statistics",
  Long: `Display detailed status information about ongoing or completed sync sessions.

Shows real-time progress, transfer speeds, and estimated completion time
for active syncs.`,
  Example: `  # Show status of all active sessions
  cloudpull status

  # Show status of specific session
  cloudpull status abc123

  # Show detailed statistics
  cloudpull status --detailed

  # Monitor status continuously
  cloudpull status --watch`,
  RunE: runStatus,
}

var (
  watchStatus    bool
  detailedStatus bool
  showHistory    bool
)

func init() {
  statusCmd.Flags().BoolVarP(&watchStatus, "watch", "w", false,
    "Continuously monitor status")
  statusCmd.Flags().BoolVarP(&detailedStatus, "detailed", "d", false,
    "Show detailed statistics")
  statusCmd.Flags().BoolVar(&showHistory, "history", false,
    "Show completed sessions")
}

func runStatus(cmd *cobra.Command, args []string) error {
  if watchStatus {
    return watchSyncStatus(args)
  }

  if showHistory {
    return showSyncHistory()
  }

  return showSyncStatus(args)
}

func showSyncStatus(args []string) error {
  fmt.Println(color.CyanString("📊 CloudPull Status"))
  fmt.Println()

  // Get active sessions
  sessions := getActiveSessions()
  if len(sessions) == 0 {
    fmt.Println(color.YellowString("No active sync sessions."))
    fmt.Println("\nUse 'cloudpull sync' to start a new sync")
    fmt.Println("Use 'cloudpull status --history' to see completed sessions")
    return nil
  }

  // Show specific session or all
  if len(args) > 0 {
    sessionID := args[0]
    for _, session := range sessions {
      if session.ID == sessionID {
        return showDetailedSession(session)
      }
    }
    return fmt.Errorf("session not found: %s", sessionID)
  }

  // Show all active sessions
  showActiveSessions(sessions)

  if detailedStatus {
    fmt.Println()
    showSystemStats()
  }

  return nil
}

func showActiveSessions(sessions []ActiveSession) {
  fmt.Printf("Active Sessions: %d\n\n", len(sessions))

  for _, session := range sessions {
    // Session header
    fmt.Printf("%s Session: %s\n", 
      color.GreenString("▶"), 
      color.CyanString(session.ID))
    fmt.Printf("  Source: %s → %s\n", session.Source, session.Destination)

    // Progress bar
    progress := float64(session.DownloadedBytes) / float64(session.TotalBytes) * 100
    bar := progressbar.NewOptions64(
      session.TotalBytes,
      progressbar.OptionSetDescription("  Progress"),
      progressbar.OptionSetWidth(40),
      progressbar.OptionShowBytes(true),
      progressbar.OptionShowCount(),
      progressbar.OptionSetPredictTime(false),
      progressbar.OptionSetTheme(progressbar.Theme{
        Saucer:        "=",
        SaucerHead:    ">",
        SaucerPadding: " ",
        BarStart:      "[",
        BarEnd:        "]",
      }),
    )
    bar.Set64(session.DownloadedBytes)
    fmt.Print("\n")

    // Statistics
    fmt.Printf("  Files: %d/%d (%.0f%%) | Speed: %s/s | ETA: %s\n",
      session.CompletedFiles, session.TotalFiles,
      float64(session.CompletedFiles)/float64(session.TotalFiles)*100,
      formatBytes(session.Speed),
      formatDuration(session.ETA))

    if session.CurrentFile != "" {
      fmt.Printf("  Current: %s\n", color.YellowString(session.CurrentFile))
    }
    fmt.Println()
  }
}

func showDetailedSession(session ActiveSession) error {
  fmt.Printf("%s Session Details: %s\n", 
    color.GreenString("▶"), 
    color.CyanString(session.ID))
  fmt.Println(strings.Repeat("─", 50))

  // Basic info
  info := [][]string{
    {"Started", session.StartTime.Format("Jan 2, 2006 3:04:05 PM")},
    {"Duration", formatDuration(time.Since(session.StartTime))},
    {"Source", session.Source},
    {"Destination", session.Destination},
  }

  for _, row := range info {
    fmt.Printf("%-15s: %s\n", row[0], row[1])
  }

  fmt.Println()

  // Progress details
  fmt.Println(color.YellowString("Progress:"))
  fmt.Printf("  Files      : %d / %d (%.1f%%)\n",
    session.CompletedFiles, session.TotalFiles,
    float64(session.CompletedFiles)/float64(session.TotalFiles)*100)
  fmt.Printf("  Downloaded : %s / %s (%.1f%%)\n",
    formatBytes(session.DownloadedBytes), formatBytes(session.TotalBytes),
    float64(session.DownloadedBytes)/float64(session.TotalBytes)*100)
  fmt.Printf("  Remaining  : %s\n", formatBytes(session.TotalBytes-session.DownloadedBytes))

  fmt.Println()

  // Transfer stats
  fmt.Println(color.YellowString("Transfer Statistics:"))
  fmt.Printf("  Current Speed : %s/s\n", formatBytes(session.Speed))
  fmt.Printf("  Average Speed : %s/s\n", formatBytes(session.AvgSpeed))
  fmt.Printf("  Peak Speed    : %s/s\n", formatBytes(session.PeakSpeed))
  fmt.Printf("  ETA           : %s\n", formatDuration(session.ETA))

  if session.CurrentFile != "" {
    fmt.Println()
    fmt.Println(color.YellowString("Current Activity:"))
    fmt.Printf("  Downloading: %s\n", session.CurrentFile)
    fmt.Printf("  File Size  : %s\n", formatBytes(session.CurrentFileSize))
    fmt.Printf("  Progress   : %.1f%%\n", session.CurrentFileProgress)
  }

  // Recent files
  if len(session.RecentFiles) > 0 {
    fmt.Println()
    fmt.Println(color.YellowString("Recently Completed:"))
    for _, file := range session.RecentFiles {
      fmt.Printf("  ✓ %s (%s)\n", file.Name, formatBytes(file.Size))
    }
  }

  return nil
}

func watchSyncStatus(args []string) error {
  fmt.Println(color.CyanString("📊 CloudPull Status Monitor"))
  fmt.Println("Press Ctrl+C to exit")
  fmt.Println()

  for {
    // Clear screen (simple version)
    fmt.Print("\033[H\033[2J")
    
    showSyncStatus(args)
    
    time.Sleep(1 * time.Second)
  }
}

func showSyncHistory() error {
  fmt.Println(color.CyanString("📜 CloudPull Sync History"))
  fmt.Println()

  history := getSyncHistory()
  if len(history) == 0 {
    fmt.Println("No completed sync sessions.")
    return nil
  }

  t := table.NewWriter()
  t.SetOutputMirror(os.Stdout)
  t.AppendHeader(table.Row{"Session ID", "Date", "Duration", "Files", "Size", "Status"})

  for _, session := range history {
    status := color.GreenString("✓ Completed")
    if session.Failed {
      status = color.RedString("✗ Failed")
    } else if session.Cancelled {
      status = color.YellowString("⚠ Cancelled")
    }

    t.AppendRow(table.Row{
      session.ID,
      session.EndTime.Format("Jan 2 15:04"),
      formatDuration(session.Duration),
      fmt.Sprintf("%d", session.TotalFiles),
      formatBytes(session.TotalBytes),
      status,
    })
  }

  t.Render()
  fmt.Printf("\nTotal: %d sessions\n", len(history))

  return nil
}

func showSystemStats() {
  fmt.Println(color.YellowString("System Statistics:"))
  
  stats := getSystemStats()
  fmt.Printf("  Network Usage    : %s/s ↓ / %s/s ↑\n", 
    formatBytes(stats.DownloadRate), formatBytes(stats.UploadRate))
  fmt.Printf("  Disk Space       : %s free of %s\n",
    formatBytes(stats.DiskFree), formatBytes(stats.DiskTotal))
  fmt.Printf("  Memory Usage     : %.1f%% (%s / %s)\n",
    float64(stats.MemUsed)/float64(stats.MemTotal)*100,
    formatBytes(stats.MemUsed), formatBytes(stats.MemTotal))
  fmt.Printf("  Active Threads   : %d\n", stats.ActiveThreads)
}

type ActiveSession struct {
  ID                  string
  StartTime           time.Time
  Source              string
  Destination         string
  TotalFiles          int
  CompletedFiles      int
  TotalBytes          int64
  DownloadedBytes     int64
  Speed               int64
  AvgSpeed            int64
  PeakSpeed           int64
  ETA                 time.Duration
  CurrentFile         string
  CurrentFileSize     int64
  CurrentFileProgress float64
  RecentFiles         []CompletedFile
}

type CompletedFile struct {
  Name string
  Size int64
}

type SystemStats struct {
  DownloadRate  int64
  UploadRate    int64
  DiskFree      int64
  DiskTotal     int64
  MemUsed       int64
  MemTotal      int64
  ActiveThreads int
}

func getActiveSessions() []ActiveSession {
  // TODO: Implement actual session tracking
  // This is placeholder data
  return []ActiveSession{
    {
      ID:              "session_active_1",
      StartTime:       time.Now().Add(-45 * time.Minute),
      Source:          "Work Documents",
      Destination:     "~/CloudPull/WorkDocs",
      TotalFiles:      324,
      CompletedFiles:  187,
      TotalBytes:      1024 * 1024 * 850,
      DownloadedBytes: 1024 * 1024 * 492,
      Speed:           1024 * 1024 * 2,
      AvgSpeed:        1024 * 1024 * 3,
      PeakSpeed:       1024 * 1024 * 8,
      ETA:             15 * time.Minute,
      CurrentFile:     "Quarterly_Report_2024.pdf",
      CurrentFileSize: 1024 * 1024 * 12,
      CurrentFileProgress: 67.5,
      RecentFiles: []CompletedFile{
        {"Budget_2024.xlsx", 1024 * 512},
        {"Meeting_Notes.docx", 1024 * 256},
      },
    },
  }
}

func getSyncHistory() []SyncSession {
  // TODO: Implement actual history
  return []SyncSession{
    {
      ID:         "session_2024_01_19_091523",
      StartTime:  time.Now().Add(-26 * time.Hour),
      TotalFiles: 1250,
      TotalBytes: 1024 * 1024 * 1024 * 5,
    },
  }
}

func getSystemStats() SystemStats {
  // TODO: Implement actual system stats
  return SystemStats{
    DownloadRate:  1024 * 1024 * 2,
    UploadRate:    1024 * 256,
    DiskFree:      1024 * 1024 * 1024 * 120,
    DiskTotal:     1024 * 1024 * 1024 * 500,
    MemUsed:       1024 * 1024 * 1024 * 4,
    MemTotal:      1024 * 1024 * 1024 * 16,
    ActiveThreads: 3,
  }
}

func formatDuration(d time.Duration) string {
  if d < time.Minute {
    return fmt.Sprintf("%ds", int(d.Seconds()))
  }
  if d < time.Hour {
    return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
  }
  return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}