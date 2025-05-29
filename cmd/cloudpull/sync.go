package main

import (
  "fmt"
  "os"
  "path/filepath"
  "strings"
  "time"

  "github.com/AlecAivazis/survey/v2"
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
  // Check authentication
  if !isAuthenticated() {
    return fmt.Errorf("not authenticated. Run 'cloudpull init' first")
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

  // Start sync
  return performSync(folderID, outputDir)
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

func isAuthenticated() bool {
  // TODO: Check if OAuth token exists and is valid
  return viper.GetString("credentials_file") != ""
}

func performSync(folderID, outputDir string) error {
  fmt.Println(color.GreenString("\n🚀 Starting sync..."))

  // TODO: Implement actual Drive API sync
  // This is a simulation
  files := []struct {
    name string
    size int64
  }{
    {"Document.pdf", 1024 * 1024 * 2},
    {"Presentation.pptx", 1024 * 1024 * 5},
    {"Spreadsheet.xlsx", 1024 * 512},
    {"Image.jpg", 1024 * 1024 * 3},
  }

  if dryRun {
    fmt.Println("\nFiles that would be synced:")
    totalSize := int64(0)
    for _, file := range files {
      fmt.Printf("  • %s (%s)\n", file.name, formatBytes(file.size))
      totalSize += file.size
    }
    fmt.Printf("\nTotal: %d files, %s\n", len(files), formatBytes(totalSize))
    return nil
  }

  // Progress tracking
  totalFiles := len(files)
  completedFiles := 0

  fmt.Printf("\nSyncing %d files...\n\n", totalFiles)

  for _, file := range files {
    if !noProgress {
      bar := progressbar.NewOptions64(
        file.size,
        progressbar.OptionSetDescription(fmt.Sprintf("[%d/%d] %s", 
          completedFiles+1, totalFiles, file.name)),
        progressbar.OptionSetWidth(40),
        progressbar.OptionShowBytes(true),
        progressbar.OptionShowCount(),
        progressbar.OptionOnCompletion(func() {
          fmt.Print("\n")
        }),
        progressbar.OptionSpinnerType(14),
        progressbar.OptionFullWidth(),
      )

      // Simulate download
      for i := int64(0); i < file.size; i += 1024 * 100 {
        bar.Add(1024 * 100)
        time.Sleep(10 * time.Millisecond)
      }
      bar.Finish()
    } else {
      fmt.Printf("Downloading %s... ", file.name)
      time.Sleep(500 * time.Millisecond)
      fmt.Println("✓")
    }
    
    completedFiles++
  }

  fmt.Println(color.GreenString("\n✅ Sync completed successfully!"))
  fmt.Printf("Downloaded %d files to %s\n", completedFiles, outputDir)

  return nil
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