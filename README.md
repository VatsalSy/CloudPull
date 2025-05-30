# CloudPull

Fast, resumable Google Drive folder synchronization tool with rsync-like functionality.

[![Go Version](https://img.shields.io/badge/go-1.21%2B-blue)](https://golang.org/dl/)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

## Features

- 🚀 **High Performance**: Concurrent downloads with intelligent scheduling
- 💾 **Resume Support**: Interrupt and resume downloads at any time
- 📊 **Progress Tracking**: Real-time progress with ETA and transfer speed
- 🔄 **Smart Sync**: Only download new or modified files
- 📁 **Large Folder Support**: Efficiently handle folders with millions of files
- 🔐 **Secure**: OAuth2 authentication with Google Drive
- 🎯 **Selective Sync**: Include/exclude patterns for fine control
- 📈 **Bandwidth Control**: Configurable rate limiting
- 🔍 **Checksum Verification**: Ensure data integrity

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/VatsalSy/CloudPull.git
cd CloudPull

# Quick setup (checks Go version, downloads deps, builds)
./scripts/setup.sh

# Or build manually (automatically downloads dependencies)
make build

# Or if you prefer manual steps:
# 1. Download dependencies
go mod download

# 2. Build the binary
go build -o build/cloudpull ./cmd/cloudpull

# Install to $GOPATH/bin (optional)
make install
```

### Pre-built Binaries

Download the latest release for your platform from the [releases page](https://github.com/VatsalSy/CloudPull/releases).

## OAuth Setup

### Setting up Google Drive API Access

1. **Create a Google Cloud Project**:
   - Go to [Google Cloud Console](https://console.cloud.google.com/)
   - Create a new project or select an existing one
   - Enable the Google Drive API for your project

2. **Create OAuth 2.0 Credentials**:
   - Go to "APIs & Services" > "Credentials"
   - Click "Create Credentials" > "OAuth client ID"
   - Choose "Desktop app" as the application type
   - Download the credentials JSON file

3. **Configure CloudPull**:
   - Copy `client_secret.example.json` as a reference
   - Place your downloaded credentials file in the CloudPull config directory
   - The file should match the pattern `client_secret*.json`
   - Default location: `~/.cloudpull/client_secret_*.json`

**Note**: Never commit your actual `client_secret*.json` file to version control. It's already included in `.gitignore`.

## Quick Start

### 1. Initialize Authentication

```bash
cloudpull init
```

This will open your browser for Google Drive authentication.

### 2. Sync a Folder

```bash
# Sync a Google Drive folder to local directory
cloudpull sync GOOGLE_DRIVE_FOLDER_ID /path/to/local/folder

# With options
cloudpull sync GOOGLE_DRIVE_FOLDER_ID /path/to/local/folder \
  --exclude "*.tmp" \
  --exclude ".DS_Store" \
  --bandwidth-limit 10 \
  --concurrent-downloads 5
```

### 3. Resume Interrupted Sync

```bash
# Resume the last sync
cloudpull resume

# Resume a specific session
cloudpull resume --session SESSION_ID
```

### 4. Check Status

```bash
# Show current sync status
cloudpull status

# Show sync history
cloudpull status --history
```

## Configuration

CloudPull can be configured via YAML file, environment variables, or command-line flags.

### Configuration File

Create `.cloudpull.yaml` in your home directory or working directory:

```yaml
google:
  credentials_path: ~/.cloudpull/credentials.json
  export_formats:
    google-apps/document: application/pdf
    google-apps/spreadsheet: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet

download:
  concurrent_downloads: 3
  chunk_size: 10485760  # 10MB
  verify_checksums: true
  bandwidth_limit: 0    # 0 = unlimited (MB/s)

sync:
  exclude_patterns:
    - "*.tmp"
    - "~$*"
    - ".DS_Store"
  
logging:
  level: info
  file: ~/.cloudpull/sync.log
```

### Environment Variables

All configuration options can be set via environment variables:

```bash
export CLOUDPULL_DOWNLOAD_CONCURRENT_DOWNLOADS=5
export CLOUDPULL_DOWNLOAD_BANDWIDTH_LIMIT=20
export CLOUDPULL_LOGGING_LEVEL=debug
```

## Advanced Usage

### Dry Run

Preview what would be synced without downloading:

```bash
cloudpull sync FOLDER_ID /local/path --dry-run
```

### Export Google Docs

Configure how Google Workspace files are exported:

```bash
# Export as Microsoft Office formats (default)
cloudpull config set google.export_formats.google-apps/document application/vnd.openxmlformats-officedocument.wordprocessingml.document

# Export as PDF
cloudpull config set google.export_formats.google-apps/document application/pdf
```

### Bandwidth Limiting

Limit bandwidth usage (in MB/s):

```bash
cloudpull sync FOLDER_ID /local/path --bandwidth-limit 5
```

### Include/Exclude Patterns

Use glob patterns to filter files:

```bash
cloudpull sync FOLDER_ID /local/path \
  --include "*.pdf" \
  --include "*.docx" \
  --exclude "temp/*"
```

## Architecture

CloudPull is built with a modular architecture:

- **CLI**: User interface built with Cobra
- **Sync Engine**: Orchestrates the synchronization process
- **API Client**: Handles Google Drive API interactions
- **State Manager**: SQLite-based state persistence
- **Progress Tracker**: Real-time progress monitoring
- **Error Handler**: Intelligent retry and recovery

## Development

### Prerequisites

- Go 1.21 or higher
- Make (optional, for using Makefile)

### Building

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Run tests
make test

# Run with coverage
make coverage
```

### Project Structure

```
cloudpull/
├── cmd/cloudpull/     # CLI commands
├── internal/          # Private packages
│   ├── api/          # Google Drive API client
│   ├── config/       # Configuration management
│   ├── errors/       # Error handling
│   ├── logger/       # Logging
│   ├── state/        # State management
│   └── sync/         # Sync engine
├── pkg/              # Public packages
│   └── progress/     # Progress tracking
└── tests/            # Test suites
```

## Troubleshooting

### Build Issues

If you encounter build errors related to missing dependencies:

```bash
# Clean and rebuild
make clean
go mod download
make build

# Or use the deps target
make deps
make build
```

### Common Issues

1. **"command not found: go"** - Install Go 1.21+ from https://golang.org/dl/
2. **Permission errors** - Ensure you have write access to the destination directory
3. **Authentication fails** - Delete `~/.cloudpull/token.json` and re-authenticate
4. **Rate limit errors** - CloudPull automatically handles these, but you can reduce concurrent downloads

## FAQ

**Q: How do I find my Google Drive folder ID?**
A: Open the folder in Google Drive web interface. The ID is in the URL: `https://drive.google.com/drive/folders/FOLDER_ID_HERE`

**Q: Can I sync multiple folders?**
A: Yes, run multiple sync commands with different folder IDs and destinations.

**Q: Does CloudPull support two-way sync?**
A: Currently, CloudPull only supports one-way sync (Drive → Local). Two-way sync is planned for future releases.

**Q: How does resume work?**
A: CloudPull stores sync state in a local SQLite database. It tracks each file's download progress and can resume from the exact byte offset.

## Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) for details.

## License

CloudPull is released under the MIT License. See [LICENSE](LICENSE) for details.

## Support

- 🐛 [Issue Tracker](https://github.com/VatsalSy/CloudPull/issues)