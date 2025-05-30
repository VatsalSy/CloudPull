package api

import (
  "context"
  "fmt"
  "os"
  "testing"
  "time"

  "github.com/stretchr/testify/assert"
  "github.com/stretchr/testify/require"
  "golang.org/x/oauth2"
  "google.golang.org/api/drive/v3"
  "google.golang.org/api/googleapi"
  "github.com/VatsalSy/CloudPull/internal/logger"
)

/**
 * Unit tests for Google Drive API integration
 * 
 * Author: CloudPull Team
 * Updated: 2025-01-29
 */

// Mock logger for testing
type mockLogger struct {
	*logger.Logger
}

func newMockLogger() *logger.Logger {
	cfg := &logger.Config{
		Level:  "error",
		Format: "json",
	}
	l, _ := logger.New(cfg)
	return l
}

func TestRateLimiter(t *testing.T) {
  t.Run("basic rate limiting", func(t *testing.T) {
    config := &RateLimiterConfig{
      RateLimit: 5,  // 5 requests per second
      BurstSize: 10,
    }
    rl := NewRateLimiter(config)
    
    ctx := context.Background()
    start := time.Now()
    
    // Make 10 requests (should use burst)
    for i := 0; i < 10; i++ {
      err := rl.Wait(ctx)
      assert.NoError(t, err)
    }
    
    // Should complete quickly due to burst
    assert.Less(t, time.Since(start), 100*time.Millisecond)
    
    // Make 5 more requests (should be rate limited)
    start = time.Now()
    for i := 0; i < 5; i++ {
      err := rl.Wait(ctx)
      assert.NoError(t, err)
    }
    
    // Should take approximately 1 second (5 requests at 5/sec)
    duration := time.Since(start)
    assert.Greater(t, duration, 800*time.Millisecond)
    assert.Less(t, duration, 1200*time.Millisecond)
  })

  t.Run("context cancellation", func(t *testing.T) {
    config := &RateLimiterConfig{
      RateLimit: 1,
      BurstSize: 1,
    }
    rl := NewRateLimiter(config)
    
    ctx, cancel := context.WithCancel(context.Background())
    
    // Use up the burst
    err := rl.Wait(ctx)
    require.NoError(t, err)
    
    // Cancel context
    cancel()
    
    // Next wait should fail immediately
    err = rl.Wait(ctx)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "cancelled")
  })

  t.Run("metrics tracking", func(t *testing.T) {
    config := &RateLimiterConfig{
      RateLimit: 10,
      BurstSize: 5,
    }
    rl := NewRateLimiter(config)
    
    ctx := context.Background()
    
    // Make some requests
    for i := 0; i < 8; i++ {
      _ = rl.Wait(ctx)
    }
    
    metrics := rl.GetMetrics()
    assert.Equal(t, int64(8), metrics.TotalRequests)
    assert.Greater(t, metrics.BlockedRequests, int64(0))
    assert.Greater(t, metrics.RequestsPerSecond, float64(0))
  })
}

func TestAdaptiveRateLimiter(t *testing.T) {
  t.Run("rate limit adjustment", func(t *testing.T) {
    config := &RateLimiterConfig{
      RateLimit: 10,
      BurstSize: 20,
    }
    arl := NewAdaptiveRateLimiter(config)
    
    // Record some rate limit errors
    arl.RecordRateLimitError()
    arl.RecordRateLimitError()
    
    // Rate limit should be reduced
    assert.Less(t, arl.GetCurrentRateLimit(), 10)
    
    // Record successes
    for i := 0; i < 5; i++ {
      arl.RecordSuccess()
      time.Sleep(10 * time.Millisecond)
    }
    
    // Wait for adjustment period
    time.Sleep(31 * time.Second)
    arl.RecordSuccess()
    
    // Rate limit should start increasing
    currentLimit := arl.GetCurrentRateLimit()
    assert.Greater(t, currentLimit, 1)
  })
}

func TestAuthManager(t *testing.T) {
  // Skip if no credentials file
  credPath := os.Getenv("GOOGLE_CREDENTIALS_PATH")
  if credPath == "" {
    t.Skip("GOOGLE_CREDENTIALS_PATH not set")
  }

  t.Run("token management", func(t *testing.T) {
    logger := newMockLogger()
    tokenPath := "/tmp/cloudpull_test_token.json"
    defer os.Remove(tokenPath)
    
    am, err := NewAuthManager(credPath, tokenPath, logger)
    require.NoError(t, err)
    
    // Check if authenticated (should be false initially)
    assert.False(t, am.IsAuthenticated())
    
    // Test token save/load
    testToken := &oauth2.Token{
      AccessToken:  "test_access_token",
      RefreshToken: "test_refresh_token",
      Expiry:       time.Now().Add(time.Hour),
    }
    
    err = am.saveToken(testToken)
    assert.NoError(t, err)
    
    loadedToken, err := am.loadToken()
    assert.NoError(t, err)
    assert.Equal(t, testToken.AccessToken, loadedToken.AccessToken)
    assert.Equal(t, testToken.RefreshToken, loadedToken.RefreshToken)
  })
}

func TestBatchProcessor(t *testing.T) {
  t.Run("batch queue management", func(t *testing.T) {
    logger := newMockLogger()
    rl := NewRateLimiter(nil)
    
    bp := NewBatchProcessor(nil, rl, logger)
    
    // Add requests
    for i := 0; i < 150; i++ {
      req := BatchRequest{
        ID:     fmt.Sprintf("req_%d", i),
        Type:   BatchGetMetadata,
        FileID: fmt.Sprintf("file_%d", i),
      }
      err := bp.AddRequest(req)
      assert.NoError(t, err)
    }
    
    // Check queue size
    bp.mu.Lock()
    queueSize := len(bp.queue)
    bp.mu.Unlock()
    assert.Equal(t, 150, queueSize)
    
    // Dequeue batch
    batch := bp.dequeueBatch()
    assert.Len(t, batch, maxBatchSize)
    
    // Check remaining queue
    bp.mu.Lock()
    remainingSize := len(bp.queue)
    bp.mu.Unlock()
    assert.Equal(t, 50, remainingSize)
  })
}

func TestFileInfoConversion(t *testing.T) {
  t.Run("google workspace file detection", func(t *testing.T) {
    testCases := []struct {
      mimeType     string
      canExport    bool
      exportFormat string
    }{
      {
        mimeType:     "application/vnd.google-apps.document",
        canExport:    true,
        exportFormat: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
      },
      {
        mimeType:     "application/vnd.google-apps.spreadsheet",
        canExport:    true,
        exportFormat: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
      },
      {
        mimeType:  "application/pdf",
        canExport: false,
      },
      {
        mimeType:  "image/jpeg",
        canExport: false,
      },
    }
    
    for _, tc := range testCases {
      t.Run(tc.mimeType, func(t *testing.T) {
        file := &drive.File{
          Id:       "test_id",
          Name:     "test_file",
          MimeType: tc.mimeType,
        }
        
        client := &DriveClient{}
        info := client.convertFileInfo(file)
        
        assert.Equal(t, tc.canExport, info.CanExport)
        if tc.canExport {
          assert.Equal(t, tc.exportFormat, info.ExportFormat)
        }
      })
    }
  })
}

func TestRetryLogic(t *testing.T) {
  t.Run("retryable error detection", func(t *testing.T) {
    client := &DriveClient{}
    
    testCases := []struct {
      err        error
      retryable  bool
      description string
    }{
      {
        err: &googleapi.Error{Code: 429},
        retryable: true,
        description: "rate limit error",
      },
      {
        err: &googleapi.Error{Code: 500},
        retryable: true,
        description: "server error",
      },
      {
        err: &googleapi.Error{Code: 404},
        retryable: false,
        description: "not found error",
      },
      {
        err: &googleapi.Error{
          Code: 403,
          Errors: []googleapi.ErrorItem{
            {Reason: "userRateLimitExceeded"},
          },
        },
        retryable: true,
        description: "user rate limit in 403",
      },
      {
        err: fmt.Errorf("connection refused"),
        retryable: true,
        description: "network error",
      },
    }
    
    for _, tc := range testCases {
      t.Run(tc.description, func(t *testing.T) {
        result := client.isRetryableError(tc.err)
        assert.Equal(t, tc.retryable, result)
      })
    }
  })
}