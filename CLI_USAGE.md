# CloudPull CLI Usage Guide

## Installation

```bash
# Clone the repository
git clone https://github.com/VatsalSy/CloudPull.git
cd CloudPull

# Build and install
make install

# Or build without installing
make build
./bin/cloudpull --help
```

## Getting Started

### 1. Initialize CloudPull

First, set up CloudPull with your Google Drive credentials:

```bash
# Interactive setup
cloudpull init

# With credentials file
cloudpull init --credentials-file ~/Downloads/credentials.json

# Skip browser authentication
cloudpull init --skip-browser
```

### 2. Start Syncing

```bash
# Interactive folder selection
cloudpull sync

# Sync specific folder by ID
cloudpull sync 1ABC123DEF456GHI

# Sync using share URL
cloudpull sync "https://drive.google.com/drive/folders/1ABC123DEF456GHI"

# Sync with custom output directory
cloudpull sync --output ~/Documents/DriveBackup

# Filter files
cloudpull sync --include "*.pdf" --include "*.docx" --exclude "temp/*"

# Dry run to see what would be synced
cloudpull sync --dry-run

# Limit folder depth
cloudpull sync --max-depth 2
```

## Command Reference

### Global Options

```bash
cloudpull --help              # Show help
cloudpull --version           # Show version
cloudpull --config FILE       # Use specific config file
cloudpull --verbose          # Enable verbose output
```

### Init Command

Initialize CloudPull with Google Drive authentication.

```bash
cloudpull init [options]

Options:
  -c, --credentials-file    Path to OAuth2 credentials JSON
      --skip-browser       Don't open browser for authentication
  -h, --help              Help for init
```

### Sync Command

Start a new sync from Google Drive.

```bash
cloudpull sync [folder-id|url] [options]

Options:
  -o, --output DIR         Output directory
  -i, --include PATTERN    Include files matching pattern (repeatable)
  -e, --exclude PATTERN    Exclude files matching pattern (repeatable)
      --dry-run           Show what would be synced
      --no-progress       Disable progress bars
      --max-depth N       Maximum folder depth (-1 for unlimited)
  -h, --help             Help for sync
```

### Resume Command

Resume an interrupted sync session.

```bash
cloudpull resume [session-id] [options]

Options:
      --latest    Resume most recent session
      --force     Force resume corrupted session
  -h, --help     Help for resume
```

### Status Command

Show sync progress and statistics.

```bash
cloudpull status [session-id] [options]

Options:
  -w, --watch      Continuously monitor status
  -d, --detailed   Show detailed statistics
      --history    Show completed sessions
  -h, --help      Help for status
```

### Config Command

Manage CloudPull configuration.

```bash
# View all configuration
cloudpull config

# Get specific value
cloudpull config get sync.max_concurrent

# Set configuration value
cloudpull config set sync.max_concurrent 5
cloudpull config set sync.bandwidth_limit 10

# Reset to defaults
cloudpull config reset

# Edit config file
cloudpull config edit
```

## Configuration

CloudPull stores configuration in `~/.cloudpull/config.yaml`.

### Environment Variables

All configuration can be overridden with environment variables:

```bash
export CLOUDPULL_SYNC_MAX_CONCURRENT=5
export CLOUDPULL_SYNC_BANDWIDTH_LIMIT=20
export CLOUDPULL_LOG_LEVEL=debug
```

### Configuration Options

| Key | Description | Default |
|-----|-------------|---------|
| `credentials_file` | OAuth2 credentials file path | - |
| `sync.default_directory` | Default download directory | `~/CloudPull` |
| `sync.max_concurrent` | Maximum concurrent downloads | `3` |
| `sync.chunk_size` | Download chunk size | `1MB` |
| `sync.bandwidth_limit` | Bandwidth limit (MB/s) | `0` (unlimited) |
| `files.skip_duplicates` | Skip existing files | `true` |
| `files.preserve_timestamps` | Keep original timestamps | `true` |
| `cache.enabled` | Enable metadata caching | `true` |
| `log.level` | Log level (debug/info/warn/error) | `info` |

## Examples

### Basic Sync Workflow

```bash
# 1. Initialize CloudPull
cloudpull init

# 2. Start syncing a folder
cloudpull sync "https://drive.google.com/drive/folders/ABC123"

# 3. Check progress
cloudpull status

# 4. If interrupted, resume later
cloudpull resume --latest
```

### Advanced Usage

```bash
# Sync only PDFs and Word documents
cloudpull sync FOLDER_ID \
  --include "*.pdf" \
  --include "*.doc*" \
  --output ~/Documents/Important

# Bandwidth-limited sync
cloudpull config set sync.bandwidth_limit 5
cloudpull sync FOLDER_ID

# Monitor multiple sync sessions
cloudpull status --watch

# Debug mode
export CLOUDPULL_LOG_LEVEL=debug
cloudpull sync --verbose
```

### Automation

```bash
# Cron job for regular syncs
0 2 * * * /usr/local/bin/cloudpull sync FOLDER_ID --output /backup/drive

# Script with error handling
#!/bin/bash
if ! cloudpull sync FOLDER_ID; then
    cloudpull resume --latest --force
fi
```

## Shell Completion

Enable tab completion for your shell:

```bash
# Bash
cloudpull completion bash > /etc/bash_completion.d/cloudpull

# Zsh
cloudpull completion zsh > "${fpath[1]}/_cloudpull"

# Fish
cloudpull completion fish > ~/.config/fish/completions/cloudpull.fish

# PowerShell
cloudpull completion powershell > cloudpull.ps1
```

## Troubleshooting

### Common Issues

1. **Authentication Failed**

   ```bash
   cloudpull init --credentials-file /path/to/new/credentials.json
   ```

2. **Resume Not Working**

   ```bash
   # List all sessions
   cloudpull resume
   
   # Force resume
   cloudpull resume SESSION_ID --force
   ```

3. **Slow Downloads**

   ```bash
   # Increase concurrent downloads
   cloudpull config set sync.max_concurrent 5
   
   # Increase chunk size
   cloudpull config set sync.chunk_size 4MB
   ```

### Debug Mode

```bash
# Enable debug logging
export CLOUDPULL_LOG_LEVEL=debug
cloudpull sync --verbose

# Check logs
tail -f ~/.cloudpull/logs/cloudpull.log
```

## Best Practices

1. **Use Dry Run First**: Always test with `--dry-run` before large syncs
2. **Set Bandwidth Limits**: Prevent network saturation with `sync.bandwidth_limit`
3. **Regular Backups**: Schedule regular syncs with cron
4. **Monitor Progress**: Use `cloudpull status --watch` for long syncs
5. **Organize Downloads**: Use meaningful output directories

## Support

- Email: [vatsalsanjay@gmail.com](mailto:vatsalsanjay@gmail.com)
