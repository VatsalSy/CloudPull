package config

import (
  "fmt"
  "os"
  "path/filepath"
  "strings"
  "sync"

  "github.com/spf13/viper"
  "time"
)

var (
  once   sync.Once
  config *Config
)

// Config represents the application configuration
type Config struct {
  // Authentication
  CredentialsFile string `mapstructure:"credentials_file"`
  TokenFile       string `mapstructure:"token_file"`

  // Sync settings
  Sync SyncConfig `mapstructure:"sync"`

  // API settings
  API APIConfig `mapstructure:"api"`

  // Error handling
  Errors ErrorConfig `mapstructure:"errors"`

  // File handling
  Files FileConfig `mapstructure:"files"`

  // Cache settings
  Cache CacheConfig `mapstructure:"cache"`

  // Logging
  Log LogConfig `mapstructure:"log"`

  // Application
  Version string `mapstructure:"version"`
}

// SyncConfig contains sync-related settings
type SyncConfig struct {
  DefaultDirectory    string `mapstructure:"default_directory"`
  MaxConcurrent       int    `mapstructure:"max_concurrent"`
  ChunkSize          string `mapstructure:"chunk_size"`
  ChunkSizeBytes     int64  `mapstructure:"chunk_size_bytes"`
  BandwidthLimit     int    `mapstructure:"bandwidth_limit"` // MB/s, 0 = unlimited
  ResumeOnFailure    bool   `mapstructure:"resume_on_failure"`
  RetryAttempts      int    `mapstructure:"retry_attempts"`
  RetryDelay         int    `mapstructure:"retry_delay"` // seconds
  MaxDepth           int    `mapstructure:"max_depth"`
  BatchSize          int    `mapstructure:"batch_size"`
  WalkerConcurrent   int    `mapstructure:"walker_concurrent"`
  QueueSize          int    `mapstructure:"queue_size"`
  ProgressInterval   int    `mapstructure:"progress_interval"` // seconds
  CheckpointInterval int    `mapstructure:"checkpoint_interval"` // seconds
  MaxErrors          int    `mapstructure:"max_errors"`
  MaxRetries         int    `mapstructure:"max_retries"`
}

// FileConfig contains file handling settings
type FileConfig struct {
  SkipDuplicates     bool     `mapstructure:"skip_duplicates"`
  PreserveTimestamps bool     `mapstructure:"preserve_timestamps"`
  FollowShortcuts    bool     `mapstructure:"follow_shortcuts"`
  IgnorePatterns     []string `mapstructure:"ignore_patterns"`
  ConvertGoogleDocs  bool     `mapstructure:"convert_google_docs"`
  GoogleDocsFormat   string   `mapstructure:"google_docs_format"` // pdf, docx, txt
}

// CacheConfig contains cache settings
type CacheConfig struct {
  Enabled   bool   `mapstructure:"enabled"`
  Directory string `mapstructure:"directory"`
  TTL       int    `mapstructure:"ttl"` // minutes
  MaxSize   int    `mapstructure:"max_size"` // MB
}

// LogConfig contains logging settings
type LogConfig struct {
  Level      string `mapstructure:"level"` // debug, info, warn, error
  Format     string `mapstructure:"format"` // json, text
  Output     string `mapstructure:"output"` // stdout, stderr, file
  File       string `mapstructure:"file"`
  MaxSize    int    `mapstructure:"max_size"`    // MB
  MaxBackups int    `mapstructure:"max_backups"`
  MaxAge     int    `mapstructure:"max_age"`     // days
  Compress   bool   `mapstructure:"compress"`
}

// APIConfig contains API-related settings
type APIConfig struct {
  MaxRetries      int `mapstructure:"max_retries"`
  RetryDelay      int `mapstructure:"retry_delay"` // seconds
  RequestTimeout  int `mapstructure:"request_timeout"` // seconds
  MaxConcurrent   int `mapstructure:"max_concurrent"`
  RateLimitPerSec int `mapstructure:"rate_limit"`
}

// ErrorConfig contains error handling settings
type ErrorConfig struct {
  MaxRetries      int     `mapstructure:"max_retries"`
  RetryDelay      int     `mapstructure:"retry_delay"` // seconds
  RetryMultiplier float64 `mapstructure:"retry_multiplier"`
  RetryMaxDelay   int     `mapstructure:"retry_max_delay"` // seconds
}

// Load initializes and loads the configuration
func Load(cfgFile ...string) (*Config, error) {
  once.Do(func() {
    configFile := ""
    if len(cfgFile) > 0 {
      configFile = cfgFile[0]
    }
    initViper(configFile)
  })

  config = &Config{}
  if err := viper.Unmarshal(config); err != nil {
    return nil, fmt.Errorf("failed to unmarshal config: %w", err)
  }

  // Set defaults if not configured
  setDefaults(config)

  return config, nil
}

// Get returns the current configuration
func Get() *Config {
  if config == nil {
    config, _ = Load("")
  }
  return config
}

// Save writes the current configuration to file
func Save() error {
  configFile := viper.ConfigFileUsed()
  if configFile == "" {
    home, _ := os.UserHomeDir()
    configFile = filepath.Join(home, ".cloudpull", "config.yaml")
  }

  // Ensure directory exists
  dir := filepath.Dir(configFile)
  if err := os.MkdirAll(dir, 0755); err != nil {
    return fmt.Errorf("failed to create config directory: %w", err)
  }

  return viper.WriteConfigAs(configFile)
}

// initViper sets up viper configuration
func initViper(cfgFile string) {
  if cfgFile != "" {
    viper.SetConfigFile(cfgFile)
  } else {
    home, _ := os.UserHomeDir()
    configDir := filepath.Join(home, ".cloudpull")
    
    viper.AddConfigPath(configDir)
    viper.SetConfigType("yaml")
    viper.SetConfigName("config")
  }

  // Environment variables
  viper.SetEnvPrefix("CLOUDPULL")
  viper.AutomaticEnv()

  // Set defaults
  setViperDefaults()

  // Read config file
  viper.ReadInConfig()
}

// setViperDefaults sets default values in viper
func setViperDefaults() {
  home, _ := os.UserHomeDir()

  // Sync defaults
  viper.SetDefault("sync.default_directory", filepath.Join(home, "CloudPull"))
  viper.SetDefault("sync.max_concurrent", 3)
  viper.SetDefault("sync.chunk_size", "1MB")
  viper.SetDefault("sync.bandwidth_limit", 0)
  viper.SetDefault("sync.resume_on_failure", true)
  viper.SetDefault("sync.retry_attempts", 3)
  viper.SetDefault("sync.retry_delay", 5)
  viper.SetDefault("sync.max_depth", -1)
  viper.SetDefault("sync.batch_size", 100)
  viper.SetDefault("sync.walker_concurrent", 5)
  viper.SetDefault("sync.queue_size", 1000)
  viper.SetDefault("sync.progress_interval", 1)
  viper.SetDefault("sync.checkpoint_interval", 30)
  viper.SetDefault("sync.max_errors", 100)
  viper.SetDefault("sync.max_retries", 3)

  // File defaults
  viper.SetDefault("files.skip_duplicates", true)
  viper.SetDefault("files.preserve_timestamps", true)
  viper.SetDefault("files.follow_shortcuts", false)
  viper.SetDefault("files.convert_google_docs", true)
  viper.SetDefault("files.google_docs_format", "pdf")
  viper.SetDefault("files.ignore_patterns", []string{
    "*.tmp",
    "~$*",
    ".DS_Store",
    "Thumbs.db",
  })

  // Cache defaults
  viper.SetDefault("cache.enabled", true)
  viper.SetDefault("cache.directory", filepath.Join(home, ".cloudpull", "cache"))
  viper.SetDefault("cache.ttl", 60)
  viper.SetDefault("cache.max_size", 100)

  // Log defaults
  viper.SetDefault("log.level", "info")
  viper.SetDefault("log.format", "text")
  viper.SetDefault("log.output", "stdout")
  viper.SetDefault("log.file", "")
  viper.SetDefault("log.max_size", 10)
  viper.SetDefault("log.max_backups", 3)
  viper.SetDefault("log.max_age", 7)
  viper.SetDefault("log.compress", true)

  // API defaults
  viper.SetDefault("api.max_retries", 3)
  viper.SetDefault("api.retry_delay", 5)
  viper.SetDefault("api.request_timeout", 30)
  viper.SetDefault("api.max_concurrent", 10)
  viper.SetDefault("api.rate_limit", 10)

  // Error defaults
  viper.SetDefault("errors.max_retries", 3)
  viper.SetDefault("errors.retry_delay", 1)
  viper.SetDefault("errors.retry_multiplier", 2.0)
  viper.SetDefault("errors.retry_max_delay", 60)

  // Version
  viper.SetDefault("version", "1.0.0")
}

// setDefaults ensures all config fields have sensible defaults
func setDefaults(cfg *Config) {
  home, _ := os.UserHomeDir()

  if cfg.Sync.DefaultDirectory == "" {
    cfg.Sync.DefaultDirectory = filepath.Join(home, "CloudPull")
  }
  
  if cfg.Sync.MaxConcurrent == 0 {
    cfg.Sync.MaxConcurrent = 3
  }
  
  if cfg.Sync.ChunkSize == "" {
    cfg.Sync.ChunkSize = "1MB"
  }

  if cfg.Cache.Directory == "" {
    cfg.Cache.Directory = filepath.Join(home, ".cloudpull", "cache")
  }

  if cfg.Log.Level == "" {
    cfg.Log.Level = "info"
  }
}

// GetChunkSizeBytes converts chunk size string to bytes
func (c *Config) GetChunkSizeBytes() (int64, error) {
  size := c.Sync.ChunkSize
  if size == "" {
    size = "1MB"
  }

  multiplier := int64(1)
  value := int64(0)

  if strings.HasSuffix(size, "KB") {
    multiplier = 1024
    fmt.Sscanf(size, "%dKB", &value)
  } else if strings.HasSuffix(size, "MB") {
    multiplier = 1024 * 1024
    fmt.Sscanf(size, "%dMB", &value)
  } else if strings.HasSuffix(size, "GB") {
    multiplier = 1024 * 1024 * 1024
    fmt.Sscanf(size, "%dGB", &value)
  } else {
    fmt.Sscanf(size, "%d", &value)
  }

  return value * multiplier, nil
}

// GetBandwidthLimitBytes converts bandwidth limit to bytes/second
func (c *Config) GetBandwidthLimitBytes() int64 {
  if c.Sync.BandwidthLimit <= 0 {
    return 0 // unlimited
  }
  return int64(c.Sync.BandwidthLimit) * 1024 * 1024 // MB/s to bytes/s
}

// ConfigPath returns the path to the config file
func ConfigPath() string {
  configFile := viper.ConfigFileUsed()
  if configFile == "" {
    home, _ := os.UserHomeDir()
    configFile = filepath.Join(home, ".cloudpull", "config.yaml")
  }
  return configFile
}

// DataDir returns the CloudPull data directory
func DataDir() string {
  home, _ := os.UserHomeDir()
  return filepath.Join(home, ".cloudpull")
}

// GetDataDir returns the CloudPull data directory
func (c *Config) GetDataDir() string {
  return DataDir()
}

// GetString returns a string value from viper
func (c *Config) GetString(key string) string {
  return viper.GetString(key)
}

// GetInt returns an int value from viper
func (c *Config) GetInt(key string) int {
  return viper.GetInt(key)
}

// GetInt64 returns an int64 value from viper
func (c *Config) GetInt64(key string) int64 {
  return viper.GetInt64(key)
}

// GetFloat64 returns a float64 value from viper
func (c *Config) GetFloat64(key string) float64 {
  return viper.GetFloat64(key)
}

// GetDuration returns a duration value from viper
func (c *Config) GetDuration(key string) time.Duration {
  // Get the value as int (seconds) and convert to duration
  seconds := viper.GetInt(key)
  return time.Duration(seconds) * time.Second
}

// GetLogLevel returns the log level
func (c *Config) GetLogLevel() string {
  return c.Log.Level
}