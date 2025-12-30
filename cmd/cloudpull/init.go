package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"

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

// setupConfig holds the configuration values collected during init.
type setupConfig struct {
	DefaultSyncDir  string
	MaxConcurrent   string
	ChunkSize       string
	BandwidthLimit  string
	EnableBandwidth bool
}

// checkExistingConfig checks if a config already exists and prompts for overwrite.
// Returns true if we should continue with setup, false if user declined.
func checkExistingConfig(configPath string) (bool, error) {
	_, err := os.Stat(configPath)
	if os.IsNotExist(err) {
		return true, nil // Config doesn't exist, continue with setup
	}
	if err != nil {
		return false, fmt.Errorf("failed to check config file: %w", err)
	}

	var overwrite bool
	prompt := &survey.Confirm{
		Message: "CloudPull is already configured. Reconfiguring will:\n" +
			"  • Delete your current configuration settings\n" +
			"  • Remove saved Google Drive authentication\n" +
			"  • Clear all sync history and progress\n" +
			"  • Require re-authentication with Google\n\n" +
			"Do you want to proceed with reconfiguration?",
		Default: false,
	}
	if err := survey.AskOne(prompt, &overwrite); err != nil {
		return false, fmt.Errorf("failed to get user input: %w", err)
	}
	return overwrite, nil
}

// getCredentialsPath prompts for and validates the credentials file path.
func getCredentialsPath() (string, error) {
	credsPath := credentialsFile

	if credsPath == "" {
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
		if err := survey.AskOne(prompt, &credsPath, survey.WithValidator(survey.Required)); err != nil {
			return "", fmt.Errorf("failed to get credentials path: %w", err)
		}
	}

	// Expand tilde in credentials file path
	if strings.HasPrefix(credsPath, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}
		credsPath = filepath.Join(homeDir, credsPath[1:])
	}

	// Validate credentials file
	if _, err := os.Stat(credsPath); err != nil {
		return "", fmt.Errorf("credentials file not found: %s", credsPath)
	}

	return credsPath, nil
}

// promptSetupConfig prompts for configuration settings.
func promptSetupConfig() (*setupConfig, error) {
	fmt.Println(color.YellowString("\n⚙️  Step 2: Configuration"))

	cfg := &setupConfig{}
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

	if err := survey.Ask(questions, cfg); err != nil {
		return nil, fmt.Errorf("failed to collect configuration: %w", err)
	}

	if cfg.EnableBandwidth {
		bandwidthPrompt := &survey.Input{
			Message: "Bandwidth limit (MB/s):",
			Default: "10",
		}
		if err := survey.AskOne(bandwidthPrompt, &cfg.BandwidthLimit); err != nil {
			return nil, fmt.Errorf("failed to get bandwidth limit: %w", err)
		}
	}

	return cfg, nil
}

// saveSetupConfig saves the configuration to file.
func saveSetupConfig(configPath, credsPath string, cfg *setupConfig) error {
	fmt.Println(color.YellowString("\n💾 Step 3: Saving Configuration"))

	maxConcurrent, err := strconv.Atoi(cfg.MaxConcurrent)
	if err != nil {
		return fmt.Errorf("invalid max concurrent value: %w", err)
	}

	var bandwidthLimit int
	if cfg.EnableBandwidth {
		bandwidthLimit, err = strconv.Atoi(cfg.BandwidthLimit)
		if err != nil {
			return fmt.Errorf("invalid bandwidth limit value: %w", err)
		}
	}

	chunkSizeBytes, err := parseChunkSize(cfg.ChunkSize)
	if err != nil {
		return fmt.Errorf("invalid chunk size: %w", err)
	}

	viper.Set("credentials_file", credsPath)
	viper.Set("sync.default_directory", cfg.DefaultSyncDir)
	viper.Set("sync.max_concurrent", maxConcurrent)
	viper.Set("sync.chunk_size", cfg.ChunkSize)
	viper.Set("sync.chunk_size_bytes", chunkSizeBytes)
	if cfg.EnableBandwidth {
		viper.Set("sync.bandwidth_limit", bandwidthLimit)
	}

	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0750); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := viper.WriteConfigAs(configPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	return nil
}

// performAuthentication handles the OAuth2 authentication flow.
func performAuthentication() error {
	fmt.Println(color.YellowString("\n🔐 Step 4: Authentication"))
	fmt.Println("CloudPull needs to authenticate with Google Drive.")

	if skipBrowser {
		fmt.Println("Run 'cloudpull auth' to complete authentication.")
		return nil
	}

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

	fmt.Println("\nStarting authentication flow...")
	if err := application.Authenticate(context.Background()); err != nil {
		var oauth2Err *oauth2.RetrieveError
		if errors.As(err, &oauth2Err) && strings.Contains(oauth2Err.ErrorDescription, "access_denied") {
			return fmt.Errorf("authentication canceled: user denied access")
		}
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("authentication canceled by user")
		}
		return fmt.Errorf("authentication failed: %w", err)
	}

	return nil
}

func runInit(cmd *cobra.Command, args []string) error {
	fmt.Println(color.CyanString("🚀 Welcome to CloudPull Setup"))
	fmt.Println()

	// Get config path
	configPath := viper.ConfigFileUsed()
	if configPath == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".cloudpull", "config.yaml")
	}

	// Check if already initialized
	shouldContinue, err := checkExistingConfig(configPath)
	if err != nil {
		return err
	}
	if !shouldContinue {
		return nil
	}

	// Get credentials path
	credsPath, err := getCredentialsPath()
	if err != nil {
		return err
	}

	// Prompt for configuration
	cfg, err := promptSetupConfig()
	if err != nil {
		return err
	}

	// Save configuration
	if err := saveSetupConfig(configPath, credsPath, cfg); err != nil {
		return err
	}

	// Perform authentication
	if err := performAuthentication(); err != nil {
		return err
	}

	fmt.Println(color.GreenString("\n✅ CloudPull initialized successfully!"))
	fmt.Println("\nNext steps:")
	fmt.Println("  • Run 'cloudpull sync' to start syncing")
	fmt.Println("  • Run 'cloudpull config' to view/edit settings")
	fmt.Println("  • Run 'cloudpull --help' for more commands")

	return nil
}

func parseChunkSize(size string) (int64, error) {
	size = strings.ToUpper(strings.TrimSpace(size))
	multiplier := int64(1)

	switch {
	case strings.HasSuffix(size, "KB"):
		multiplier = 1024
		size = strings.TrimSuffix(size, "KB")
	case strings.HasSuffix(size, "MB"):
		multiplier = 1024 * 1024
		size = strings.TrimSuffix(size, "MB")
	case strings.HasSuffix(size, "GB"):
		multiplier = 1024 * 1024 * 1024
		size = strings.TrimSuffix(size, "GB")
	}

	var value int64
	n, err := fmt.Sscanf(size, "%d", &value)
	if err != nil {
		return 0, fmt.Errorf("failed to parse chunk size: %w", err)
	}
	if n != 1 {
		return 0, fmt.Errorf("invalid chunk size format: %s", size)
	}
	if value <= 0 {
		return 0, fmt.Errorf("chunk size must be positive, got: %d", value)
	}

	return value * multiplier, nil
}
