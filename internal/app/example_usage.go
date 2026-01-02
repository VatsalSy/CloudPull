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
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/VatsalSy/CloudPull/internal/util"
)

// Example demonstrates complete app usage.
func Example() {
	// Create application instance
	app, err := New()
	if err != nil {
		log.Printf("Failed to create app: %v", err)
		return
	}
	defer app.Stop()

	// Initialize application
	if err := app.Initialize(); err != nil {
		log.Printf("Failed to initialize app: %v", err)
		return
	}

	// Initialize authentication
	if err := app.InitializeAuth(); err != nil {
		log.Printf("Failed to initialize auth: %v", err)
		return
	}

	// Initialize sync engine
	if err := app.InitializeSyncEngine(); err != nil {
		log.Printf("Failed to initialize sync engine: %v", err)
		return
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
		// Create context with 5-minute timeout for sync operation
		syncCtx, syncCancel := context.WithTimeout(ctx, 5*time.Minute)
		defer syncCancel() // Ensure context is canceled to avoid resource leaks

		options := &SyncOptions{
			IncludePatterns: []string{"*.pdf", "*.docx"},
			ExcludePatterns: []string{"temp/*", "*.tmp"},
			MaxDepth:        5,
		}

		if err := app.StartSync(syncCtx, "root", "~/CloudPull/MyDrive", options); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				log.Printf("Sync timed out after 5 minutes")
			} else {
				log.Printf("Sync failed: %v", err)
			}
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
					util.FormatBytes(progress.CurrentSpeed),
				)
			}
		}
	}
}

// ExampleResumeSession shows how to resume a session.
func ExampleResumeSession() {
	// Create application instance with proper error handling
	app, err := New()
	if err != nil {
		log.Printf("Failed to create app: %v", err)
		return
	}

	// Initialize application components with error checking
	if err := app.Initialize(); err != nil {
		log.Printf("Failed to initialize app: %v", err)
		return
	}

	if err := app.InitializeAuth(); err != nil {
		log.Printf("Failed to initialize auth: %v", err)
		return
	}

	if err := app.InitializeSyncEngine(); err != nil {
		log.Printf("Failed to initialize sync engine: %v", err)
		return
	}

	// Create context with timeout for database operations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get latest session
	session, err := app.GetLatestSession(ctx)
	if err != nil {
		log.Printf("Failed to get latest session: %v", err)
		return
	}

	if session == nil {
		fmt.Println("No sessions found")
		return
	}

	// Resume the session with a longer timeout for sync operations
	fmt.Printf("Resuming session %s...\n", session.ID)
	syncCtx, syncCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer syncCancel()
	if err := app.ResumeSync(syncCtx, session.ID); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("Resume sync timed out after 10 minutes")
		} else {
			log.Printf("Failed to resume sync: %v", err)
		}
		return
	}
}

// ExampleAuthentication shows authentication flow.
func ExampleAuthentication() {
	// Create application instance with proper error handling
	app, err := New()
	if err != nil {
		log.Printf("Failed to create app: %v", err)
		return
	}

	// Initialize application
	if err := app.Initialize(); err != nil {
		log.Printf("Failed to initialize app: %v", err)
		return
	}

	// Initialize authentication separately to show the process
	if err := app.InitializeAuth(); err != nil {
		log.Printf("Failed to initialize auth: %v", err)
		return
	}

	// Create context with timeout for authentication
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Authenticate
	fmt.Println("Starting authentication...")
	if err := app.Authenticate(ctx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("Authentication timed out after 2 minutes")
		} else {
			log.Printf("Authentication failed: %v", err)
		}
		return
	}

	fmt.Println("Authentication successful!")
}

// ExampleListSessions shows how to list all sessions.
func ExampleListSessions() {
	// Create application instance with proper error handling
	app, err := New()
	if err != nil {
		log.Printf("Failed to create app: %v", err)
		return
	}

	// Initialize application (authentication not needed for listing sessions)
	if err := app.Initialize(); err != nil {
		log.Printf("Failed to initialize app: %v", err)
		return
	}

	// Create context with timeout for database operations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get all sessions
	sessions, err := app.GetSessions(ctx)
	if err != nil {
		log.Printf("Failed to get sessions: %v", err)
		return
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

// ExampleRobustErrorHandling demonstrates production-ready error handling patterns.
func ExampleRobustErrorHandling() {
	// Pattern 1: Return errors to caller for flexible handling
	runSync := func() error {
		app, err := New()
		if err != nil {
			return fmt.Errorf("failed to create app: %w", err)
		}

		if err := app.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize app: %w", err)
		}

		if err := app.InitializeAuth(); err != nil {
			return fmt.Errorf("failed to initialize auth: %w", err)
		}

		if err := app.InitializeSyncEngine(); err != nil {
			return fmt.Errorf("failed to initialize sync engine: %w", err)
		}

		// Create context with timeout for sync operation
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		options := &SyncOptions{
			IncludePatterns: []string{"*.pdf"},
			MaxDepth:        3,
		}

		if err := app.StartSync(ctx, "root", "~/CloudPull/Documents", options); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("sync timed out after 5 minutes")
			}
			return fmt.Errorf("sync failed: %w", err)
		}

		return nil
	}

	// Pattern 2: Retry logic for transient failures
	var lastErr error
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		if err := runSync(); err != nil {
			lastErr = err
			log.Printf("Attempt %d/%d failed: %v", i+1, maxRetries, err)
			if i < maxRetries-1 {
				time.Sleep(time.Second * time.Duration(i+1)) // Exponential backoff
			}
			continue
		}
		// Success
		fmt.Println("Sync completed successfully")
		return
	}

	// All retries failed
	log.Printf("Sync failed after %d attempts: %v", maxRetries, lastErr)
}

// ExampleGracefulDegradation shows handling partial failures.
func ExampleGracefulDegradation() {
	app, err := New()
	if err != nil {
		log.Printf("Failed to create app: %v", err)
		return
	}

	// Initialize core components
	if err := app.Initialize(); err != nil {
		log.Printf("Failed to initialize app: %v", err)
		return
	}

	// Try to initialize auth, but continue if it fails
	authErr := app.InitializeAuth()
	if authErr != nil {
		log.Printf("Warning: Authentication initialization failed: %v", authErr)
		log.Println("Continuing with limited functionality...")
	}

	// Initialize sync engine
	if err := app.InitializeSyncEngine(); err != nil {
		log.Printf("Failed to initialize sync engine: %v", err)
		return
	}

	// Create context with timeout for operations
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	// If auth failed earlier, try to authenticate now
	if authErr != nil {
		log.Println("Attempting authentication...")
		authCtx, authCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer authCancel()
		if err := app.Authenticate(authCtx); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				log.Printf("Authentication timed out after 2 minutes")
			} else {
				log.Printf("Authentication failed: %v", err)
			}
			log.Println("Some features may be unavailable")
		}
	}

	// Continue with available functionality
	sessions, err := app.GetSessions(ctx)
	if err != nil {
		log.Printf("Failed to get sessions: %v", err)
	} else {
		fmt.Printf("Found %d sessions\n", len(sessions))
	}
}
