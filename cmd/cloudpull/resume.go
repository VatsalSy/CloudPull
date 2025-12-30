package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/VatsalSy/CloudPull/internal/app"
	"github.com/VatsalSy/CloudPull/internal/state"
	"github.com/VatsalSy/CloudPull/internal/util"
)

var resumeCmd = &cobra.Command{
	Use:   "resume [session-id]",
	Short: "Resume an interrupted sync session",
	Long: `Resume a previously interrupted sync session.

CloudPull automatically saves sync progress, allowing you to resume
downloads that were interrupted due to network issues, system shutdown,
or manual cancellation.`,
	Example: `  # List and select session to resume
  cloudpull resume

  # Resume specific session
  cloudpull resume abc123

  # Resume most recent session
  cloudpull resume --latest`,
	RunE: runResume,
}

// Status constants for progress monitoring.
const statusStopped = "stopped"

var (
	resumeLatest bool
	forceResume  bool
)

func init() {
	resumeCmd.Flags().BoolVar(&resumeLatest, "latest", false,
		"Resume the most recent interrupted session")
	resumeCmd.Flags().BoolVar(&forceResume, "force", false,
		"Force resume even if session appears corrupted")
}

func runResume(cmd *cobra.Command, args []string) error {
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

	fmt.Println(color.CyanString("🔄 CloudPull Resume"))
	fmt.Println()

	ctx := context.Background()

	// Get session to resume
	session, err := getSessionToResume(ctx, application, args)
	if err != nil {
		return err
	}

	// Display session info
	displaySessionInfo(session)

	// Check session status and get confirmation
	shouldProceed, err := checkSessionAndConfirm(session)
	if err != nil {
		return err
	}
	if !shouldProceed {
		return nil
	}

	// Resume sync with progress monitoring
	return executeResume(ctx, application, session)
}

// getSessionToResume retrieves the session based on args or user selection.
func getSessionToResume(ctx context.Context, application *app.App, args []string) (*state.Session, error) {
	switch {
	case len(args) > 0:
		return getSessionByID(ctx, application, args[0])
	case resumeLatest:
		return getLatestSession(ctx, application)
	default:
		return selectSessionFromApp(ctx, application)
	}
}

// getSessionByID finds a session by its ID using direct database lookup.
func getSessionByID(ctx context.Context, application *app.App, sessionID string) (*state.Session, error) {
	session, err := application.GetSessionByID(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session by ID: %w", err)
	}
	return session, nil
}

// getLatestSession retrieves the most recent interrupted session.
func getLatestSession(ctx context.Context, application *app.App) (*state.Session, error) {
	session, err := application.GetLatestSession(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest session: %w", err)
	}
	if session == nil {
		return nil, fmt.Errorf("no interrupted sessions found")
	}
	return session, nil
}

// displaySessionInfo prints session details to stdout.
func displaySessionInfo(session *state.Session) {
	fmt.Println(color.YellowString("Session Details:"))
	fmt.Printf("  ID: %s\n", session.ID)
	fmt.Printf("  Started: %s\n", session.StartTime.Format("Jan 2, 2006 3:04 PM"))
	fmt.Printf("  Source: %s\n", session.RootFolderName.String)
	fmt.Printf("  Destination: %s\n", session.DestinationPath)
	if session.TotalFiles > 0 {
		fmt.Printf("  Progress: %d/%d files (%.1f%%)\n",
			session.CompletedFiles, session.TotalFiles,
			float64(session.CompletedFiles)/float64(session.TotalFiles)*100)
		fmt.Printf("  Downloaded: %s of %s\n",
			util.FormatBytes(session.CompletedBytes), util.FormatBytes(session.TotalBytes))
	} else {
		fmt.Printf("  Progress: %d files completed\n", session.CompletedFiles)
	}
	fmt.Println()
}

// checkSessionAndConfirm validates session status and prompts for confirmation.
func checkSessionAndConfirm(session *state.Session) (bool, error) {
	// Check if already completed
	if session.Status == state.SessionStatusCompleted {
		fmt.Println(color.YellowString("⚠️  Warning: Session is already completed"))
		return false, nil
	}

	// Check if failed and needs force
	if session.Status == state.SessionStatusFailed && !forceResume {
		fmt.Println(color.RedString("⚠️  Warning: Session failed previously"))
		var proceed bool
		prompt := &survey.Confirm{
			Message: "Attempt to resume anyway?",
			Default: false,
		}
		if err := survey.AskOne(prompt, &proceed); err != nil {
			return false, fmt.Errorf("failed to get user confirmation for failed session: %w", err)
		}
		if !proceed {
			return false, nil
		}
	}

	// Confirm resume
	var confirm bool
	prompt := &survey.Confirm{
		Message: "Resume this sync session?",
		Default: true,
	}
	if err := survey.AskOne(prompt, &confirm); err != nil {
		return false, fmt.Errorf("failed to get resume confirmation: %w", err)
	}
	return confirm, nil
}

// executeResume performs the actual resume operation with progress monitoring.
func executeResume(ctx context.Context, application *app.App, session *state.Session) error {
	errChan := make(chan error, 1)

	// Create a cancellable context for the monitor goroutine
	monitorCtx, cancelMonitor := context.WithCancel(ctx)
	defer cancelMonitor() // Ensure cleanup

	go func() {
		errChan <- application.ResumeSync(ctx, session.ID)
	}()

	// Monitor progress with context
	go monitorResumeProgress(monitorCtx, application)

	// Wait for completion
	if err := <-errChan; err != nil {
		cancelMonitor() // Cancel monitor on error
		return fmt.Errorf("resume failed: %w", err)
	}

	cancelMonitor() // Cancel monitor on success
	fmt.Println(color.GreenString("\n✅ Sync resumed successfully!"))
	return nil
}

func selectSessionFromApp(ctx context.Context, app *app.App) (*state.Session, error) {
	sessions, err := app.GetSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get sessions: %w", err)
	}

	// Filter out completed sessions
	var resumableSessions []*state.Session
	for _, s := range sessions {
		if s.Status != state.SessionStatusCompleted && s.Status != state.SessionStatusCancelled {
			resumableSessions = append(resumableSessions, s)
		}
	}

	if len(resumableSessions) == 0 {
		fmt.Println(color.YellowString("No resumable sessions found."))
		return nil, nil
	}

	// Create table
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"#", "Session ID", "Started", "Progress", "Size", "Status"})

	options := make([]string, len(resumableSessions))
	for i, session := range resumableSessions {
		var progress string
		if session.TotalFiles > 0 {
			progress = fmt.Sprintf("%d/%d (%.0f%%)",
				session.CompletedFiles, session.TotalFiles,
				float64(session.CompletedFiles)/float64(session.TotalFiles)*100)
		} else {
			progress = fmt.Sprintf("%d files", session.CompletedFiles)
		}

		size := "N/A"
		if session.TotalBytes > 0 {
			size = fmt.Sprintf("%s/%s",
				util.FormatBytes(session.CompletedBytes),
				util.FormatBytes(session.TotalBytes))
		} else if session.CompletedBytes > 0 {
			size = util.FormatBytes(session.CompletedBytes)
		}

		statusColor := session.Status
		switch session.Status {
		case state.SessionStatusFailed:
			statusColor = color.RedString(session.Status)
		case state.SessionStatusPaused:
			statusColor = color.YellowString(session.Status)
		case state.SessionStatusActive:
			statusColor = color.GreenString(session.Status)
		}

		t.AppendRow(table.Row{
			i + 1,
			session.ID[:8] + "...",
			session.StartTime.Format("Jan 2 15:04"),
			progress,
			size,
			statusColor,
		})

		options[i] = fmt.Sprintf("%s - %s (%s)",
			session.ID[:8],
			session.StartTime.Format("Jan 2 15:04"),
			progress)
	}

	fmt.Println("Resumable Sessions:")
	t.Render()
	fmt.Println()

	var selected string
	prompt := &survey.Select{
		Message: "Select session to resume:",
		Options: options,
	}
	if err := survey.AskOne(prompt, &selected); err != nil {
		return nil, fmt.Errorf("failed to get session selection: %w", err)
	}

	if selected != "" {
		// Extract index from selection
		for i, opt := range options {
			if opt == selected {
				return resumableSessions[i], nil
			}
		}
	}
	return nil, nil
}

func monitorResumeProgress(ctx context.Context, app *app.App) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastFiles := int64(0)
	lastUpdate := time.Now()

	for {
		select {
		case <-ctx.Done():
			// Context canceled, exit cleanly
			fmt.Println() // New line after progress
			return
		case <-ticker.C:
			progress := app.GetProgress()
			if progress == nil {
				continue
			}

			// Update progress every second or on file completion
			if progress.CompletedFiles > lastFiles || time.Since(lastUpdate) > time.Second {
				percentage := 0.0
				if progress.TotalFiles > 0 {
					percentage = float64(progress.CompletedFiles) / float64(progress.TotalFiles) * 100
				}
				fmt.Printf("\rProgress: %d/%d files (%.1f%%) | Speed: %s/s | Active: %d",
					progress.CompletedFiles, progress.TotalFiles,
					percentage,
					util.FormatBytes(progress.CurrentSpeed),
					progress.ActiveDownloads)
				lastFiles = progress.CompletedFiles
				lastUpdate = time.Now()
			}

			// Check if complete
			if progress.Status == statusStopped {
				fmt.Println() // New line after progress
				return
			}
		}
	}
}
