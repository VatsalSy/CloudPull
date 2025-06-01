package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaultConfig(t *testing.T) {
	viper.Reset()
	// Load an empty config file to ensure setViperDefaults are applied, then setDefaults.
	// This simulates the application's typical load path when no user config file exists.
	cfg, err := Load(filepath.Join(t.TempDir(), "non_existent_config.yaml"))
	require.NoError(t, err, "Load() with non-existent path should not produce an error")
	require.NotNil(t, cfg, "Load() should return a non-nil Config object")

	// Assert values based on setViperDefaults() followed by setDefaults()
	// These values are taken from the setViperDefaults in config.go
	homeDir, _ := os.UserHomeDir()
	expectedDefaultDir := filepath.Join(homeDir, "CloudPull")
	assert.Equal(t, expectedDefaultDir, cfg.Sync.DefaultDirectory, "Default Sync.DefaultDirectory is incorrect")
	assert.Equal(t, "info", cfg.Log.Level, "Default Log.Level is incorrect")
	assert.Equal(t, 3, cfg.API.MaxRetries, "Default API.MaxRetries is incorrect") // From setViperDefaults
	assert.Equal(t, "1MB", cfg.Sync.ChunkSize, "Default Sync.ChunkSize is incorrect") // From setViperDefaults
}

func TestLoadFromFile(t *testing.T) {
	// Use a local viper instance for this test to have more control
	// and avoid issues with the global viper and sync.Once in Load().
	v := viper.New()

	tempDir := t.TempDir()
	tempConfigFile := filepath.Join(tempDir, "test_config.yaml")

	configContent := `
log:
  level: "debug"
sync:
  max_concurrent: 5
  default_directory: "/tmp/my_custom_downloads"
api:
  request_timeout: 60
`
	err := os.WriteFile(tempConfigFile, []byte(configContent), 0600)
	require.NoError(t, err, "Failed to write temporary config file")

	// Configure the local viper instance
	v.SetConfigFile(tempConfigFile)
	err = v.ReadInConfig()
	require.NoError(t, err, "Local viper failed to read config file")

	// Apply global defaults to the local viper instance.
	// This is a bit of a workaround as setViperDefaults() acts on global viper.
	// We are effectively reapplying them here to simulate the desired state for LoadFromViper.
	homeDirForDefaults, _ := os.UserHomeDir()
	v.SetDefault("sync.default_directory", filepath.Join(homeDirForDefaults, "CloudPull"))
	v.SetDefault("sync.max_concurrent", 3)
	v.SetDefault("sync.chunk_size", "1MB")
	v.SetDefault("log.level", "info")
	v.SetDefault("api.max_retries", 3)
	v.SetDefault("api.request_timeout", 30)
	// ... (add other relevant defaults from setViperDefaults if their absence affects the test outcome for missing keys)

	cfg, err := LoadFromViper(v)
	require.NoError(t, err, "LoadFromViper() should not produce an error")
	require.NotNil(t, cfg, "LoadFromViper() should return a non-nil Config object")

	// Assert values from the file (these should override SetDefault values)
	assert.Equal(t, "debug", cfg.Log.Level, "Log.Level should be from file")
	assert.Equal(t, 5, cfg.Sync.MaxConcurrent, "Sync.MaxConcurrent should be from file")
	assert.Equal(t, "/tmp/my_custom_downloads", cfg.Sync.DefaultDirectory, "Sync.DefaultDirectory should be from file")
	assert.Equal(t, 60, cfg.API.RequestTimeout, "API.RequestTimeout should be from file")

	// Fields not in the file should take values from v.SetDefault (which mirror setViperDefaults)
	assert.Equal(t, 3, cfg.API.MaxRetries, "API.MaxRetries should be default (3)")
	assert.Equal(t, "1MB", cfg.Sync.ChunkSize, "Sync.ChunkSize should be default ('1MB')")
}

func TestGetChunkSizeBytesRefined(t *testing.T) {
	tests := []struct {
		name     string
		chunkStr string // Value to set in cfg.Sync.ChunkSize
		expected int64
	}{
		{name: "10KB", chunkStr: "10KB", expected: 10 * 1024},
		{name: "2MB", chunkStr: "2MB", expected: 2 * 1024 * 1024},
		{name: "1GB", chunkStr: "1GB", expected: 1 * 1024 * 1024 * 1024},
		{name: "512 bytes as string", chunkStr: "512", expected: 512},
		{name: "empty string (uses method's internal default 1MB)", chunkStr: "", expected: 1 * 1024 * 1024},
		{name: "invalid unit", chunkStr: "10XX", expected: 10}, // fmt.Sscanf("10XX", "%d...", &val) yields val=10
		{name: "just number (bytes)", chunkStr: "2048", expected: 2048},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfgForTest := &Config{
				Sync: SyncConfig{ChunkSize: tt.chunkStr},
			}
			actual, err := cfgForTest.GetChunkSizeBytes()
			require.NoError(t, err) // Sscanf doesn't return error here, just 0 for value if parse fails
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetBandwidthLimitBytes(t *testing.T) {
    tests := []struct {
        name         string
        syncBandwidthLimit int    // Value to set in cfg.Sync.BandwidthLimit (MB)
        expected     int64
    }{
        {name: "10MBps", syncBandwidthLimit: 10, expected: 10 * 1024 * 1024},
        {name: "2MBps", syncBandwidthLimit: 2, expected: 2 * 1024 * 1024},
        {name: "0 (unlimited)", syncBandwidthLimit: 0, expected: 0},
        {name: "-5 (unlimited)", syncBandwidthLimit: -5, expected: 0}, // Negative should also be unlimited
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cfgForTest := &Config{
				Sync: SyncConfig{BandwidthLimit: tt.syncBandwidthLimit},
			}
            actual := cfgForTest.GetBandwidthLimitBytes()
            assert.Equal(t, tt.expected, actual)
        })
    }
}
