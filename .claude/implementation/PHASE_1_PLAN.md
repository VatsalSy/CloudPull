# Phase 1: Core Infrastructure Implementation Plan

## Overview
Phase 1 focuses on building the foundational components that all other features will depend on.

## Components to Build

### 1. Project Setup
- [ ] Initialize Go module: `go mod init github.com/yourusername/cloudpull`
- [ ] Set up directory structure
- [ ] Configure development tools (linting, formatting)
- [ ] Create Makefile for common tasks

### 2. Configuration System
**File**: `internal/config/config.go`

```go
type Config struct {
    Google    GoogleConfig
    Download  DownloadConfig
    RateLimit RateLimitConfig
    Storage   StorageConfig
    Logging   LoggingConfig
}

type GoogleConfig struct {
    CredentialsPath string
    TokenPath       string
    ExportFormats   map[string]string
}

type DownloadConfig struct {
    ConcurrentDownloads int
    ChunkSize          int64
    BandwidthLimit     int64
    VerifyChecksums    bool
    ResumePartial      bool
}
```

### 3. State Management Database
**File**: `internal/state/database.go`

Key Methods:
- `InitDatabase(path string) (*StateDB, error)`
- `CreateSession(rootFolderID, destination string) (*Session, error)`
- `GetOrCreateFile(driveID string, metadata FileMetadata) (*File, error)`
- `UpdateFileStatus(fileID string, status Status, bytesDownloaded int64) error`
- `GetPendingFiles(limit int) ([]*File, error)`
- `GetResumePoint(fileID string) (int64, error)`

### 4. Logging System
**File**: `internal/logger/logger.go`

Using zerolog for structured logging:
```go
type Logger struct {
    *zerolog.Logger
}

func NewLogger(config LoggingConfig) *Logger
func (l *Logger) WithComponent(component string) *Logger
func (l *Logger) TrackProgress(current, total int64)
```

### 5. Error Handling Framework
**File**: `internal/errors/errors.go`

```go
type ErrorType int

const (
    ErrNetwork ErrorType = iota
    ErrRateLimit
    ErrPermission
    ErrNotFound
    ErrQuotaExceeded
    ErrStorageFull
    ErrCorrupted
)

type CloudPullError struct {
    Type    ErrorType
    Message string
    Wrapped error
    Retry   bool
}
```

## Implementation Order

1. **Week 1**: Project setup and configuration system
2. **Week 1**: Database schema and basic state management
3. **Week 2**: Logging system integration
4. **Week 2**: Error handling framework

## Testing Requirements

### Unit Tests
- Configuration loading and validation
- Database operations (using in-memory SQLite)
- Error type classification
- Logger output formatting

### Integration Tests
- Full state management workflow
- Configuration file parsing
- Database migration handling

## Success Criteria
- [ ] All unit tests passing
- [ ] Database can handle 100k file entries efficiently
- [ ] Configuration system supports all planned features
- [ ] Logging provides sufficient debugging information
- [ ] Error types cover all known failure modes