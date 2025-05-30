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
	var session *state.Session
	if len(args) > 0 {
		// Get specific session
		sessions, err := application.GetSessions(ctx)
		if err != nil {
			return fmt.Errorf("failed to get sessions: %w", err)
		}
		for _, s := range sessions {
			if s.ID == args[0] {
				session = s
				break
			}
		}
		if session == nil {
			return fmt.Errorf("session not found: %s", args[0])
		}
	} else if resumeLatest {
		session, err = application.GetLatestSession(ctx)
		if err != nil {
			return fmt.Errorf("failed to get latest session: %w", err)
		}
		if session == nil {
			return fmt.Errorf("no interrupted sessions found")
		}
	} else {
		// Show session list
		session, err = selectSessionFromApp(ctx, application)
		if err != nil {
			return err
		}
		if session == nil {
			return fmt.Errorf("no session selected")
		}
	}

	// Display session info
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
			formatBytes(session.CompletedBytes), formatBytes(session.TotalBytes))
	} else {
		fmt.Printf("  Progress: %d files completed\n", session.CompletedFiles)
	}
	fmt.Println()

	// Check session status
	if session.Status == state.SessionStatusCompleted {
		fmt.Println(color.YellowString("⚠️  Warning: Session is already completed"))
		return nil
	}

	if session.Status == state.SessionStatusFailed && !forceResume {
		fmt.Println(color.RedString("⚠️  Warning: Session failed previously"))
		var proceed bool
		prompt := &survey.Confirm{
			Message: "Attempt to resume anyway?",
			Default: false,
		}
		survey.AskOne(prompt, &proceed)
		if !proceed {
			return nil
		}
	}

	// Confirm resume
	var confirm bool
	prompt := &survey.Confirm{
		Message: "Resume this sync session?",
		Default: true,
	}
	survey.AskOne(prompt, &confirm)
	if !confirm {
		return nil
	}

	// Resume sync with progress monitoring
	errChan := make(chan error, 1)

	go func() {
		errChan <- application.ResumeSync(ctx, session.ID)
	}()

	// Monitor progress
	go monitorResumeProgress(application)

	// Wait for completion
	if err := <-errChan; err != nil {
		return fmt.Errorf("resume failed: %w", err)
	}

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
		progress := "N/A"
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
				formatBytes(session.CompletedBytes),
				formatBytes(session.TotalBytes))
		} else if session.CompletedBytes > 0 {
			size = formatBytes(session.CompletedBytes)
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
	survey.AskOne(prompt, &selected)

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

func monitorResumeProgress(app *app.App) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	lastFiles := int64(0)
	lastUpdate := time.Now()

	for {
		progress := app.GetProgress()
		if progress == nil {
			time.Sleep(time.Second)
			continue
		}

		// Update progress every second or on file completion
		if progress.CompletedFiles > lastFiles || time.Since(lastUpdate) > time.Second {
			fmt.Printf("\rProgress: %d/%d files (%.1f%%) | Speed: %s/s | Active: %d",
				progress.CompletedFiles, progress.TotalFiles,
				float64(progress.CompletedFiles)/float64(progress.TotalFiles)*100,
				formatBytes(progress.CurrentSpeed),
				progress.ActiveDownloads)
			lastFiles = progress.CompletedFiles
			lastUpdate = time.Now()
		}

		// Check if complete
		if progress.Status == "stopped" {
			fmt.Println() // New line after progress
			break
		}

		<-ticker.C
	}
}
