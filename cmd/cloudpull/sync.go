package main

import (
  "context"
  "fmt"
  "os"
  "path/filepath"
  "strings"
  "time"

  "github.com/AlecAivazis/survey/v2"
  "github.com/VatsalSy/CloudPull/internal/app"
  "github.com/fatih/color"
  "github.com/schollz/progressbar/v3"
  "github.com/spf13/cobra"
  "github.com/spf13/viper"
)

var syncCmd = &cobra.Command{
  Use:   "sync [folder-id|folder-url]",
  Short: "Start a new sync from Google Drive",
  Long: `Start syncing files from a Google Drive folder to your local filesystem.

You can specify the folder by:
  • Folder ID: The unique identifier from the Drive URL
  • Share URL: The full Google Drive sharing URL
  • Nothing: Interactive folder selection`,
  Example: `  # Interactive folder selection
  cloudpull sync

  # Sync using folder ID
  cloudpull sync 1ABC123DEF456GHI

  # Sync using share URL
  cloudpull sync "https://drive.google.com/drive/folders/1ABC123DEF456GHI"

  # Sync with custom options
  cloudpull sync --output ~/Documents/DriveSync --include "*.pdf" --exclude "temp/*"`,
  RunE: runSync,
}

var (
  outputDir      string
  includePattern []string
  excludePattern []string
  dryRun         bool
  noProgress     bool
  maxDepth       int
)

func init() {
  syncCmd.Flags().StringVarP(&outputDir, "output", "o", "",
    "Output directory (default: configured sync directory)")
  syncCmd.Flags().StringSliceVarP(&includePattern, "include", "i", []string{},
    "Include only files matching pattern (can be used multiple times)")
  syncCmd.Flags().StringSliceVarP(&excludePattern, "exclude", "e", []string{},
    "Exclude files matching pattern (can be used multiple times)")
  syncCmd.Flags().BoolVar(&dryRun, "dry-run", false,
    "Show what would be synced without downloading")
  syncCmd.Flags().BoolVar(&noProgress, "no-progress", false,
    "Disable progress bars")
  syncCmd.Flags().IntVar(&maxDepth, "max-depth", -1,
    "Maximum folder depth to sync (-1 for unlimited)")
}

func runSync(cmd *cobra.Command, args []string) error {
  // Initialize app
  application, err := app.New()
  if err != nil {
    return fmt.Errorf("failed to create application: %w", err)
  }

  if err := application.Initialize(); err != nil {
    return fmt.Errorf("failed to initialize application: %w", err)
  }

  if err := application.InitializeAuth(); err != nil {
    return fmt.Errorf("not authenticated. Run 'cloudpull init' first")
  }

  if err := application.InitializeSyncEngine(); err != nil {
    return fmt.Errorf("failed to initialize sync engine: %w", err)
  }

  fmt.Println(color.CyanString("📂 CloudPull Sync"))
  fmt.Println()

  // Get folder to sync
  var folderID string
  if len(args) > 0 {
    folderID = extractFolderID(args[0])
  } else {
    // Interactive folder selection
    folderID = selectDriveFolder()
    if folderID == "" {
      return fmt.Errorf("no folder selected")
    }
  }

  // Determine output directory
  if outputDir == "" {
    outputDir = viper.GetString("sync.default_directory")
    if outputDir == "" {
      home, _ := os.UserHomeDir()
      outputDir = filepath.Join(home, "CloudPull", folderID)
    }
  }

  // Confirm sync settings
  fmt.Println(color.YellowString("Sync Configuration:"))
  fmt.Printf("  Source: Google Drive folder %s\n", folderID)
  fmt.Printf("  Destination: %s\n", outputDir)
  if len(includePattern) > 0 {
    fmt.Printf("  Include: %s\n", strings.Join(includePattern, ", "))
  }
  if len(excludePattern) > 0 {
    fmt.Printf("  Exclude: %s\n", strings.Join(excludePattern, ", "))
  }
  if dryRun {
    fmt.Println(color.YellowString("  Mode: DRY RUN (no files will be downloaded)"))
  }
  fmt.Println()

  if !dryRun {
    var proceed bool
    prompt := &survey.Confirm{
      Message: "Start sync?",
      Default: true,
    }
    survey.AskOne(prompt, &proceed)
    if !proceed {
      return nil
    }
  }

  // Create output directory
  if err := os.MkdirAll(outputDir, 0755); err != nil {
    return fmt.Errorf("failed to create output directory: %w", err)
  }

  // Prepare sync options
  syncOptions := &app.SyncOptions{
    IncludePatterns: includePattern,
    ExcludePatterns: excludePattern,
    MaxDepth:        maxDepth,
    DryRun:          dryRun,
  }

  // Start sync with progress monitoring
  ctx := context.Background()
  errChan := make(chan error, 1)
  
  go func() {
    errChan <- application.StartSync(ctx, folderID, outputDir, syncOptions)
  }()

  // Monitor progress
  if !noProgress && !dryRun {
    go monitorSyncProgress(application)
  }

  // Wait for completion
  if err := <-errChan; err != nil {
    return fmt.Errorf("sync failed: %w", err)
  }

  fmt.Println(color.GreenString("\n✅ Sync completed successfully!"))
  return nil
}

func extractFolderID(input string) string {
  // Extract folder ID from URL or return as-is
  if strings.Contains(input, "drive.google.com") {
    parts := strings.Split(input, "/")
    for i, part := range parts {
      if part == "folders" && i+1 < len(parts) {
        return parts[i+1]
      }
    }
  }
  return input
}

func selectDriveFolder() string {
  // TODO: Implement Drive API folder listing
  fmt.Println("Interactive folder selection coming soon...")
  
  // Placeholder
  var folderID string
  prompt := &survey.Input{
    Message: "Enter Google Drive folder ID or URL:",
  }
  survey.AskOne(prompt, &folderID)
  return extractFolderID(folderID)
}

func monitorSyncProgress(app *app.App) {
  ticker := time.NewTicker(100 * time.Millisecond)
  defer ticker.Stop()

  var bar *progressbar.ProgressBar
  lastFiles := int64(0)

  for {
    progress := app.GetProgress()
    if progress == nil {
      time.Sleep(time.Second)
      continue
    }

    // Initialize progress bar on first update
    if bar == nil && progress.TotalFiles > 0 {
      bar = progressbar.NewOptions64(
        progress.TotalFiles,
        progressbar.OptionSetDescription("Syncing files"),
        progressbar.OptionSetWidth(40),
        progressbar.OptionShowCount(),
        progressbar.OptionShowIts(),
        progressbar.OptionSetItsString("files"),
        progressbar.OptionOnCompletion(func() {
          fmt.Print("\n")
        }),
        progressbar.OptionSpinnerType(14),
        progressbar.OptionFullWidth(),
        progressbar.OptionSetRenderBlankState(true),
      )
    }

    // Update progress
    if bar != nil && progress.CompletedFiles > lastFiles {
      bar.Set64(progress.CompletedFiles)
      lastFiles = progress.CompletedFiles
    }

    // Check if complete
    if progress.Status == "stopped" {
      if bar != nil {
        bar.Finish()
      }
      break
    }

    <-ticker.C
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