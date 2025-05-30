package main

import (
  "fmt"
  "os"
  "path/filepath"

  "github.com/spf13/cobra"
  "github.com/spf13/viper"
)

var (
  cfgFile string
  verbose bool
  rootCmd = &cobra.Command{
    Use:   "cloudpull",
    Short: "A powerful tool for syncing files from Google Drive",
    Long: `CloudPull is a CLI tool that provides efficient file synchronization
from Google Drive to your local filesystem.

Features:
  • Selective sync with pattern matching
  • Resume interrupted downloads
  • Real-time progress tracking
  • Bandwidth throttling
  • Multiple account support`,
    Version: "0.1.0",
  }
)

// Execute runs the root command
func Execute() error {
  return rootCmd.Execute()
}

func init() {
  cobra.OnInitialize(initConfig)

  // Global flags
  rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", 
    "config file (default is $HOME/.cloudpull/config.yaml)")
  rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, 
    "verbose output")

  // Bind flags to viper
  viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))

  // Add commands
  rootCmd.AddCommand(initCmd)
  rootCmd.AddCommand(authCmd)
  rootCmd.AddCommand(syncCmd)
  rootCmd.AddCommand(resumeCmd)
  rootCmd.AddCommand(statusCmd)
  rootCmd.AddCommand(configCmd)
  rootCmd.AddCommand(cleanupCmd)

  // Enable shell completion
  rootCmd.CompletionOptions.DisableDefaultCmd = false
}

func initConfig() {
  if cfgFile != "" {
    // Use config file from the flag
    viper.SetConfigFile(cfgFile)
  } else {
    // Find home directory
    home, err := os.UserHomeDir()
    cobra.CheckErr(err)

    // Search config in home directory
    configDir := filepath.Join(home, ".cloudpull")
    viper.AddConfigPath(configDir)
    viper.SetConfigType("yaml")
    viper.SetConfigName("config")

    // Create config directory if it doesn't exist
    if _, err := os.Stat(configDir); os.IsNotExist(err) {
      os.MkdirAll(configDir, 0755)
    }
  }

  // Environment variables
  viper.SetEnvPrefix("CLOUDPULL")
  viper.AutomaticEnv()

  // Read config file
  if err := viper.ReadInConfig(); err == nil {
    if verbose {
      fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
    }
  }
}