# CloudPull Technical Design Document

## Architecture Overview

### Core Components

```text
┌─────────────────────────────────────────────────────────┐
│                   CloudPull Main                         │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │   CLI/UI    │  │ Sync Engine  │  │ State Manager │  │
│  └─────────────┘  └──────────────┘  └───────────────┘  │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │ Rate Limiter│  │ File Handler │  │ Error Handler │  │
│  └─────────────┘  └──────────────┘  └───────────────┘  │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────┐   │
│  │            Google Drive API Client               │   │
│  └─────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

## Component Specifications

### 1. CLI Interface

- **Framework**: Cobra
- **Commands**:
  - `cloudpull init` - Initialize authentication
  - `cloudpull sync <folder-id> <destination>` - Start sync
  - `cloudpull resume` - Resume interrupted sync
  - `cloudpull status` - Show sync progress
  - `cloudpull config` - Manage configuration

### 2. Sync Engine

- **Responsibilities**:
  - Orchestrate download operations
  - Manage worker pools
  - Handle pause/resume logic
  - Coordinate state updates

### 3. State Manager

- **Database**: SQLite
- **Tables**:

  ```sql
  -- Sync sessions
  CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    root_folder_id TEXT NOT NULL,
    destination_path TEXT NOT NULL,
    start_time TIMESTAMP,
    end_time TIMESTAMP,
    status TEXT,
    total_files INTEGER,
    completed_files INTEGER,
    total_bytes INTEGER,
    completed_bytes INTEGER
  );

  -- Folder tracking
  CREATE TABLE folders (
    id TEXT PRIMARY KEY,
    drive_id TEXT UNIQUE NOT NULL,
    parent_id TEXT,
    name TEXT NOT NULL,
    path TEXT NOT NULL,
    status TEXT NOT NULL,
    last_modified TIMESTAMP,
    session_id TEXT,
    FOREIGN KEY (session_id) REFERENCES sessions(id)
  );

  -- File tracking
  CREATE TABLE files (
    id TEXT PRIMARY KEY,
    drive_id TEXT UNIQUE NOT NULL,
    folder_id TEXT NOT NULL,
    name TEXT NOT NULL,
    path TEXT NOT NULL,
    size INTEGER NOT NULL,
    md5_checksum TEXT,
    mime_type TEXT,
    status TEXT NOT NULL,
    bytes_downloaded INTEGER DEFAULT 0,
    last_modified TIMESTAMP,
    session_id TEXT,
    FOREIGN KEY (folder_id) REFERENCES folders(id),
    FOREIGN KEY (session_id) REFERENCES sessions(id)
  );

  -- Error log
  CREATE TABLE errors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id TEXT NOT NULL,
    item_type TEXT NOT NULL,
    error_type TEXT NOT NULL,
    error_message TEXT,
    retry_count INTEGER DEFAULT 0,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
  );
  ```

### 4. Google Drive Client

- **Features**:
  - OAuth2 authentication
  - Batch API requests
  - Automatic retry with exponential backoff
  - Export format handling for Google Docs

### 5. Rate Limiter

- **Algorithm**: Token bucket
- **Configuration**:
  - Requests per second: 10 (default)
  - Burst capacity: 20
  - Dynamic adjustment based on 429 responses

### 6. Download Manager

- **Concurrency**: 3-5 parallel downloads
- **Features**:
  - Chunked downloads for large files
  - Resume from byte offset
  - Checksum verification
  - Bandwidth throttling

## Data Flow

```text
1. User initiates sync
   ↓
2. Authenticate with Google Drive
   ↓
3. Initialize/resume session in database
   ↓
4. Start folder traversal (BFS/DFS)
   ↓
5. For each item:
   a. Check if already downloaded
   b. Add to download queue if needed
   c. Update progress
   ↓
6. Download workers process queue
   ↓
7. Verify and finalize downloads
   ↓
8. Update session status
```

## Performance Considerations

### Memory Management

- Stream folder listings (don't load all at once)
- Use pagination (1000 items per page)
- Implement LRU cache for metadata
- Buffer pool for file downloads

### Concurrency Model

```go
type WorkerPool struct {
    workers    int
    jobQueue   chan DownloadJob
    results    chan DownloadResult
    semaphore  chan struct{}
}
```

### Error Handling Strategy

1. **Transient Errors** (network, rate limit)
   - Exponential backoff with jitter
   - Max 5 retries

2. **Permanent Errors** (permissions, not found)
   - Log and skip
   - Mark in database

3. **Fatal Errors** (out of space, database corruption)
   - Graceful shutdown
   - Preserve state for recovery

## Security Measures

- Encrypted credential storage
- Secure token refresh
- File permission preservation
- No credential logging
- Optional local file encryption

## Monitoring & Metrics

- Download speed tracking
- API quota usage monitoring
- Error rate by type
- Memory usage profiling
- Progress reporting