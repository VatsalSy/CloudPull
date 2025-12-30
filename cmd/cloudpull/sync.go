package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/VatsalSy/CloudPull/internal/app"
)

// Status constants for sync progress monitoring.
const (
	syncStatusStopped   = "stopped"
	syncStatusCompleted = "completed"
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
	outputDir       string
	includePatterns []string
	excludePatterns []string
	dryRun          bool
	noProgress      bool
	maxDepth        int
	noConfirm       bool
)

func init() {
	syncCmd.Flags().StringVarP(&outputDir, "output", "o", "",
		"Output directory (default: configured sync directory)")
	syncCmd.Flags().StringSliceVarP(&includePatterns, "include", "i", []string{},
		"Include only files matching pattern (can be used multiple times)")
	syncCmd.Flags().StringSliceVarP(&excludePatterns, "exclude", "e", []string{},
		"Exclude files matching pattern (can be used multiple times)")
	syncCmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"Show what would be synced without downloading")
	syncCmd.Flags().BoolVar(&noProgress, "no-progress", false,
		"Disable progress bars")
	syncCmd.Flags().IntVar(&maxDepth, "max-depth", -1,
		"Maximum folder depth to sync (-1 for unlimited)")
	syncCmd.Flags().BoolVarP(&noConfirm, "yes", "y", false,
		"Skip confirmation prompt")
}

func runSync(cmd *cobra.Command, args []string) error {
	// Initialize app
	application, err := initializeSyncApp()
	if err != nil {
		return err
	}

	fmt.Println(color.CyanString("📂 CloudPull Sync"))
	fmt.Println()

	// Get folder to sync
	folderID, err := getFolderID(args)
	if err != nil {
		return err
	}

	// Determine and validate output directory
	outDir, err := determineOutputDirectory(folderID)
	if err != nil {
		return err
	}

	// Display sync configuration
	displaySyncConfig(folderID, outDir)

	// Get user confirmation if needed
	if !dryRun && !noConfirm {
		proceed, err := confirmSyncStart()
		if err != nil {
			return err
		}
		if !proceed {
			return nil
		}
	}

	// Create output directory
	if err := os.MkdirAll(outDir, 0750); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Execute the sync operation
	return executeSyncOperation(application, folderID, outDir)
}

// initializeSyncApp creates and initializes the application for sync.
func initializeSyncApp() (*app.App, error) {
	application, err := app.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create application: %w", err)
	}

	if err := application.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize application: %w", err)
	}

	if err := application.InitializeAuth(); err != nil {
		return nil, fmt.Errorf("failed to initialize authentication: %w", err)
	}

	if !application.IsAuthenticated() {
		return nil, fmt.Errorf("not authenticated. Run 'cloudpull auth' first")
	}

	if err := application.InitializeSyncEngine(); err != nil {
		return nil, fmt.Errorf("failed to initialize sync engine: %w", err)
	}

	return application, nil
}

// getFolderID extracts or prompts for the folder ID to sync.
func getFolderID(args []string) (string, error) {
	if len(args) > 0 {
		folderID := extractFolderID(args[0])
		if folderID == "" {
			return "", fmt.Errorf("invalid folder ID or URL")
		}
		return folderID, nil
	}

	// Interactive folder selection
	folderID := selectDriveFolder()
	if folderID == "" {
		return "", fmt.Errorf("no folder selected")
	}
	return folderID, nil
}

// determineOutputDirectory determines and validates the output directory.
func determineOutputDirectory(folderID string) (string, error) {
	if outputDir != "" {
		return outputDir, nil
	}

	configDir := viper.GetString("sync.default_directory")
	if configDir != "" {
		return configDir, nil
	}

	// Build safe default path
	return buildSafeOutputPath(folderID)
}

// buildSafeOutputPath constructs a safe output path from the folder ID.
func buildSafeOutputPath(folderID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	// Sanitize folderID to prevent path traversal
	cleanedFolderID := filepath.Clean(folderID)
	if strings.ContainsAny(cleanedFolderID, "/\\") {
		return "", fmt.Errorf("invalid folder ID: contains path separators")
	}

	cleanedFolderID = strings.TrimLeft(cleanedFolderID, "./")
	if strings.Contains(cleanedFolderID, "..") {
		return "", fmt.Errorf("invalid folder ID: contains directory traversal")
	}

	outDir := filepath.Join(home, "CloudPull", cleanedFolderID)
	baseDir := filepath.Join(home, "CloudPull")

	if !strings.HasPrefix(filepath.Clean(outDir), filepath.Clean(baseDir)) {
		return "", fmt.Errorf("invalid output directory: path traversal detected")
	}

	return outDir, nil
}

// displaySyncConfig prints the sync configuration to stdout.
func displaySyncConfig(folderID, outDir string) {
	fmt.Println(color.YellowString("Sync Configuration:"))
	fmt.Printf("  Source: Google Drive folder %s\n", folderID)
	fmt.Printf("  Destination: %s\n", outDir)
	if len(includePatterns) > 0 {
		fmt.Printf("  Include: %s\n", strings.Join(includePatterns, ", "))
	}
	if len(excludePatterns) > 0 {
		fmt.Printf("  Exclude: %s\n", strings.Join(excludePatterns, ", "))
	}
	if dryRun {
		fmt.Println(color.YellowString("  Mode: DRY RUN (no files will be downloaded)"))
	}
	fmt.Println()
}

// confirmSyncStart prompts the user to confirm starting the sync.
func confirmSyncStart() (bool, error) {
	var proceed bool
	prompt := &survey.Confirm{
		Message: "Start sync?",
		Default: true,
	}
	err := survey.AskOne(prompt, &proceed)
	if err != nil {
		if err.Error() == "interrupt" {
			return false, fmt.Errorf("sync canceled by user")
		}
		return false, fmt.Errorf("failed to get user confirmation: %w", err)
	}
	return proceed, nil
}

// executeSyncOperation runs the sync and handles completion/interruption.
func executeSyncOperation(application *app.App, folderID, outDir string) error {
	syncOptions := &app.SyncOptions{
		IncludePatterns: includePatterns,
		ExcludePatterns: excludePatterns,
		MaxDepth:        maxDepth,
		DryRun:          dryRun,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	sessionID, err := application.StartSyncWithSession(ctx, folderID, outDir, syncOptions)
	if err != nil {
		return fmt.Errorf("failed to start sync: %w", err)
	}

	syncEngine := application.GetSyncEngine()
	if syncEngine == nil {
		return fmt.Errorf("sync engine not initialized")
	}
	completionChan := syncEngine.WaitForCompletion()

	progressDone := make(chan struct{})
	if !noProgress && !dryRun {
		go func() {
			monitorSyncProgress(application, completionChan)
			close(progressDone)
		}()
	}

	return waitForSyncCompletion(application, sessionID, cancel, sigChan, completionChan, progressDone)
}

// waitForSyncCompletion waits for the sync to complete or be interrupted.
func waitForSyncCompletion(
	application *app.App,
	sessionID string,
	cancel context.CancelFunc,
	sigChan chan os.Signal,
	completionChan <-chan struct{},
	progressDone chan struct{},
) error {
	for {
		select {
		case <-completionChan:
			fmt.Println(color.GreenString("\n✅ Sync completed successfully!"))
			return nil
		case <-progressDone:
			fmt.Println(color.GreenString("\n✅ Sync completed successfully!"))
			return nil
		case <-time.After(100 * time.Millisecond):
			if progress := application.GetProgress(); progress != nil {
				if progress.Status == syncStatusStopped || progress.Status == syncStatusCompleted {
					fmt.Println(color.GreenString("\n✅ Sync completed successfully!"))
					return nil
				}
			}
		case sig := <-sigChan:
			return handleSyncInterrupt(application, sessionID, cancel, sig, progressDone)
		}
	}
}

// handleSyncInterrupt handles the sync interruption gracefully.
func handleSyncInterrupt(
	application *app.App,
	sessionID string,
	cancel context.CancelFunc,
	sig os.Signal,
	progressDone chan struct{},
) error {
	fmt.Printf("\n%s Received signal: %v\n", color.YellowString("⚠️"), sig)
	fmt.Println("Cleaning up sync session...")

	cancel()

	// Force exit after timeout
	go func() {
		time.Sleep(10 * time.Second)
		fmt.Println("Force exit due to shutdown timeout")
		os.Exit(1)
	}()

	// Clean up the session
	if sessionID != "" {
		if err := application.CleanupSession(sessionID); err != nil {
			fmt.Printf("%s Failed to clean up session: %v\n", color.RedString("❌"), err)
		} else {
			fmt.Println(color.GreenString("✓ Session cleaned up"))
		}
	}

	// Wait for progress monitoring to finish with timeout
	if !noProgress && !dryRun {
		select {
		case <-progressDone:
			// Progress monitoring finished
		case <-time.After(5 * time.Second):
			fmt.Println("Progress monitoring timeout")
		}
	}

	return fmt.Errorf("sync interrupted by user")
}

func extractFolderID(input string) string {
	// Extract folder ID from URL or return as-is
	if strings.Contains(input, "drive.google.com") {
		parts := strings.Split(input, "/")
		for i, part := range parts {
			if part == "folders" && i+1 < len(parts) {
				folderID := parts[i+1]
				// Strip any query parameters
				if idx := strings.Index(folderID, "?"); idx != -1 {
					folderID = folderID[:idx]
				}
				// Validate folder ID pattern
				if isValidDriveID(folderID) {
					return folderID
				}
				return ""
			}
		}
	}
	// Validate if input is already a valid Drive ID
	if isValidDriveID(input) {
		return input
	}
	return ""
}

// isValidDriveID validates if a string matches the Google Drive ID pattern.
func isValidDriveID(id string) bool {
	// Google Drive IDs are typically 10+ characters containing alphanumeric, underscore, and hyphen
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]{10,}$`, id)
	return matched
}

func selectDriveFolder() string {
	// TODO: Implement Drive API folder listing
	fmt.Println("Interactive folder selection coming soon...")

	// Placeholder
	var folderID string
	prompt := &survey.Input{
		Message: "Enter Google Drive folder ID or URL:",
	}
	err := survey.AskOne(prompt, &folderID)
	if err != nil {
		// Handle user cancellation or I/O errors
		return ""
	}
	return extractFolderID(folderID)
}

func monitorSyncProgress(app *app.App, completionChan <-chan struct{}) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var bar *progressbar.ProgressBar
	lastFiles := int64(0)

	// Create a copy of the completion channel to avoid consuming it
	done := make(chan struct{})
	go func() {
		<-completionChan
		close(done)
	}()

	for {
		select {
		case <-done:
			// Sync completed
			if bar != nil {
				_ = bar.Finish()
			}
			return
		case <-ticker.C:
			progress := app.GetProgress()
			if progress == nil {
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

			// Update progress bar max if TotalFiles increased
			if bar != nil && progress.TotalFiles > bar.GetMax64() {
				bar.ChangeMax64(progress.TotalFiles)
			}

			// Update progress
			if bar != nil && progress.CompletedFiles > lastFiles {
				_ = bar.Set64(progress.CompletedFiles)
				lastFiles = progress.CompletedFiles
			}

			// Check if complete via status
			if progress.Status == syncStatusStopped || progress.Status == syncStatusCompleted {
				if bar != nil {
					_ = bar.Finish()
				}
				return
			}
		}
	}
}
