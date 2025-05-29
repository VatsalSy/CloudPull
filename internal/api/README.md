# Google Drive API Integration

This package provides a production-ready Google Drive API integration for CloudPull with comprehensive error handling, rate limiting, and batch operations support.

## Components

### 1. Authentication (`auth.go`)
- OAuth2 authentication flow
- Automatic token refresh
- Secure token storage
- Browser-based authentication

### 2. Drive Client (`client.go`)
- High-level API wrapper
- Resumable downloads with byte ranges
- Google Workspace file export
- Automatic retry with exponential backoff
- Progress tracking

### 3. Rate Limiter (`ratelimiter.go`)
- Token bucket algorithm
- Adaptive rate limiting
- Per-operation limits
- Multi-tenant support
- Metrics collection

### 4. Batch Operations (`batch.go`)
- Efficient batch processing
- Parallel downloads
- Metadata fetching
- Progress tracking

## Usage

### Basic Setup

```go
// Initialize authentication
authManager, err := api.NewAuthManager(credentialsPath, tokenPath, logger)
if err != nil {
    return err
}

// Get Drive service
service, err := authManager.GetDriveService(ctx)
if err != nil {
    return err
}

// Create rate limiter
rateLimiter := api.NewRateLimiter(nil) // Uses default config

// Create Drive client
client := api.NewDriveClient(service, rateLimiter, logger)
```

### Downloading Files

```go
// Download with progress tracking
err := client.DownloadFile(ctx, fileID, destPath, func(downloaded, total int64) {
    percent := float64(downloaded) / float64(total) * 100
    fmt.Printf("Progress: %.2f%%\n", percent)
})
```

### Batch Operations

```go
// Batch download multiple files
downloader := api.NewBatchDownloader(client, logger, 3)
tasks := []api.DownloadTask{
    {FileID: "id1", DestPath: "/path/1"},
    {FileID: "id2", DestPath: "/path/2"},
}
results, err := downloader.DownloadFiles(ctx, tasks, progressFn)
```

### Export Google Workspace Files

```go
// Export Google Docs as DOCX
err := client.ExportFile(ctx, fileID, 
    "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    destPath, progressFn)
```

## Configuration

### Rate Limiter Configuration

```go
config := &api.RateLimiterConfig{
    RateLimit:       10,  // Requests per second
    BurstSize:       20,  // Burst capacity
    BatchRateLimit:  5,   // Batch operations per second
    ExportRateLimit: 3,   // Export operations per second
}
rateLimiter := api.NewRateLimiter(config)
```

### Adaptive Rate Limiting

The adaptive rate limiter automatically adjusts rates based on API responses:

```go
adaptiveRL := api.NewAdaptiveRateLimiter(config)
// Automatically reduces rate on 429 errors
// Gradually increases rate when successful
```

## Error Handling

The client automatically retries on:
- Rate limit errors (429)
- Server errors (500, 502, 503, 504)
- Network errors (connection refused, timeout)

```go
// Errors are wrapped with context
if err != nil {
    if apiErr, ok := err.(*googleapi.Error); ok {
        switch apiErr.Code {
        case 404:
            // File not found
        case 403:
            // Check for rate limit in Errors field
        case 401:
            // Authentication error
        }
    }
}
```

## Google Workspace Export Formats

| File Type | Export Format | Extension |
|-----------|---------------|-----------|
| Google Docs | DOCX | .docx |
| Google Sheets | XLSX | .xlsx |
| Google Slides | PPTX | .pptx |
| Google Drawings | PDF | .pdf |

## Performance Considerations

1. **Batch Operations**: Use batch operations for multiple files to reduce API calls
2. **Chunk Size**: Default 10MB chunks for downloads, adjustable via `client.chunkSize`
3. **Concurrent Downloads**: Default 3 concurrent downloads in batch operations
4. **Rate Limits**: Respect Google's limits (1000 queries per 100 seconds per user)

## Testing

Run tests with:
```bash
go test ./internal/api -v
```

For integration tests, set `GOOGLE_CREDENTIALS_PATH`:
```bash
export GOOGLE_CREDENTIALS_PATH=/path/to/credentials.json
go test ./internal/api -v -tags=integration
```