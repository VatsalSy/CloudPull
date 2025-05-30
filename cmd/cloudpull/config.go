package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CloudPull configuration",
	Long: `View and modify CloudPull configuration settings.

Configuration can be managed through:
  • Interactive prompts
  • Direct key-value updates
  • Environment variables (CLOUDPULL_*)
  • Direct file editing`,
	Example: `  # View all configuration
  cloudpull config

  # View specific setting
  cloudpull config get sync.max_concurrent

  # Update setting
  cloudpull config set sync.max_concurrent 5

  # Reset to defaults
  cloudpull config reset

  # Edit config file directly
  cloudpull config edit`,
}

var (
	configGetCmd = &cobra.Command{
		Use:   "get [key]",
		Short: "Get configuration value",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runConfigGet,
	}

	configSetCmd = &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set configuration value",
		Args:  cobra.ExactArgs(2),
		RunE:  runConfigSet,
	}

	configResetCmd = &cobra.Command{
		Use:   "reset",
		Short: "Reset configuration to defaults",
		RunE:  runConfigReset,
	}

	configEditCmd = &cobra.Command{
		Use:   "edit",
		Short: "Edit configuration file in default editor",
		RunE:  runConfigEdit,
	}
)

func init() {
	// Add subcommands
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configResetCmd)
	configCmd.AddCommand(configEditCmd)

	// Set default run function
	configCmd.Run = func(cmd *cobra.Command, args []string) {
		runConfigList()
	}
}

func runConfigList() {
	fmt.Println(color.CyanString("⚙️  CloudPull Configuration"))
	fmt.Println()

	configFile := viper.ConfigFileUsed()
	if configFile == "" {
		home, _ := os.UserHomeDir()
		configFile = filepath.Join(home, ".cloudpull", "config.yaml")
	}
	fmt.Printf("Config file: %s\n\n", configFile)

	// Group configurations
	groups := map[string][]ConfigItem{
		"Authentication": {
			{"credentials_file", "OAuth2 credentials file", viper.GetString("credentials_file")},
			{"token_file", "Stored auth token", viper.GetString("token_file")},
		},
		"Sync Settings": {
			{"sync.default_directory", "Default sync directory", viper.GetString("sync.default_directory")},
			{"sync.max_concurrent", "Max concurrent downloads", fmt.Sprintf("%d", viper.GetInt("sync.max_concurrent"))},
			{"sync.chunk_size", "Download chunk size", viper.GetString("sync.chunk_size")},
			{"sync.bandwidth_limit", "Bandwidth limit (MB/s)", formatOptionalInt(viper.GetInt("sync.bandwidth_limit"))},
			{"sync.resume_on_failure", "Auto-resume on failure", fmt.Sprintf("%v", viper.GetBool("sync.resume_on_failure"))},
		},
		"File Handling": {
			{"files.skip_duplicates", "Skip duplicate files", fmt.Sprintf("%v", viper.GetBool("files.skip_duplicates"))},
			{"files.preserve_timestamps", "Preserve timestamps", fmt.Sprintf("%v", viper.GetBool("files.preserve_timestamps"))},
			{"files.follow_shortcuts", "Follow Drive shortcuts", fmt.Sprintf("%v", viper.GetBool("files.follow_shortcuts"))},
		},
		"Advanced": {
			{"cache.enabled", "Enable metadata cache", fmt.Sprintf("%v", viper.GetBool("cache.enabled"))},
			{"cache.ttl", "Cache TTL (minutes)", fmt.Sprintf("%d", viper.GetInt("cache.ttl"))},
			{"log.level", "Log level", viper.GetString("log.level")},
			{"log.file", "Log file path", viper.GetString("log.file")},
		},
	}

	// Display each group
	for groupName, items := range groups {
		fmt.Println(color.YellowString(groupName + ":"))

		t := table.NewWriter()
		t.SetStyle(table.StyleLight)
		t.SetColumnConfigs([]table.ColumnConfig{
			{Number: 1, AutoMerge: false, WidthMax: 30},
			{Number: 2, AutoMerge: false, WidthMax: 40},
			{Number: 3, AutoMerge: false, WidthMax: 40},
		})

		for _, item := range items {
			value := item.Value
			if value == "" || value == "0" || value == "<nil>" {
				value = color.New(color.FgHiBlack).Sprint("(not set)")
			}
			t.AppendRow(table.Row{item.Key, item.Description, value})
		}

		fmt.Println(t.Render())
		fmt.Println()
	}

	fmt.Println("Use 'cloudpull config set <key> <value>' to update settings")
	fmt.Println("Use 'cloudpull config edit' to edit the config file directly")
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		// Show all as key-value pairs
		settings := viper.AllSettings()
		for key, value := range flattenMap("", settings) {
			fmt.Printf("%s=%v\n", key, value)
		}
		return nil
	}

	key := args[0]
	if !viper.IsSet(key) {
		return fmt.Errorf("configuration key not found: %s", key)
	}

	fmt.Println(viper.Get(key))
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	// Validate key
	validKeys := getAllValidKeys()
	if !contains(validKeys, key) {
		fmt.Printf(color.YellowString("Warning: '%s' is not a recognized configuration key\n"), key)
		var proceed bool
		prompt := &survey.Confirm{
			Message: "Set it anyway?",
			Default: false,
		}
		survey.AskOne(prompt, &proceed)
		if !proceed {
			return nil
		}
	}

	// Convert value to appropriate type
	oldValue := viper.Get(key)
	var newValue interface{}

	switch oldValue.(type) {
	case bool:
		newValue = strings.ToLower(value) == "true"
	case int, int64:
		fmt.Sscanf(value, "%d", &newValue)
	default:
		newValue = value
	}

	// Set value
	viper.Set(key, newValue)

	// Save configuration
	configFile := viper.ConfigFileUsed()
	if configFile == "" {
		home, _ := os.UserHomeDir()
		configFile = filepath.Join(home, ".cloudpull", "config.yaml")
	}

	if err := viper.WriteConfigAs(configFile); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Printf(color.GreenString("✓ Set %s = %v\n"), key, newValue)
	return nil
}

func runConfigReset(cmd *cobra.Command, args []string) error {
	fmt.Println(color.YellowString("⚠️  Warning: This will reset all configuration to defaults"))

	var confirm bool
	prompt := &survey.Confirm{
		Message: "Are you sure?",
		Default: false,
	}
	survey.AskOne(prompt, &confirm)
	if !confirm {
		return nil
	}

	// Create default configuration
	defaults := map[string]interface{}{
		"sync": map[string]interface{}{
			"default_directory": filepath.Join(os.Getenv("HOME"), "CloudPull"),
			"max_concurrent":    3,
			"chunk_size":        "1MB",
			"resume_on_failure": true,
		},
		"files": map[string]interface{}{
			"skip_duplicates":     true,
			"preserve_timestamps": true,
			"follow_shortcuts":    false,
		},
		"cache": map[string]interface{}{
			"enabled": true,
			"ttl":     60,
		},
		"log": map[string]interface{}{
			"level": "info",
			"file":  "",
		},
	}

	// Reset viper
	viper.Reset()
	for key, value := range defaults {
		viper.Set(key, value)
	}

	// Save configuration
	configFile := viper.ConfigFileUsed()
	if configFile == "" {
		home, _ := os.UserHomeDir()
		configFile = filepath.Join(home, ".cloudpull", "config.yaml")
	}

	if err := viper.WriteConfigAs(configFile); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Println(color.GreenString("✓ Configuration reset to defaults"))
	return nil
}

func runConfigEdit(cmd *cobra.Command, args []string) error {
	configFile := viper.ConfigFileUsed()
	if configFile == "" {
		home, _ := os.UserHomeDir()
		configFile = filepath.Join(home, ".cloudpull", "config.yaml")
	}

	// Ensure file exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// Create with current settings
		if err := viper.WriteConfigAs(configFile); err != nil {
			return fmt.Errorf("failed to create config file: %w", err)
		}
	}

	// Get editor
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
		if runtime.GOOS == "windows" {
			editor = "notepad"
		}
	}

	fmt.Printf("Opening %s in %s...\n", configFile, editor)

	// Open editor
	editorCmd := exec.Command(editor, configFile)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	if err := editorCmd.Run(); err != nil {
		return fmt.Errorf("failed to open editor: %w", err)
	}

	// Reload configuration
	viper.ReadInConfig()
	fmt.Println(color.GreenString("✓ Configuration reloaded"))

	return nil
}

type ConfigItem struct {
	Key         string
	Description string
	Value       string
}

func formatOptionalInt(value int) string {
	if value == 0 {
		return "(unlimited)"
	}
	return fmt.Sprintf("%d", value)
}

func flattenMap(prefix string, m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for key, value := range m {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		switch v := value.(type) {
		case map[string]interface{}:
			for k, val := range flattenMap(fullKey, v) {
				result[k] = val
			}
		default:
			result[fullKey] = value
		}
	}

	return result
}

func getAllValidKeys() []string {
	return []string{
		"credentials_file",
		"token_file",
		"sync.default_directory",
		"sync.max_concurrent",
		"sync.chunk_size",
		"sync.bandwidth_limit",
		"sync.resume_on_failure",
		"files.skip_duplicates",
		"files.preserve_timestamps",
		"files.follow_shortcuts",
		"cache.enabled",
		"cache.ttl",
		"log.level",
		"log.file",
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
