# CloudPull Error Handling System

A comprehensive error handling and retry system designed for cloud synchronization operations, providing
structured error types, configurable retry policies, and integrated logging.

## Features

- **Structured Error Types**: Categorized errors with metadata for proper handling
- **Configurable Retry Policies**: Per-error-type retry configuration with exponential backoff
- **Error Recovery Strategies**: Automatic recovery based on error types
- **Batch Error Handling**: Aggregate and handle multiple errors from batch operations
- **Context-Aware**: Proper handling of context cancellation and timeouts
- **Adaptive Backoff**: Adjusts retry intervals based on error patterns
- **Integrated Logging**: Structured logging with zerolog for debugging and monitoring

## Error Categories

| Error Type | Description | Retryable | Default Strategy |
|------------|-------------|-----------|------------------|
| `Network` | Network connectivity issues | Yes | Exponential backoff with jitter |
| `APIQuota` | Rate limiting or quota exceeded | Yes | Long backoff with jitter |
| `Permission` | Authentication/authorization failures | No | No retry |
| `Storage` | Disk space or I/O errors | Yes | Short backoff |
| `Corruption` | Data integrity issues | Yes | Restart from beginning |
| `Configuration` | Invalid configuration | No | No retry |
| `Context` | Context cancellation/timeout | No | No retry |

## Usage Examples

### Basic Error Handling

```go
import (
    "context"
    "github.com/cloudpull/cloudpull/internal/errors"
    "github.com/cloudpull/cloudpull/internal/logger"
)

// Initialize logger and error handler
log := logger.New(&logger.Config{
    Level: "info",
    Pretty: true,
})

handler := errors.NewHandler(log)

// Wrap and handle an error
err := performOperation()
if err != nil {
    // Wrap with context
    wrappedErr := errors.WrapWithContext(ctx, err, "operation_name", "/path/to/resource")

    // Determine recovery strategy
    strategy := handler.HandleError(ctx, wrappedErr)

    // Execute recovery
    if strategy != errors.RecoveryStrategyNone {
        err = handler.RecoverWithStrategy(ctx, wrappedErr, strategy, func() error {
            return performOperation()
        })
    }
}
```

### Custom Retry Policy

```go
// Configure custom retry policy for API quota errors
quotaPolicy := &errors.RetryPolicy{
    MaxAttempts:  20,
    InitialDelay: 30 * time.Second,
    MaxDelay:     10 * time.Minute,
    Multiplier:   1.5,
    Jitter:       true,
}

handler.SetRetryPolicy(errors.ErrorTypeAPIQuota, quotaPolicy)
```

### Batch Error Handling

```go
// Create error batch
batch := &errors.ErrorBatch{
    Op: "batch_upload",
}

// Process items and collect errors
for _, item := range items {
    if err := processItem(item); err != nil {
        syncErr := errors.New(errors.ErrorTypeNetwork, "upload", item.Path, err)
        batch.Add(syncErr)
    }
}

// Handle batch errors
if batch.HasErrors() {
    strategies := handler.HandleBatchErrors(ctx, batch)

    // Retry retryable errors
    for _, err := range batch.RetryableErrors() {
        strategy := strategies[err]
        if strategy != errors.RecoveryStrategyNone {
            handler.RecoverWithStrategy(ctx, err, strategy, retryOperation)
        }
    }
}
```

### Exponential Backoff with Jitter

```go
// Simple retry with backoff
err := errors.RetryWithBackoff(ctx, 5, func() error {
    return makeAPICall()
})

// Custom backoff configuration
config := &errors.BackoffConfig{
    InitialInterval:     1 * time.Second,
    MaxInterval:         60 * time.Second,
    Multiplier:          2.0,
    MaxElapsedTime:      15 * time.Minute,
    RandomizationFactor: 0.5,
}

err := errors.RetryOperation(ctx, operation, config, shouldRetry)
```

### Adaptive Backoff

```go
// Create adaptive backoff that adjusts based on success/failure patterns
adaptiveBackoff := errors.NewAdaptiveBackoff(
    &errors.BackoffConfig{
        InitialInterval: 5 * time.Second,
        MaxInterval:     2 * time.Minute,
        Multiplier:      1.5,
    },
    &errors.AdaptiveConfig{
        SuccessThreshold: 3,
        ErrorThreshold:   5,
        AdaptationFactor: 0.5,
    },
)

// Use in retry loop
for {
    err := operation()
    if err == nil {
        adaptiveBackoff.RecordSuccess()
        break
    }

    adaptiveBackoff.RecordError(errorType)
    backoff := adaptiveBackoff.NextBackOff()
    if backoff < 0 {
        return err // Max time exceeded
    }

    time.Sleep(backoff)
}
```

## Integration with Logger

The error handler integrates seamlessly with the logger package:

```go
// Errors are automatically logged with context
handler.HandleError(ctx, err) // Logs error details

// Structured error logging
log.StructuredError(err, map[string]interface{}{
    "file": filePath,
    "size": fileSize,
    "attempt": retryAttempt,
})

// Operation logging with error handling
err := log.LogOperation("sync_file", func() error {
    return syncFile(path)
})
```

## Recovery Strategies

| Strategy | Description | Use Case |
|----------|-------------|----------|
| `None` | No recovery possible | Permanent errors |
| `Retry` | Simple retry without delay | Quick transient errors |
| `Backoff` | Retry with exponential backoff | Network/API errors |
| `Restart` | Restart operation from beginning | Corruption errors |
| `Skip` | Skip failed item and continue | Batch operations |

## Best Practices

1. **Always wrap errors with context**: Use `WrapWithContext` to add operation and resource information
2. **Configure appropriate retry policies**: Set policies based on your API limits and requirements
3. **Handle batch errors gracefully**: Use `ErrorBatch` for operations on multiple items
4. **Log errors with structure**: Include relevant context for debugging
5. **Respect context cancellation**: Check context in long-running operations
6. **Use adaptive backoff for variable conditions**: Let the system adjust to changing error patterns

## Testing

The error handling system includes comprehensive tests:

```bash
go test ./internal/errors -v
go test ./internal/logger -v
```

## Performance Considerations

- Jitter prevents thundering herd problems
- Adaptive backoff reduces unnecessary retries
- Context awareness prevents resource waste
- Structured logging enables efficient debugging

## Future Enhancements

- Circuit breaker pattern for repeated failures
- Error metrics and monitoring integration
- Distributed tracing support
- Custom error categorization plugins
- Error recovery checkpoints for long operations
