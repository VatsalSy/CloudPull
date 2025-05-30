# CloudPull Integration Plan

## Overview
This document outlines how all the components integrate to create the complete CloudPull application.

## Component Integration Map

```
┌─────────────┐     ┌──────────────┐     ┌───────────────┐
│     CLI     │────▶│ Sync Engine  │────▶│ State Manager │
└─────────────┘     └──────────────┘     └───────────────┘
       │                    │                      │
       ▼                    ▼                      ▼
┌─────────────┐     ┌──────────────┐     ┌───────────────┐
│   Config    │     │ API Client   │     │   Database    │
└─────────────┘     └──────────────┘     └───────────────┘
       │                    │
       ▼                    ▼
┌─────────────┐     ┌──────────────┐
│   Logger    │     │Rate Limiter  │
└─────────────┘     └──────────────┘
```

## Integration Points

### 1. CLI → Sync Engine
- CLI commands invoke sync engine methods
- Pass configuration and command options
- Receive progress events for display

### 2. Sync Engine → API Client
- Request file listings and metadata
- Download files with resume support
- Handle rate limiting transparently

### 3. Sync Engine → State Manager
- Persist sync progress
- Query resumable state
- Update file/folder status

### 4. API Client → Rate Limiter
- All API calls go through rate limiter
- Automatic backoff on quota errors
- Adaptive rate adjustment

### 5. All Components → Error Handler
- Consistent error handling
- Retry logic based on error type
- Logging of errors

### 6. All Components → Logger
- Structured logging throughout
- Debug/trace for troubleshooting
- Performance metrics

## Data Flow

1. **New Sync**:
   ```
   CLI sync command
   → Create session in database
   → Initialize sync engine
   → Start folder walker
   → Queue files for download
   → Download workers process queue
   → Update progress in database
   → Report progress to CLI
   ```

2. **Resume Sync**:
   ```
   CLI resume command
   → Load session from database
   → Query pending files
   → Resume folder scanning if needed
   → Continue downloads from last byte
   → Update progress
   ```

3. **Error Recovery**:
   ```
   Download error occurs
   → Error handler classifies error
   → Retry with backoff if transient
   → Update error count in database
   → Skip file if permanent error
   → Continue with next file
   ```

## Configuration Flow
- CLI reads config file via Viper
- Config passed to all components
- Environment variables override file
- Runtime flags override everything

## Testing Integration
1. Unit tests for each component
2. Integration tests with mock Drive API
3. End-to-end tests with real API (limited)
4. Performance tests with large datasets
5. Chaos tests for error scenarios

## Deployment Considerations
- Single binary distribution
- Embedded database schema
- Auto-migration on startup
- Graceful shutdown handling
- Signal handling for pause/resume