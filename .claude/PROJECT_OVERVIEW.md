# CloudPull - Google Drive Sync Tool

## Project Overview

CloudPull is a high-performance, resumable Google Drive synchronization tool written in Go. It provides rsync-like functionality for downloading large folder structures from Google Drive to local storage.

## Key Features

- **Resumable Downloads**: Interrupt and resume at any time
- **Massive Folder Support**: Handle millions of files efficiently
- **Rate Limiting**: Respect Google Drive API quotas
- **Concurrent Downloads**: Maximize throughput with parallel transfers
- **State Management**: SQLite-based tracking for reliability
- **Cross-Platform**: Single binary for Windows, Mac, and Linux

## Technology Stack

- **Language**: Go 1.21+
- **Database**: SQLite for state management
- **CLI**: Cobra framework
- **Google API**: Official google.golang.org/api/drive/v3

## Project Status

- [ ] Planning Phase (In Progress)
- [ ] Core Infrastructure
- [ ] Google Drive Integration
- [ ] Sync Engine Implementation
- [ ] Testing & Optimization
- [ ] Documentation & Release

## Directory Structure

```text
CloudPull/
├── .claude/          # Project planning documents
├── cmd/              # CLI entry points
├── internal/         # Private application code
├── pkg/              # Public libraries
├── configs/          # Configuration files
└── tests/            # Test suites
```

## Development Phases

1. **Phase 1**: Core infrastructure and state management
2. **Phase 2**: Google Drive API integration
3. **Phase 3**: Sync engine and download manager
4. **Phase 4**: Error handling and recovery
5. **Phase 5**: Performance optimization and testing

## Success Metrics

- Handle folders with 1M+ files
- Achieve 90%+ of available bandwidth utilization
- 100% resume success rate after interruption
- < 100MB memory usage for typical operations
