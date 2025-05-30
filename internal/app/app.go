/**
 * Main Application Coordinator for CloudPull
 *
 * Features:
 * - Dependency injection and initialization
 * - Component lifecycle management
 * - Graceful shutdown handling
 * - Signal handling (SIGINT/SIGTERM)
 * - Configuration management
 *
 * Author: CloudPull Team
 * Updated: 2025-01-29
 */

package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"

	"github.com/VatsalSy/CloudPull/internal/api"
	"github.com/VatsalSy/CloudPull/internal/config"
	"github.com/VatsalSy/CloudPull/internal/errors"
	"github.com/VatsalSy/CloudPull/internal/logger"
	"github.com/VatsalSy/CloudPull/internal/state"
	cloudsync "github.com/VatsalSy/CloudPull/internal/sync"
)

// App is the main application coordinator.
type App struct {
	config        *config.Config
	logger        *logger.Logger
	authManager   *api.AuthManager
	apiClient     *api.DriveClient
	stateManager  *state.Manager
	syncEngine    *cloudsync.Engine
	errorHandler  *errors.Handler
	shutdownChan  chan struct{}
	mu            sync.RWMutex
	shutdownOnce  sync.Once
	isInitialized bool
	isRunning     bool
}

// New creates a new application instance.
func New() (*App, error) {
	return &App{
		shutdownChan: make(chan struct{}),
	}, nil
}

// Initialize initializes the application with configuration.
func (app *App) Initialize() error {
	app.mu.Lock()
	defer app.mu.Unlock()

	if app.isInitialized {
		return errors.Errorf("application already initialized")
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return errors.Wrap(err, "failed to load configuration")
	}
	app.config = cfg

	// Initialize logger
	// Create output writer based on config
	var output io.Writer = os.Stdout
	outputPath := cfg.GetString("log.output")
	if outputPath != "" && outputPath != "stdout" {
		file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return errors.Wrap(err, "failed to open log file")
		}
		output = file
	}

	logConfig := &logger.Config{
		Level:         cfg.GetLogLevel(),
		Output:        output,
		Pretty:        cfg.GetString("log.format") == "pretty",
		IncludeCaller: true,
	}

	app.logger = logger.New(logConfig)
	if app.logger == nil {
		return errors.NewSimple("failed to initialize logger")
	}

	app.logger.Info("Initializing CloudPull",
		"version", cfg.GetString("version"),
		"config", viper.ConfigFileUsed(),
	)

	// Initialize error handler
	app.errorHandler = errors.NewHandler(app.logger)

	// Initialize database
	dbPath := filepath.Join(cfg.GetDataDir(), "cloudpull.db")
	if err := app.initializeDatabase(dbPath); err != nil {
		return errors.Wrap(err, "failed to initialize database")
	}

	// Initialize state manager
	dbConfig := state.DefaultConfig()
	dbConfig.Path = dbPath
	app.stateManager, err = state.NewManager(dbConfig)
	if err != nil {
		return errors.Wrap(err, "failed to initialize state manager")
	}

	app.isInitialized = true
	app.logger.Info("Application initialized successfully")

	return nil
}

// InitializeAuth initializes authentication components.
func (app *App) InitializeAuth() error {
	app.mu.Lock()
	defer app.mu.Unlock()

	if !app.isInitialized {
		return errors.Errorf("application not initialized")
	}

	if app.authManager != nil {
		return nil // Already initialized
	}

	// Get credentials path
	credentialsPath := app.config.GetString("credentials_file")
	if credentialsPath == "" {
		return errors.Errorf("credentials file not configured")
	}

	// Expand path
	credentialsPath = app.expandPath(credentialsPath)

	// Check if file exists
	if _, err := os.Stat(credentialsPath); err != nil {
		return errors.Wrap(err, "credentials file not found")
	}

	// Get token path
	tokenPath := filepath.Join(app.config.GetDataDir(), "token.json")

	// Initialize auth manager
	authManager, err := api.NewAuthManager(credentialsPath, tokenPath, app.logger)
	if err != nil {
		return errors.Wrap(err, "failed to initialize auth manager")
	}

	app.authManager = authManager

	// Get Drive service
	driveService, err := authManager.GetDriveService(context.Background())
	if err != nil {
		return errors.Wrap(err, "failed to get drive service")
	}

	// Initialize rate limiter
	rateLimiterConfig := &api.RateLimiterConfig{
		RateLimit:       app.config.GetInt("api.rate_limit"),
		BurstSize:       app.config.GetInt("api.rate_limit") * 2,
		BatchRateLimit:  app.config.GetInt("api.rate_limit") / 2,
		ExportRateLimit: app.config.GetInt("api.rate_limit") / 4,
	}
	rateLimiter := api.NewRateLimiter(rateLimiterConfig)

	// Initialize API client
	app.apiClient = api.NewDriveClient(driveService, rateLimiter, app.logger)

	app.logger.Info("Authentication initialized successfully")
	return nil
}

// InitializeSyncEngine initializes the sync engine.
func (app *App) InitializeSyncEngine() error {
	app.mu.Lock()
	defer app.mu.Unlock()

	if !app.isInitialized {
		return errors.Errorf("application not initialized")
	}

	if app.apiClient == nil {
		return errors.Errorf("API client not initialized")
	}

	if app.syncEngine != nil {
		return nil // Already initialized
	}

	// Create sync engine configuration
	engineConfig := &cloudsync.EngineConfig{
		WalkerConfig: &cloudsync.WalkerConfig{
			MaxDepth:          app.config.GetInt("sync.max_depth"),
			Strategy:          cloudsync.TraversalBFS,
			Concurrency:       3, // Number of concurrent folder scanners
			ChannelBufferSize: 100,
		},
		DownloadConfig: &cloudsync.DownloadManagerConfig{
			MaxConcurrent:   app.config.GetInt("sync.max_concurrent"),
			ChunkSize:       app.config.GetInt64("sync.chunk_size_bytes"),
			VerifyChecksums: true,
			TempDir:         app.config.GetString("sync.temp_dir"),
		},
		WorkerConfig: &cloudsync.WorkerPoolConfig{
			WorkerCount:     app.config.GetInt("sync.max_concurrent"),
			MaxRetries:      app.config.GetInt("sync.max_retries"),
			ShutdownTimeout: app.config.GetDuration("sync.shutdown_timeout"),
		},
		ProgressInterval:   app.config.GetDuration("sync.progress_interval"),
		CheckpointInterval: app.config.GetDuration("sync.checkpoint_interval"),
		MaxErrors:          app.config.GetInt("sync.max_errors"),
	}

	// Create sync engine
	engine, err := cloudsync.NewEngine(
		app.apiClient,
		app.stateManager,
		app.errorHandler,
		app.logger,
		engineConfig,
	)
	if err != nil {
		return errors.Wrap(err, "failed to create sync engine")
	}

	app.syncEngine = engine
	app.logger.Info("Sync engine initialized successfully")

	return nil
}

// Authenticate performs OAuth2 authentication.
func (app *App) Authenticate(ctx context.Context) error {
	if app.authManager == nil {
		if err := app.InitializeAuth(); err != nil {
			return err
		}
	}

	// Check if already authenticated
	if app.authManager.IsAuthenticated() {
		app.logger.Info("Already authenticated")
		return nil
	}

	// Perform authentication
	_, err := app.authManager.GetClient(ctx)
	if err != nil {
		return errors.Wrap(err, "authentication failed")
	}

	app.logger.Info("Authentication successful")
	return nil
}

// RevokeAuth revokes the current authentication.
func (app *App) RevokeAuth(ctx context.Context) error {
	if app.authManager == nil {
		if err := app.InitializeAuth(); err != nil {
			return err
		}
	}

	// Revoke the token
	if err := app.authManager.RevokeToken(ctx); err != nil {
		return errors.Wrap(err, "failed to revoke authentication")
	}

	app.logger.Info("Authentication revoked successfully")
	return nil
}

// IsAuthenticated checks if the user is already authenticated.
func (app *App) IsAuthenticated() bool {
	if app.authManager == nil {
		return false
	}
	return app.authManager.IsAuthenticated()
}

// StartSync starts a new sync session.
func (app *App) StartSync(ctx context.Context, folderID, outputDir string, options *SyncOptions) error {
	if err := app.ensureReady(); err != nil {
		return err
	}

	app.mu.Lock()
	if app.isRunning {
		app.mu.Unlock()
		return errors.Errorf("sync already running")
	}
	app.isRunning = true
	app.mu.Unlock()

	// Apply options
	if options != nil {
		app.applySyncOptions(options)
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Setup signal handling
	go app.handleSignals(cancel)

	// Start sync engine
	if err := app.syncEngine.StartNewSession(ctx, folderID, outputDir); err != nil {
		app.mu.Lock()
		app.isRunning = false
		app.mu.Unlock()
		return errors.Wrap(err, "failed to start sync")
	}

	// Monitor progress
	go app.monitorProgress(ctx)

	// Wait for completion or cancellation
	select {
	case <-app.syncEngine.WaitForCompletion():
		// Sync completed naturally
		app.logger.Info("Sync completed")
	case <-ctx.Done():
		// Context canceled (user interrupt)
		app.logger.Info("Sync canceled")
		app.syncEngine.Stop()
	}

	app.mu.Lock()
	app.isRunning = false
	app.mu.Unlock()

	return nil
}

// StartSyncWithSession starts a new sync session and returns the session ID.
func (app *App) StartSyncWithSession(ctx context.Context, folderID, outputDir string, options *SyncOptions) (string, error) {
	if err := app.ensureReady(); err != nil {
		return "", err
	}

	app.mu.Lock()
	if app.isRunning {
		app.mu.Unlock()
		return "", errors.Errorf("sync already running")
	}
	app.isRunning = true
	app.mu.Unlock()

	// Apply options
	if options != nil {
		app.applySyncOptions(options)
	}

	// Start sync engine and get session ID
	sessionID, err := app.syncEngine.StartNewSessionWithID(ctx, folderID, outputDir)
	if err != nil {
		app.mu.Lock()
		app.isRunning = false
		app.mu.Unlock()
		return "", errors.Wrap(err, "failed to start sync")
	}

	// Monitor progress
	go app.monitorProgress(ctx)

	// Wait for completion or cancellation in background
	go func() {
		select {
		case <-app.syncEngine.WaitForCompletion():
			// Sync completed naturally
			app.logger.Info("Sync completed")
		case <-ctx.Done():
			// Context canceled (user interrupt)
			app.logger.Info("Sync canceled")
			app.syncEngine.Stop()
		}

		app.mu.Lock()
		app.isRunning = false
		app.mu.Unlock()
	}()

	return sessionID, nil
}

// ResumeSync resumes an existing sync session.
func (app *App) ResumeSync(ctx context.Context, sessionID string) error {
	if err := app.ensureReady(); err != nil {
		return err
	}

	app.mu.Lock()
	if app.isRunning {
		app.mu.Unlock()
		return errors.Errorf("sync already running")
	}
	app.isRunning = true
	app.mu.Unlock()

	// Create context with cancellation
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Setup signal handling
	go app.handleSignals(cancel)

	// Resume sync engine
	if err := app.syncEngine.ResumeSession(ctx, sessionID); err != nil {
		app.mu.Lock()
		app.isRunning = false
		app.mu.Unlock()
		return errors.Wrap(err, "failed to resume sync")
	}

	// Monitor progress
	go app.monitorProgress(ctx)

	// Wait for completion or cancellation
	select {
	case <-app.syncEngine.WaitForCompletion():
		// Sync completed naturally
		app.logger.Info("Sync completed")
	case <-ctx.Done():
		// Context canceled (user interrupt)
		app.logger.Info("Sync canceled")
		app.syncEngine.Stop()
	}

	app.mu.Lock()
	app.isRunning = false
	app.mu.Unlock()

	return nil
}

// GetSessions returns all sync sessions.
func (app *App) GetSessions(ctx context.Context) ([]*state.Session, error) {
	if app.stateManager == nil {
		return nil, errors.Errorf("state manager not initialized")
	}

	// Get up to 100 recent sessions
	return app.stateManager.Sessions().List(ctx, 100, 0)
}

// GetLatestSession returns the most recent session.
func (app *App) GetLatestSession(ctx context.Context) (*state.Session, error) {
	if app.stateManager == nil {
		return nil, errors.Errorf("state manager not initialized")
	}

	sessions, err := app.stateManager.Sessions().List(ctx, 100, 0)
	if err != nil {
		return nil, err
	}

	if len(sessions) == 0 {
		return nil, nil
	}

	// Sessions are ordered by created_at DESC
	return sessions[0], nil
}

// GetProgress returns current sync progress.
func (app *App) GetProgress() *cloudsync.SyncProgress {
	app.mu.RLock()
	defer app.mu.RUnlock()

	if app.syncEngine == nil || !app.isRunning {
		return nil
	}

	return app.syncEngine.GetProgress()
}

// Stop stops the application gracefully.
func (app *App) Stop() error {
	app.shutdownOnce.Do(func() {
		close(app.shutdownChan)

		app.mu.Lock()
		defer app.mu.Unlock()

		app.logger.Info("Shutting down CloudPull...")

		// Stop sync engine if running
		if app.syncEngine != nil && app.isRunning {
			if err := app.syncEngine.Stop(); err != nil {
				app.logger.Error(err, "Failed to stop sync engine")
			}
		}

		// Close state manager
		if app.stateManager != nil {
			if err := app.stateManager.Close(); err != nil {
				app.logger.Error(err, "Failed to close state manager")
			}
		}

		app.logger.Info("CloudPull shutdown complete")
	})

	return nil
}

// Private methods

func (app *App) initializeDatabase(dbPath string) error {
	// Ensure directory exists
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0750); err != nil {
		return errors.Wrap(err, "failed to create data directory")
	}

	// Create database config
	dbConfig := state.DefaultConfig()
	dbConfig.Path = dbPath

	// Initialize database
	db, err := state.NewDB(dbConfig)
	if err != nil {
		return err
	}
	defer db.Close()

	app.logger.Info("Database initialized", "path", dbPath)
	return nil
}

func (app *App) ensureReady() error {
	if !app.isInitialized {
		return errors.Errorf("application not initialized")
	}

	if app.authManager == nil {
		if err := app.InitializeAuth(); err != nil {
			return err
		}
	}

	if app.syncEngine == nil {
		if err := app.InitializeSyncEngine(); err != nil {
			return err
		}
	}

	return nil
}

func (app *App) handleSignals(cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	app.setupSignalHandling(sigChan)

	select {
	case sig := <-sigChan:
		app.logger.Info("Received signal", "signal", sig)
		cancel()
	case <-app.shutdownChan:
		cancel()
	}
}

func (app *App) monitorProgress(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	lastProgress := &cloudsync.SyncProgress{}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			progress := app.GetProgress()
			if progress == nil {
				continue
			}

			// Only log significant changes
			if progress.CompletedFiles != lastProgress.CompletedFiles ||
				progress.FailedFiles != lastProgress.FailedFiles {

				app.logger.Info("Sync progress",
					"completed", progress.CompletedFiles,
					"failed", progress.FailedFiles,
					"total", progress.TotalFiles,
					"speed", formatBytes(progress.CurrentSpeed)+"/s",
				)
				lastProgress = progress
			}
		}
	}
}

func (app *App) applySyncOptions(options *SyncOptions) {
	// Apply include/exclude patterns
	if len(options.IncludePatterns) > 0 || len(options.ExcludePatterns) > 0 {
		// TODO: Pass patterns to sync engine
		app.logger.Info("Filter patterns applied",
			"include", options.IncludePatterns,
			"exclude", options.ExcludePatterns,
		)
	}

	// Apply bandwidth limit
	if options.BandwidthLimit > 0 {
		// TODO: Configure rate limiter
		app.logger.Info("Bandwidth limit applied",
			"limit", formatBytes(options.BandwidthLimit)+"/s",
		)
	}
}

func (app *App) expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[2:])
	}
	return path
}

// SyncOptions contains options for sync operations.
type SyncOptions struct {
	IncludePatterns []string
	ExcludePatterns []string
	MaxDepth        int
	BandwidthLimit  int64
	DryRun          bool
}

// Helper functions

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

// GetAllSessions returns all sync sessions.
func (app *App) GetAllSessions() ([]*state.Session, error) {
	if app.stateManager == nil {
		return nil, errors.NewSimple("state manager not initialized")
	}

	ctx := context.Background()
	return app.stateManager.GetAllSessions(ctx)
}

// IsSessionRunning checks if a session is currently running.
func (app *App) IsSessionRunning(sessionID string) bool {
	app.mu.RLock()
	defer app.mu.RUnlock()

	if !app.isRunning || app.syncEngine == nil {
		return false
	}

	// Check if this is the current session ID
	currentProgress := app.GetProgress()
	return currentProgress != nil && currentProgress.SessionID == sessionID
}

// CleanupSession removes a stuck or inactive session.
func (app *App) CleanupSession(sessionID string) error {
	if app.stateManager == nil {
		return errors.NewSimple("state manager not initialized")
	}

	ctx := context.Background()

	// First update the session status to canceled if it's still active
	session, err := app.stateManager.GetSession(ctx, sessionID)
	if err != nil {
		return errors.Wrap(err, "failed to get session")
	}

	if session.Status == state.SessionStatusActive {
		if err := app.stateManager.UpdateSessionStatus(ctx, sessionID, state.SessionStatusCancelled); err != nil {
			return errors.Wrap(err, "failed to update session status")
		}
	}

	// TODO: Add method to delete or archive session if needed
	return nil
}

// GetSyncEngine returns the sync engine.
func (app *App) GetSyncEngine() *cloudsync.Engine {
	app.mu.RLock()
	defer app.mu.RUnlock()
	return app.syncEngine
}
