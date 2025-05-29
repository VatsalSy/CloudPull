# CloudPull Integration Guide

This document explains how all CloudPull components work together to provide a seamless file synchronization experience.

## Architecture Overview

CloudPull follows a modular architecture with clear separation of concerns:

```
┌─────────────────────────────────────────────────────────────┐
│                         CLI Layer                             │
│  (cmd/cloudpull/*)                                           │
│  - User commands (init, auth, sync, resume)                  │
│  - Progress display                                           │
│  - Configuration management                                   │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│                    App Coordinator                           │
│  (internal/app/app.go)                                       │
│  - Dependency injection                                      │
│  - Component lifecycle management                            │
│  - Signal handling                                           │
│  - Configuration loading                                     │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│                   Core Components                            │
├─────────────────────────────────────────────────────────────┤
│  Sync Engine          │  API Client      │  State Manager   │
│  - Orchestration      │  - Google Drive  │  - Database      │
│  - Progress tracking  │  - Rate limiting │  - Session mgmt  │
│  - Worker pools       │  - Auth flow     │  - File tracking │
├─────────────────────────────────────────────────────────────┤
│         Error Handler        │         Logger               │
│  - Retry logic              │  - Structured logging        │
│  - Error categorization     │  - Multiple outputs          │
└─────────────────────────────────────────────────────────────┘
```

## Component Integration

### 1. Application Initialization

When CloudPull starts, the following initialization sequence occurs:

```go
app := app.New()
app.Initialize()           // Logger, Error Handler, Database
app.InitializeAuth()       // Auth Manager, API Client
app.InitializeSyncEngine() // Sync Engine with all dependencies
```

### 2. Authentication Flow

```
User → CLI → App → AuthManager → Google OAuth2 → Token Storage
                                       ↓
                              API Client (authenticated)
```

### 3. Sync Operation Flow

```
CLI Command → App.StartSync() → SyncEngine.StartNewSession()
                                      ↓
                              ┌──────────────┐
                              │ Folder Walker │ → Discovers files
                              └──────┬───────┘
                                     ↓
                              ┌──────────────┐
                              │Download Mgr  │ → Downloads files
                              └──────┬───────┘
                                     ↓
                              ┌──────────────┐
                              │State Manager │ → Tracks progress
                              └──────────────┘
```

### 4. Progress Monitoring

The sync engine emits progress events that flow through:

```
SyncEngine → ProgressTracker → App.GetProgress() → CLI Display
```

### 5. Error Handling

Errors are handled at multiple levels:

```
API Error → Error Handler → Retry Logic → Success/Failure
                                ↓
                          State Manager (tracks failures)
                                ↓
                          Progress Updates
```

## Key Integration Points

### CLI → App Coordinator

The CLI commands interact with the app coordinator through well-defined methods:

- `app.Initialize()` - Set up core components
- `app.Authenticate()` - Handle OAuth2 flow
- `app.StartSync()` - Begin new sync operation
- `app.ResumeSync()` - Continue interrupted sync
- `app.GetProgress()` - Monitor sync progress
- `app.Stop()` - Graceful shutdown

### App Coordinator → Components

The app coordinator manages component lifecycle and dependency injection:

```go
// Create sync engine with all dependencies
engine := sync.NewEngine(
    apiClient,      // For Drive API calls
    stateManager,   // For persistence
    errorHandler,   // For retry logic
    logger,         // For logging
    config,         // For settings
)
```

### Component Communication

Components communicate through:

1. **Direct method calls** - For synchronous operations
2. **Channels** - For async events and progress updates
3. **Context propagation** - For cancellation and timeouts
4. **Shared interfaces** - For loose coupling

## Configuration Flow

Configuration flows from CLI flags → Viper → Config struct → Components:

```
CLI Flags
    ↓
Viper Configuration
    ↓
config.Config struct
    ↓
Component configs (EngineConfig, APIConfig, etc.)
```

## Session Management

Sessions provide resumable sync operations:

1. **Create Session**: New sync creates session in database
2. **Track Progress**: File completions update session state
3. **Checkpoint**: Periodic saves ensure progress isn't lost
4. **Resume**: Load session and continue from last checkpoint

## Graceful Shutdown

The app handles shutdown gracefully:

1. **Signal received** (SIGINT/SIGTERM)
2. **Context cancelled** propagates to all components
3. **Workers finish** current operations
4. **State saved** to database
5. **Resources cleaned up**

## Example Integration

Here's a complete example showing the integration:

```go
// Initialize application
app := app.New()
app.Initialize()
app.InitializeAuth()
app.InitializeSyncEngine()

// Start sync with options
options := &app.SyncOptions{
    IncludePatterns: []string{"*.pdf"},
    ExcludePatterns: []string{"temp/*"},
    MaxDepth: 5,
}

// Run sync
ctx := context.Background()
err := app.StartSync(ctx, "folderID", "~/output", options)

// Monitor progress
for {
    progress := app.GetProgress()
    fmt.Printf("Progress: %d/%d files\n", 
        progress.CompletedFiles, 
        progress.TotalFiles)
    
    if progress.Status == "stopped" {
        break
    }
    time.Sleep(time.Second)
}

// Cleanup
app.Stop()
```

## Testing Integration

Integration can be tested at multiple levels:

1. **Unit tests** - Test individual components
2. **Integration tests** - Test component interactions
3. **End-to-end tests** - Test complete workflows
4. **Manual testing** - Use example programs

See `internal/app/app_test.go` for integration test examples.

## Extending CloudPull

To add new features:

1. **Add to appropriate component** (e.g., new download strategy)
2. **Update interfaces** if needed
3. **Wire through app coordinator**
4. **Add CLI command/flag**
5. **Update configuration**
6. **Add tests**

The modular architecture makes it easy to extend CloudPull without affecting existing functionality.