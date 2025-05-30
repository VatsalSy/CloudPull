package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/VatsalSy/CloudPull/internal/app"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize CloudPull with Google Drive authentication",
	Long: `Initialize CloudPull by setting up authentication with Google Drive.

This command will guide you through:
  1. Creating a Google Cloud project (if needed)
  2. Enabling the Google Drive API
  3. Setting up OAuth2 credentials
  4. Authorizing CloudPull to access your Drive`,
	Example: `  # Interactive setup
  cloudpull init

  # Non-interactive with credentials file
  cloudpull init --credentials-file ~/Downloads/credentials.json`,
	RunE: runInit,
}

var (
	credentialsFile string
	skipBrowser     bool
)

func init() {
	initCmd.Flags().StringVarP(&credentialsFile, "credentials-file", "c", "",
		"Path to OAuth2 credentials JSON file")
	initCmd.Flags().BoolVar(&skipBrowser, "skip-browser", false,
		"Don't automatically open browser for authentication")
}

func runInit(cmd *cobra.Command, args []string) error {
	fmt.Println(color.CyanString("🚀 Welcome to CloudPull Setup"))
	fmt.Println()

	// Check if already initialized
	configPath := viper.ConfigFileUsed()
	if configPath == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".cloudpull", "config.yaml")
	}

	if _, err := os.Stat(configPath); err == nil {
		var overwrite bool
		prompt := &survey.Confirm{
			Message: "CloudPull is already configured. Reconfigure?",
			Default: false,
		}
		survey.AskOne(prompt, &overwrite)
		if !overwrite {
			return nil
		}
	}

	// Step 1: Get credentials
	if credentialsFile == "" {
		fmt.Println(color.YellowString("\n📋 Step 1: Google Cloud Credentials"))
		fmt.Println("To use CloudPull, you need OAuth2 credentials from Google Cloud Console.")
		fmt.Println("\nFollow these steps:")
		fmt.Println("1. Go to https://console.cloud.google.com/")
		fmt.Println("2. Create a new project or select existing")
		fmt.Println("3. Enable Google Drive API")
		fmt.Println("4. Create OAuth2 credentials (Desktop application)")
		fmt.Println("5. Download the credentials JSON file")
		fmt.Println()

		prompt := &survey.Input{
			Message: "Path to credentials JSON file:",
			Suggest: func(toComplete string) []string {
				files, _ := filepath.Glob(toComplete + "*.json")
				return files
			},
		}
		survey.AskOne(prompt, &credentialsFile, survey.WithValidator(survey.Required))
	}

	// Validate credentials file
	if _, err := os.Stat(credentialsFile); err != nil {
		return fmt.Errorf("credentials file not found: %s", credentialsFile)
	}

	// Step 2: Configure settings
	fmt.Println(color.YellowString("\n⚙️  Step 2: Configuration"))

	var config struct {
		DefaultSyncDir  string
		MaxConcurrent   string
		ChunkSize       string
		EnableBandwidth bool
		BandwidthLimit  string
	}

	questions := []*survey.Question{
		{
			Name: "DefaultSyncDir",
			Prompt: &survey.Input{
				Message: "Default sync directory:",
				Default: filepath.Join(os.Getenv("HOME"), "CloudPull"),
			},
		},
		{
			Name: "MaxConcurrent",
			Prompt: &survey.Input{
				Message: "Maximum concurrent downloads:",
				Default: "3",
			},
		},
		{
			Name: "ChunkSize",
			Prompt: &survey.Select{
				Message: "Download chunk size:",
				Options: []string{"256KB", "512KB", "1MB", "2MB", "4MB"},
				Default: "1MB",
			},
		},
		{
			Name: "EnableBandwidth",
			Prompt: &survey.Confirm{
				Message: "Enable bandwidth limiting?",
				Default: false,
			},
		},
	}

	if err := survey.Ask(questions, &config); err != nil {
		return err
	}

	if config.EnableBandwidth {
		bandwidthPrompt := &survey.Input{
			Message: "Bandwidth limit (MB/s):",
			Default: "10",
		}
		if err := survey.AskOne(bandwidthPrompt, &config.BandwidthLimit); err != nil {
			return err
		}
	}

	// Step 3: Save configuration
	fmt.Println(color.YellowString("\n💾 Step 3: Saving Configuration"))

	// Parse numeric values
	maxConcurrent, err := strconv.Atoi(config.MaxConcurrent)
	if err != nil {
		return fmt.Errorf("invalid max concurrent value: %w", err)
	}

	var bandwidthLimit int
	if config.EnableBandwidth {
		bandwidthLimit, err = strconv.Atoi(config.BandwidthLimit)
		if err != nil {
			return fmt.Errorf("invalid bandwidth limit value: %w", err)
		}
	}

	// Parse chunk size to bytes
	chunkSizeBytes := parseChunkSize(config.ChunkSize)

	viper.Set("credentials_file", credentialsFile)
	viper.Set("sync.default_directory", config.DefaultSyncDir)
	viper.Set("sync.max_concurrent", maxConcurrent)
	viper.Set("sync.chunk_size", config.ChunkSize)
	viper.Set("sync.chunk_size_bytes", chunkSizeBytes)
	if config.EnableBandwidth {
		viper.Set("sync.bandwidth_limit", bandwidthLimit)
	}

	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0750); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := viper.WriteConfigAs(configPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	// Step 4: Authenticate
	fmt.Println(color.YellowString("\n🔐 Step 4: Authentication"))
	fmt.Println("CloudPull needs to authenticate with Google Drive.")

	if !skipBrowser {
		// Initialize app
		application, err := app.New()
		if err != nil {
			return fmt.Errorf("failed to create application: %w", err)
		}

		if err := application.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize application: %w", err)
		}

		if err := application.InitializeAuth(); err != nil {
			return fmt.Errorf("failed to initialize authentication: %w", err)
		}

		// Perform authentication
		fmt.Println("\nStarting authentication flow...")
		if err := application.Authenticate(context.Background()); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	} else {
		fmt.Println("Run 'cloudpull auth' to complete authentication.")
	}

	fmt.Println(color.GreenString("\n✅ CloudPull initialized successfully!"))
	fmt.Println("\nNext steps:")
	fmt.Println("  • Run 'cloudpull sync' to start syncing")
	fmt.Println("  • Run 'cloudpull config' to view/edit settings")
	fmt.Println("  • Run 'cloudpull --help' for more commands")

	return nil
}

func parseChunkSize(size string) int64 {
	size = strings.ToUpper(strings.TrimSpace(size))
	multiplier := int64(1)

	if strings.HasSuffix(size, "KB") {
		multiplier = 1024
		size = strings.TrimSuffix(size, "KB")
	} else if strings.HasSuffix(size, "MB") {
		multiplier = 1024 * 1024
		size = strings.TrimSuffix(size, "MB")
	} else if strings.HasSuffix(size, "GB") {
		multiplier = 1024 * 1024 * 1024
		size = strings.TrimSuffix(size, "GB")
	}

	var value int64
	fmt.Sscanf(size, "%d", &value)
	return value * multiplier
}
