/**
 * Example Usage of CloudPull Application Coordinator
 *
 * This file demonstrates how to use the app package to coordinate
 * all CloudPull components for a complete sync operation.
 *
 * Author: CloudPull Team
 * Updated: 2025-01-29
 */

package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Example demonstrates complete app usage.
func Example() {
	// Create application instance
	app, err := New()
	if err != nil {
		log.Fatal("Failed to create app:", err)
	}

	// Initialize application
	if err := app.Initialize(); err != nil {
		log.Fatal("Failed to initialize app:", err)
	}

	// Initialize authentication
	if err := app.InitializeAuth(); err != nil {
		log.Fatal("Failed to initialize auth:", err)
	}

	// Initialize sync engine
	if err := app.InitializeSyncEngine(); err != nil {
		log.Fatal("Failed to initialize sync engine:", err)
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		fmt.Printf("\nReceived signal: %v\n", sig)
		cancel()
	}()

	// Example 1: Start new sync
	fmt.Println("Starting new sync...")
	go func() {
		options := &SyncOptions{
			IncludePatterns: []string{"*.pdf", "*.docx"},
			ExcludePatterns: []string{"temp/*", "*.tmp"},
			MaxDepth:        5,
		}

		if err := app.StartSync(ctx, "root", "~/CloudPull/MyDrive", options); err != nil {
			log.Printf("Sync failed: %v", err)
		}
	}()

	// Monitor progress
	progressTicker := time.NewTicker(2 * time.Second)
	defer progressTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nStopping application...")
			app.Stop()
			return
		case <-progressTicker.C:
			progress := app.GetProgress()
			if progress != nil {
				fmt.Printf("Progress: %d/%d files (%.1f%%) | Speed: %s/s\n",
					progress.CompletedFiles,
					progress.TotalFiles,
					float64(progress.CompletedFiles)/float64(progress.TotalFiles)*100,
					formatBytes(progress.CurrentSpeed),
				)
			}
		}
	}
}

// formatBytes converts bytes to human-readable format.
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

// ExampleResumeSession shows how to resume a session.
func ExampleResumeSession() {
	// NOTE: Error handling below uses log.Fatal for simplicity in this example.
	// Production code should implement more robust error handling strategies
	// such as returning errors to the caller, graceful degradation, or retry logic.
	app, _ := New()
	app.Initialize()
	app.InitializeAuth()
	app.InitializeSyncEngine()

	ctx := context.Background()

	// Get latest session
	session, err := app.GetLatestSession(ctx)
	if err != nil {
		log.Fatal("Failed to get latest session:", err)
	}

	if session == nil {
		fmt.Println("No sessions found")
		return
	}

	// Resume the session
	fmt.Printf("Resuming session %s...\n", session.ID)
	if err := app.ResumeSync(ctx, session.ID); err != nil {
		log.Fatal("Failed to resume sync:", err)
	}
}

// ExampleAuthentication shows authentication flow.
func ExampleAuthentication() {
	app, _ := New()
	app.Initialize()
	app.InitializeAuth()

	ctx := context.Background()

	// Authenticate
	fmt.Println("Starting authentication...")
	if err := app.Authenticate(ctx); err != nil {
		log.Fatal("Authentication failed:", err)
	}

	fmt.Println("Authentication successful!")
}

// ExampleListSessions shows how to list all sessions.
func ExampleListSessions() {
	app, _ := New()
	app.Initialize()

	ctx := context.Background()

	// Get all sessions
	sessions, err := app.GetSessions(ctx)
	if err != nil {
		log.Fatal("Failed to get sessions:", err)
	}

	fmt.Printf("Found %d sessions:\n", len(sessions))
	for _, session := range sessions {
		fmt.Printf("  - %s: %s (Status: %s, Files: %d/%d)\n",
			session.ID,
			session.StartTime.Format("2006-01-02 15:04"),
			session.Status,
			session.CompletedFiles,
			session.TotalFiles,
		)
	}
}
