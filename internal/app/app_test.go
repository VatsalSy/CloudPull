package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppInitialization(t *testing.T) {
	// Setup test config
	setupTestConfig(t)

	// Create app
	app, err := New()
	require.NoError(t, err)
	assert.NotNil(t, app)

	// Initialize
	err = app.Initialize()
	require.NoError(t, err)

	// Verify components
	assert.True(t, app.isInitialized)
	assert.NotNil(t, app.logger)
	assert.NotNil(t, app.stateManager)
	assert.NotNil(t, app.errorHandler)
	assert.NotNil(t, app.config)
}

func TestAppAuthInitialization(t *testing.T) {
	// Skip if no credentials file
	credFile := os.Getenv("CLOUDPULL_TEST_CREDENTIALS")
	if credFile == "" {
		t.Skip("CLOUDPULL_TEST_CREDENTIALS not set")
	}

	setupTestConfig(t)
	viper.Set("credentials_file", credFile)

	app, err := New()
	require.NoError(t, err)

	err = app.Initialize()
	require.NoError(t, err)

	err = app.InitializeAuth()
	require.NoError(t, err)

	assert.NotNil(t, app.authManager)
	assert.NotNil(t, app.apiClient)
}

func TestAppSyncEngineInitialization(t *testing.T) {
	// Skip if no credentials
	credFile := os.Getenv("CLOUDPULL_TEST_CREDENTIALS")
	if credFile == "" {
		t.Skip("CLOUDPULL_TEST_CREDENTIALS not set")
	}

	setupTestConfig(t)
	viper.Set("credentials_file", credFile)

	app, err := New()
	require.NoError(t, err)

	err = app.Initialize()
	require.NoError(t, err)

	err = app.InitializeAuth()
	require.NoError(t, err)

	err = app.InitializeSyncEngine()
	require.NoError(t, err)

	assert.NotNil(t, app.syncEngine)
}

func TestAppShutdown(t *testing.T) {
	setupTestConfig(t)

	app, err := New()
	require.NoError(t, err)

	err = app.Initialize()
	require.NoError(t, err)

	// Test shutdown
	err = app.Stop()
	assert.NoError(t, err)

	// Shutdown should be idempotent
	err = app.Stop()
	assert.NoError(t, err)
}

func TestAppSignalHandling(t *testing.T) {
	setupTestConfig(t)

	app, err := New()
	require.NoError(t, err)

	err = app.Initialize()
	require.NoError(t, err)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Simulate signal handling
	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// App should handle gracefully
	err = app.Stop()
	assert.NoError(t, err)
}

func TestSyncOptions(t *testing.T) {
	options := &SyncOptions{
		IncludePatterns: []string{"*.pdf", "*.doc"},
		ExcludePatterns: []string{"temp/*"},
		MaxDepth:        5,
		BandwidthLimit:  1024 * 1024 * 10, // 10 MB/s
		DryRun:          true,
	}

	assert.Equal(t, 2, len(options.IncludePatterns))
	assert.Equal(t, 1, len(options.ExcludePatterns))
	assert.Equal(t, 5, options.MaxDepth)
	assert.Equal(t, int64(10485760), options.BandwidthLimit)
	assert.True(t, options.DryRun)
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		expected string
		bytes    int64
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatBytes(tt.bytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper functions

func setupTestConfig(t *testing.T) {
	t.Helper()

	// Create temp directory
	tempDir := t.TempDir()

	// Reset viper
	viper.Reset()

	// Set test configuration
	viper.Set("version", "test")
	viper.Set("log.level", "debug")
	viper.Set("log.format", "text")
	viper.Set("log.output", "stdout")

	// Use temp directory for data
	dataDir := filepath.Join(tempDir, ".cloudpull")
	viper.Set("data_dir", dataDir)

	// Sync settings
	viper.Set("sync.max_concurrent", 2)
	viper.Set("sync.chunk_size_bytes", 1024*1024)
	viper.Set("sync.progress_interval", 1)
	viper.Set("sync.checkpoint_interval", 5)
	viper.Set("sync.max_errors", 10)

	// API settings
	viper.Set("api.max_retries", 3)
	viper.Set("api.retry_delay", 1)
	viper.Set("api.request_timeout", 30)
	viper.Set("api.max_concurrent", 5)
	viper.Set("api.rate_limit", 10)

	// Error settings
	viper.Set("errors.max_retries", 3)
	viper.Set("errors.retry_delay", 1)
	viper.Set("errors.retry_multiplier", 2.0)
	viper.Set("errors.retry_max_delay", 30)
}
