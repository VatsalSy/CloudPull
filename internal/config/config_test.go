package config

import (
	"os"
	"path/filepath"
	"testing"
	"time" // Ensure time is imported

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

func TestLoadWithEnvOverrides(t *testing.T) {
	// Simple Viper Env Test
	t.Setenv("CLOUDPULL_TESTKEY", "env_value_direct")
	vDirect := viper.New()
	vDirect.SetEnvPrefix("CLOUDPULL")
	vDirect.AutomaticEnv()
	assert.Equal(t, "env_value_direct", vDirect.GetString("testkey"), "Direct viper GetString for env var failed")

	tempDir := t.TempDir()
	tempConfigFile := filepath.Join(tempDir, "env_override_config.yaml")

	// 1. Create a base config file
	baseConfigContent := `
log:
  level: "info" # Will be overridden by env
sync:
  max_concurrent: 3 # Will be overridden by env
api:
  request_timeout: 30 # Will remain from file
`
	err := os.WriteFile(tempConfigFile, []byte(baseConfigContent), 0600)
	require.NoError(t, err, "Failed to write base config file for env override test")

	// 2. Set environment variables for the test
	t.Setenv("CLOUDPULL_LOG_LEVEL", "debug")
	t.Setenv("CLOUDPULL_SYNC_MAX_CONCURRENT", "10")
	t.Setenv("CLOUDPULL_API_MAX_RETRIES", "7") // Not in file, will override default
	t.Setenv("CLOUDPULL_NEW_SETTING_FROM_ENV", "env_value_specific")


	// 3. Configure a new local viper instance
	v := viper.New()

	// Set up environment variable handling first
	v.SetEnvPrefix("CLOUDPULL")
	// Default replacer converts KEY to KEY by replacing . with _ for env var names.
	// e.g., log.level becomes CLOUDPULL_LOG_LEVEL. This is usually default.
	// v.SetEnvKeyReplacer(strings.NewReplacer(".", "_")) // Explicitly set if needed, but usually default works.
	v.AutomaticEnv() // Make viper aware of environment variables

	// 4. Set defaults (these have the lowest precedence)
	// Viper will use these if no value is found in config file or env vars.
	homeDirForDefaults, err := os.UserHomeDir()
	require.NoError(t, err)
	v.SetDefault("sync.default_directory", filepath.Join(homeDirForDefaults, "CloudPull"))
	v.SetDefault("log.level", "default_info")
	v.SetDefault("sync.max_concurrent", 1)
	v.SetDefault("api.request_timeout", 15)
	v.SetDefault("api.max_retries", 3)

	// 5. Set config file path and read it.
	// Values from the config file will override defaults.
	// Environment variables (due to AutomaticEnv) should override both file and defaults.
	v.SetConfigFile(tempConfigFile)
	err = v.ReadInConfig()
	// If the file does not exist, viper.ReadInConfig() returns a specific error.
	// We created it, so it should be read. If not, defaults + env would apply.
	if _, ok := err.(viper.ConfigFileNotFoundError); ok && tempConfigFile != "" {
		// This means the tempConfigFile was not found by viper, which is an error in test setup.
		require.NoError(t, err, "Viper could not find the temp config file: "+tempConfigFile)
	} else if err != nil {
		// Other types of errors during read
		require.NoError(t, err, "Error reading config file")
	}

	// Explicitly bind environment variables AFTER reading config and AFTER AutomaticEnv.
	// This ensures they have higher precedence for the specified keys.
	// AutomaticEnv is still useful for other env vars not explicitly bound.
	require.NoError(t, v.BindEnv("log.level", "CLOUDPULL_LOG_LEVEL"))
	require.NoError(t, v.BindEnv("sync.max_concurrent", "CLOUDPULL_SYNC_MAX_CONCURRENT"))
	require.NoError(t, v.BindEnv("api.max_retries", "CLOUDPULL_API_MAX_RETRIES"))
	// For NEW_SETTING_FROM_ENV, AutomaticEnv should handle it if it's not a nested key,
	// or we can bind it too if needed: require.NoError(t, v.BindEnv("new_setting_from_env", "CLOUDPULL_NEW_SETTING_FROM_ENV"))


	// Debug: Check viper's understanding of the values BEFORE LoadFromViper
	assert.Equal(t, "debug", v.GetString("log.level"), "[Viper Direct] Log.Level should be from env 'debug'")
	assert.Equal(t, 10, v.GetInt("sync.max_concurrent"), "[Viper Direct] Sync.MaxConcurrent should be from env '10'")
	assert.Equal(t, 30, v.GetInt("api.request_timeout"), "[Viper Direct] API.RequestTimeout should be from file '30'") // This is not overridden by env
	assert.Equal(t, 7, v.GetInt("api.max_retries"), "[Viper Direct] API.MaxRetries should be from env '7'")
	assert.Equal(t, "env_value_specific", v.GetString("new_setting_from_env"), "[Viper Direct] New setting from env var not found") // Corrected expected value

	// 6. Load configuration using the local viper instance
	cfg, err := LoadFromViper(v) // LoadFromViper unmarshals 'v' and then runs setDefaults(cfg)
	require.NoError(t, err, "LoadFromViper() with env overrides failed")
	require.NotNil(t, cfg, "Config object is nil after load with env overrides")

	// 7. Assertions
	// Viper's precedence: Env > File > Defaults set by v.SetDefault()
	// Then, LoadFromViper calls setDefaults(cfg) which can alter zero-valued fields in cfg.

	assert.Equal(t, "debug", cfg.Log.Level, "Log.Level should be from env 'debug'") // Env "debug" > File "info" > Default "default_info". setDefaults sets to "info" if empty.
	assert.Equal(t, 10, cfg.Sync.MaxConcurrent, "Sync.MaxConcurrent should be from env '10'") // Env 10 > File 3 > Default 1. setDefaults sets to 3 if 0.

	// API.RequestTimeout: Env (not set) > File 30 > Default 15. Not in setDefaults.
	assert.Equal(t, 30, cfg.API.RequestTimeout, "API.RequestTimeout should be from file '30'")

	// API.MaxRetries: Env 7 > File (not set) > Default 3. Not in setDefaults.
	assert.Equal(t, 7, cfg.API.MaxRetries, "API.MaxRetries should be from env '7'")

	// For a key only in env, use the GetString method from Config, which proxies to its viper instance
	assert.Equal(t, "env_value_specific", cfg.GetString("new_setting_from_env"), "New setting from env var not found") // Corrected expected value
}

func TestSaveConfig(t *testing.T) {
	viper.Reset() // Reset global viper for a clean slate
	tempDir := t.TempDir()
	tempSavePath := filepath.Join(tempDir, "saved_config.yaml")

	// 1. Set values directly on the global viper instance
	viper.Set("log.level", "error")
	viper.Set("sync.max_concurrent", 12)
	viper.Set("api.request_timeout", 75)
	// Set a value that would typically come from setViperDefaults
	viper.SetDefault("api.max_retries", 3) // Ensure this default is active in global viper
	viper.Set("api.max_retries", viper.GetInt("api.max_retries")) // Explicitly set it so it's "in use"

	// 2. Tell viper where to save this configuration
	viper.SetConfigFile(tempSavePath)

	// 3. Call the Save function from the config package
	err := Save()
	require.NoError(t, err, "config.Save() failed")

	// 4. Verify the file was created
	_, err = os.Stat(tempSavePath)
	require.NoError(t, err, "Saved config file does not exist at tempSavePath")

	// 5. Read back the saved file with a new viper instance to verify its content
	readerViper := viper.New()
	readerViper.SetConfigFile(tempSavePath)
	err = readerViper.ReadInConfig()
	require.NoError(t, err, "Failed to read back saved config file")

	assert.Equal(t, "error", readerViper.GetString("log.level"), "Saved log.level is incorrect")
	assert.Equal(t, 12, readerViper.GetInt("sync.max_concurrent"), "Saved sync.max_concurrent is incorrect")
	assert.Equal(t, 75, readerViper.GetInt("api.request_timeout"), "Saved api.request_timeout is incorrect")
	assert.Equal(t, 3, readerViper.GetInt("api.max_retries"), "Saved api.max_retries (default) is incorrect")
}

func TestConfigPathAndDataDir(t *testing.T) {
	viper.Reset()
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err, "Failed to get user home directory")

	// Test ConfigPath()
	// 1. When no config file is explicitly set or used by viper
	expectedDefaultConfigPath := filepath.Join(homeDir, ".cloudpull", "config.yaml")
	assert.Equal(t, expectedDefaultConfigPath, ConfigPath(), "Default ConfigPath is incorrect")

	// 2. After setting a config file path in viper
	customConfigPath := filepath.Join(t.TempDir(), "custom_config.yaml")
	viper.SetConfigFile(customConfigPath)
	// Note: ConfigPath() uses viper.ConfigFileUsed(). This function returns a value
	// only if viper has successfully read a config file or if the path was set via SetConfigFile
	// AND THEN ReadInConfig was called (even if file doesn't exist yet for ReadInConfig)
	// or if WriteConfigAs was used.
	// For a more direct test of SetConfigFile's effect if ReadInConfig wasn't called,
	// one might need to inspect internal viper state, which is not ideal.
	// However, if Load() was called with 'customConfigPath', then ConfigFileUsed() would be set.
	// Let's simulate Load() having set it for this test.
	// A simple viper.SetConfigFile might not be enough for viper.ConfigFileUsed() to pick it up without a read attempt.
	// To make viper.ConfigFileUsed() reliable, we can make it "read" the (possibly non-existent) file.
	_ = viper.ReadInConfig() // Attempt to read, doesn't matter if it fails for this path test
	assert.Equal(t, customConfigPath, ConfigPath(), "ConfigPath after SetConfigFile is incorrect")

	viper.Reset() // Reset to test default DataDir behavior without custom config path interference

	// Test DataDir()
	expectedDefaultDataDir := filepath.Join(homeDir, ".cloudpull")
	assert.Equal(t, expectedDefaultDataDir, DataDir(), "Default DataDir is incorrect")

	// Test cfg.GetDataDir()
	// Load a default config to get a Config instance
	cfg, err := Load(filepath.Join(t.TempDir(), "another_non_existent_config.yaml")) // Load with a path to trigger initViper once
	require.NoError(t, err)
	assert.Equal(t, expectedDefaultDataDir, cfg.GetDataDir(), "cfg.GetDataDir() is incorrect")
}

func TestGenericGetters(t *testing.T) {
	v := viper.New() // Use a local viper instance

	v.Set("mykey.string", "testval")
	v.Set("mykey.int", 123)
	v.Set("mykey.durationsec", 5) // Stored as int (seconds) for GetDuration
	v.Set("mykey.float", 12.34)
	v.Set("mykey.bool", true)


	// Apply some defaults to the local viper instance to mimic setViperDefaults
	// This is important because LoadFromViper doesn't call setViperDefaults itself.
	// However, for GetString, GetInt etc., these are direct viper calls proxied by the Config struct,
	// so the defaults set here will be available if the keys are not found from the Set() calls above.
	// For this test, we are testing keys that *are* set, so defaults are less critical here.
	// If we were testing GetString for a key that wasn't Set, then SetDefault would be important.

	cfg, err := LoadFromViper(v)
	require.NoError(t, err, "LoadFromViper failed")
	require.NotNil(t, cfg)

	assert.Equal(t, "testval", cfg.GetString("mykey.string"), "GetString failed")
	assert.Equal(t, 123, cfg.GetInt("mykey.int"), "GetInt failed")
	assert.Equal(t, 5*time.Second, cfg.GetDuration("mykey.durationsec"), "GetDuration failed") // time.Second will be resolved by import
	assert.Equal(t, 12.34, cfg.GetFloat64("mykey.float"), "GetFloat64 failed")
	assert.True(t, cfg.viper.GetBool("mykey.bool"), "GetBool (initial true) failed") // Use cfg.viper.GetBool

	// For GetBool, viper's GetBool is quite flexible with string parsing ("true", "false", "1", "0")
	// Here we set it as a proper boolean.
	v.Set("mykey.booltrue", true)
	v.Set("mykey.boolfalse", false)
	// cfg variable already holds the config loaded from viper instance 'v'.
	// No need to reload into cfgAfterBool unless v was somehow disassociated from cfg.
	// cfg.viper should still point to v.

	assert.True(t, cfg.viper.GetBool("mykey.booltrue"), "GetBool (true) failed")  // Use cfg.viper.GetBool
	assert.False(t, cfg.viper.GetBool("mykey.boolfalse"), "GetBool (false) failed") // Use cfg.viper.GetBool


	// Test GetInt64
	v.Set("mykey.int64", int64(1234567890123))
	cfgAfterInt64, _ := LoadFromViper(v)
	assert.Equal(t, int64(1234567890123), cfgAfterInt64.GetInt64("mykey.int64"), "GetInt64 failed")

}
