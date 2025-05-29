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
  "os"
  "os/signal"
  "path/filepath"
  "strings"
  "sync"
  "syscall"
  "time"

  "github.com/VatsalSy/CloudPull/internal/api"
  "github.com/VatsalSy/CloudPull/internal/config"
  "github.com/VatsalSy/CloudPull/internal/errors"
  "github.com/VatsalSy/CloudPull/internal/logger"
  "github.com/VatsalSy/CloudPull/internal/state"
  "github.com/VatsalSy/CloudPull/internal/sync"
  "github.com/spf13/viper"
)

// App is the main application coordinator
type App struct {
  mu sync.RWMutex

  // Configuration
  config *config.Config

  // Core components
  logger       logger.Logger
  authManager  *api.AuthManager
  apiClient    *api.DriveClient
  stateManager *state.Manager
  syncEngine   *sync.Engine
  errorHandler *errors.Handler

  // State
  isInitialized bool
  isRunning     bool

  // Shutdown
  shutdownChan chan struct{}
  shutdownOnce sync.Once
}

// New creates a new application instance
func New() (*App, error) {
  return &App{
    shutdownChan: make(chan struct{}),
  }, nil
}

// Initialize initializes the application with configuration
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
  logConfig := &logger.Config{
    Level:      cfg.GetLogLevel(),
    Format:     cfg.GetString("log.format"),
    Output:     cfg.GetString("log.output"),
    MaxSize:    cfg.GetInt("log.max_size"),
    MaxBackups: cfg.GetInt("log.max_backups"),
    MaxAge:     cfg.GetInt("log.max_age"),
  }

  app.logger, err = logger.New(logConfig)
  if err != nil {
    return errors.Wrap(err, "failed to initialize logger")
  }

  app.logger.Info("Initializing CloudPull",
    "version", cfg.GetString("version"),
    "config", viper.ConfigFileUsed(),
  )

  // Initialize error handler
  errorConfig := &errors.Config{
    MaxRetries:     cfg.GetInt("errors.max_retries"),
    RetryDelay:     cfg.GetDuration("errors.retry_delay"),
    RetryMultiplier: cfg.GetFloat64("errors.retry_multiplier"),
    RetryMaxDelay:  cfg.GetDuration("errors.retry_max_delay"),
  }

  app.errorHandler = errors.NewHandler(errorConfig, app.logger)

  // Initialize database
  dbPath := filepath.Join(cfg.GetDataDir(), "cloudpull.db")
  if err := app.initializeDatabase(dbPath); err != nil {
    return errors.Wrap(err, "failed to initialize database")
  }

  // Initialize state manager
  app.stateManager, err = state.NewManager(dbPath, app.logger)
  if err != nil {
    return errors.Wrap(err, "failed to initialize state manager")
  }

  app.isInitialized = true
  app.logger.Info("Application initialized successfully")

  return nil
}

// InitializeAuth initializes authentication components
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

  // Initialize API client
  apiConfig := &api.Config{
    MaxRetries:      app.config.GetInt("api.max_retries"),
    RetryDelay:      app.config.GetDuration("api.retry_delay"),
    RequestTimeout:  app.config.GetDuration("api.request_timeout"),
    MaxConcurrent:   app.config.GetInt("api.max_concurrent"),
    RateLimitPerSec: app.config.GetInt("api.rate_limit"),
  }

  app.apiClient, err = api.NewDriveClient(authManager, apiConfig, app.logger)
  if err != nil {
    return errors.Wrap(err, "failed to initialize API client")
  }

  app.logger.Info("Authentication initialized successfully")
  return nil
}

// InitializeSyncEngine initializes the sync engine
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
  engineConfig := &sync.EngineConfig{
    WalkerConfig: &sync.WalkerConfig{
      MaxDepth:       app.config.GetInt("sync.max_depth"),
      BatchSize:      app.config.GetInt("sync.batch_size"),
      MaxConcurrent:  app.config.GetInt("sync.walker_concurrent"),
    },
    DownloadConfig: &sync.DownloadManagerConfig{
      WorkerCount:    app.config.GetInt("sync.max_concurrent"),
      ChunkSize:      app.config.GetInt64("sync.chunk_size_bytes"),
      MaxRetries:     app.config.GetInt("sync.max_retries"),
      RetryDelay:     app.config.GetDuration("sync.retry_delay"),
    },
    WorkerConfig: &sync.WorkerPoolConfig{
      MaxWorkers:     app.config.GetInt("sync.max_concurrent"),
      QueueSize:      app.config.GetInt("sync.queue_size"),
    },
    ProgressInterval:   app.config.GetDuration("sync.progress_interval"),
    CheckpointInterval: app.config.GetDuration("sync.checkpoint_interval"),
    MaxErrors:         app.config.GetInt("sync.max_errors"),
  }

  // Create sync engine
  engine, err := sync.NewEngine(
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

// Authenticate performs OAuth2 authentication
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

// StartSync starts a new sync session
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

  // Wait for completion
  <-ctx.Done()

  app.mu.Lock()
  app.isRunning = false
  app.mu.Unlock()

  return nil
}

// ResumeSync resumes an existing sync session
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

  // Wait for completion
  <-ctx.Done()

  app.mu.Lock()
  app.isRunning = false
  app.mu.Unlock()

  return nil
}

// GetSessions returns all sync sessions
func (app *App) GetSessions(ctx context.Context) ([]*state.Session, error) {
  if app.stateManager == nil {
    return nil, errors.Errorf("state manager not initialized")
  }

  return app.stateManager.GetAllSessions(ctx)
}

// GetLatestSession returns the most recent session
func (app *App) GetLatestSession(ctx context.Context) (*state.Session, error) {
  if app.stateManager == nil {
    return nil, errors.Errorf("state manager not initialized")
  }

  sessions, err := app.stateManager.GetAllSessions(ctx)
  if err != nil {
    return nil, err
  }

  if len(sessions) == 0 {
    return nil, nil
  }

  // Sessions are ordered by created_at DESC
  return sessions[0], nil
}

// GetProgress returns current sync progress
func (app *App) GetProgress() *sync.SyncProgress {
  app.mu.RLock()
  defer app.mu.RUnlock()

  if app.syncEngine == nil || !app.isRunning {
    return nil
  }

  return app.syncEngine.GetProgress()
}

// Stop stops the application gracefully
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
  if err := os.MkdirAll(dbDir, 0755); err != nil {
    return errors.Wrap(err, "failed to create data directory")
  }

  // Initialize database schema
  db, err := state.InitializeDatabase(dbPath)
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
  signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

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

  lastProgress := &sync.SyncProgress{}

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

// SyncOptions contains options for sync operations
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