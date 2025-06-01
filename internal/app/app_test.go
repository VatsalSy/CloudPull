package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/VatsalSy/CloudPull/internal/config"
)

func TestAppInitialization(t *testing.T) {
	// Setup test config
	v := setupTestConfig(t)

	// Create config loader that uses our local viper instance
	configLoader := func() (*config.Config, error) {
		return config.LoadFromViper(v)
	}

	// Create app with custom config loader
	app, err := New(WithConfigLoader(configLoader))
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

	v := setupTestConfig(t)
	v.Set("credentials_file", credFile)

	// Create config loader that uses our local viper instance
	configLoader := func() (*config.Config, error) {
		return config.LoadFromViper(v)
	}

	app, err := New(WithConfigLoader(configLoader))
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

	v := setupTestConfig(t)
	v.Set("credentials_file", credFile)

	// Create config loader that uses our local viper instance
	configLoader := func() (*config.Config, error) {
		return config.LoadFromViper(v)
	}

	app, err := New(WithConfigLoader(configLoader))
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
	v := setupTestConfig(t)

	// Create config loader that uses our local viper instance
	configLoader := func() (*config.Config, error) {
		return config.LoadFromViper(v)
	}

	app, err := New(WithConfigLoader(configLoader))
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
	if testing.Short() {
		t.Skip("Skipping signal handling test in short mode")
	}

	v := setupTestConfig(t)

	// Create config loader that uses our local viper instance
	configLoader := func() (*config.Config, error) {
		return config.LoadFromViper(v)
	}

	app, err := New(WithConfigLoader(configLoader))
	require.NoError(t, err)

	err = app.Initialize()
	require.NoError(t, err)

	// Create a context that the app will use
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Skip signal tests on Windows
	if runtime.GOOS == "windows" {
		t.Skip("Skipping signal test on Windows")
	}

	// Use WaitGroup to ensure signal handler is set up
	var wg sync.WaitGroup
	wg.Add(1)

	// Start the app's signal handling in a goroutine
	go func() {
		// Signal that the goroutine has started
		wg.Done()
		// This will block until a signal is received
		app.handleSignals(cancel)
	}()

	// Wait for signal handler goroutine to start
	wg.Wait()

	// Send SIGINT to the current process
	err = syscall.Kill(os.Getpid(), syscall.SIGINT)
	require.NoError(t, err)

	// Wait for context to be canceled by the signal handler
	select {
	case <-ctx.Done():
		// Signal was handled correctly
	case <-time.After(2 * time.Second):
		t.Fatal("Signal handler did not cancel context within timeout")
	}

	// App should handle graceful shutdown
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

// Helper functions

func setupTestConfig(t *testing.T) *viper.Viper {
	t.Helper()

	// Create temp directory
	tempDir := t.TempDir()

	// Create a new local viper instance
	v := viper.New()

	// Set test configuration on the local instance
	v.Set("version", "test")
	v.Set("log.level", "debug")
	v.Set("log.format", "text")
	v.Set("log.output", "stdout")

	// Use temp directory for data
	dataDir := filepath.Join(tempDir, ".cloudpull")
	v.Set("data_dir", dataDir)

	// Sync settings
	v.Set("sync.max_concurrent", 2)
	v.Set("sync.chunk_size_bytes", 1024*1024)
	v.Set("sync.progress_interval", 1)
	v.Set("sync.checkpoint_interval", 5)
	v.Set("sync.max_errors", 10)

	// API settings
	v.Set("api.max_retries", 3)
	v.Set("api.retry_delay", 1)
	v.Set("api.request_timeout", 30)
	v.Set("api.max_concurrent", 5)
	v.Set("api.rate_limit", 10)

	// Error settings
	v.Set("errors.max_retries", 3)
	v.Set("errors.retry_delay", 1)
	v.Set("errors.retry_multiplier", 2.0)
	v.Set("errors.retry_max_delay", 30)

	return v
}
