-- CloudPull Database Schema
-- SQLite3 Database for State Management

-- Enable foreign keys
PRAGMA foreign_keys = ON;

-- Sync sessions table
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    root_folder_id TEXT NOT NULL,
    root_folder_name TEXT,
    destination_path TEXT NOT NULL,
    start_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    end_time TIMESTAMP,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'completed', 'failed', 'cancelled')),
    total_files INTEGER DEFAULT 0,
    completed_files INTEGER DEFAULT 0,
    failed_files INTEGER DEFAULT 0,
    skipped_files INTEGER DEFAULT 0,
    total_bytes INTEGER DEFAULT 0,
    completed_bytes INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Folders table
CREATE TABLE IF NOT EXISTS folders (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    drive_id TEXT NOT NULL,
    parent_id TEXT,
    session_id TEXT NOT NULL,
    name TEXT NOT NULL,
    path TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'scanning', 'scanned', 'failed')),
    error_message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(drive_id, session_id),
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
    FOREIGN KEY (parent_id) REFERENCES folders(id) ON DELETE CASCADE
);

-- Files table
CREATE TABLE IF NOT EXISTS files (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    drive_id TEXT NOT NULL,
    folder_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    name TEXT NOT NULL,
    path TEXT NOT NULL,
    size INTEGER NOT NULL,
    md5_checksum TEXT,
    mime_type TEXT,
    is_google_doc BOOLEAN DEFAULT FALSE,
    export_mime_type TEXT,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'downloading', 'completed', 'failed', 'skipped')),
    bytes_downloaded INTEGER DEFAULT 0,
    download_attempts INTEGER DEFAULT 0,
    error_message TEXT,
    drive_modified_time TIMESTAMP,
    local_modified_time TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(drive_id, session_id),
    FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE CASCADE,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

-- Download chunks table (for resumable downloads)
CREATE TABLE IF NOT EXISTS download_chunks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id TEXT NOT NULL,
    chunk_index INTEGER NOT NULL,
    start_byte INTEGER NOT NULL,
    end_byte INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'downloading', 'completed', 'failed')),
    attempts INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    UNIQUE(file_id, chunk_index),
    FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
);

-- Error log table
CREATE TABLE IF NOT EXISTS error_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    item_id TEXT NOT NULL,
    item_type TEXT NOT NULL CHECK (item_type IN ('file', 'folder')),
    error_type TEXT NOT NULL,
    error_code TEXT,
    error_message TEXT,
    stack_trace TEXT,
    retry_count INTEGER DEFAULT 0,
    is_retryable BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

-- Configuration table
CREATE TABLE IF NOT EXISTS config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_folders_drive_id ON folders(drive_id);
CREATE INDEX IF NOT EXISTS idx_folders_status ON folders(status);
CREATE INDEX IF NOT EXISTS idx_folders_session_id ON folders(session_id);

CREATE INDEX IF NOT EXISTS idx_files_drive_id ON files(drive_id);
CREATE INDEX IF NOT EXISTS idx_files_status ON files(status);
CREATE INDEX IF NOT EXISTS idx_files_session_id ON files(session_id);
CREATE INDEX IF NOT EXISTS idx_files_folder_id ON files(folder_id);

CREATE INDEX IF NOT EXISTS idx_chunks_file_id ON download_chunks(file_id);
CREATE INDEX IF NOT EXISTS idx_chunks_status ON download_chunks(status);

CREATE INDEX IF NOT EXISTS idx_errors_session_id ON error_log(session_id);
CREATE INDEX IF NOT EXISTS idx_errors_item_id ON error_log(item_id);

-- Triggers for updated_at
CREATE TRIGGER IF NOT EXISTS update_sessions_timestamp
    AFTER UPDATE ON sessions
    FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
    BEGIN
        UPDATE sessions SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
    END;

CREATE TRIGGER IF NOT EXISTS update_folders_timestamp
    AFTER UPDATE ON folders
    FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
    BEGIN
        UPDATE folders SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
    END;

CREATE TRIGGER IF NOT EXISTS update_files_timestamp
    AFTER UPDATE ON files
    FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
    BEGIN
        UPDATE files SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
    END;

CREATE TRIGGER IF NOT EXISTS update_config_timestamp
    AFTER UPDATE ON config
    FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
    BEGIN
        UPDATE config SET updated_at = CURRENT_TIMESTAMP WHERE key = NEW.key;
    END;

-- Views for easier querying
CREATE VIEW IF NOT EXISTS session_summary AS
SELECT
    s.id,
    s.root_folder_name,
    s.destination_path,
    s.status,
    s.total_files,
    s.completed_files,
    s.failed_files,
    s.skipped_files,
    COALESCE(ROUND(CAST(s.completed_files AS FLOAT) / NULLIF(s.total_files, 0) * 100, 2), 0) as progress_percent,
    s.total_bytes,
    s.completed_bytes,
    COALESCE(ROUND(CAST(s.completed_bytes AS FLOAT) / NULLIF(s.total_bytes, 0) * 100, 2), 0) as bytes_progress_percent,
    s.start_time,
    s.end_time,
    CASE
        WHEN s.end_time IS NOT NULL THEN (julianday(s.end_time) - julianday(s.start_time)) * 86400
        ELSE (julianday('now') - julianday(s.start_time)) * 86400
    END as duration_seconds
FROM sessions s;

CREATE VIEW IF NOT EXISTS pending_downloads AS
SELECT
    f.id,
    f.drive_id,
    f.name,
    f.path,
    f.size,
    f.mime_type,
    f.is_google_doc,
    f.export_mime_type,
    f.bytes_downloaded,
    f.download_attempts,
    fo.path as folder_path
FROM files f
JOIN folders fo ON f.folder_id = fo.id
WHERE f.status IN ('pending', 'downloading')
ORDER BY f.size ASC; -- Download smaller files first
