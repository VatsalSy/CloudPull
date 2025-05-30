package main

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/VatsalSy/CloudPull/internal/app"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate with Google Drive",
	Long: `Authenticate CloudPull with Google Drive.

This command initiates the OAuth2 authentication flow to authorize
CloudPull to access your Google Drive files.`,
	Example: `  # Start authentication
  cloudpull auth
  
  # Re-authenticate (replace existing credentials)
  cloudpull auth --force`,
	RunE: runAuth,
}

var (
	forceAuth  bool
	revokeAuth bool
)

func init() {
	authCmd.Flags().BoolVar(&forceAuth, "force", false,
		"Force re-authentication even if already authenticated")
	authCmd.Flags().BoolVar(&revokeAuth, "revoke", false,
		"Revoke current authentication")
}

func runAuth(cmd *cobra.Command, args []string) error {
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

	ctx := context.Background()

	// Handle revoke
	if revokeAuth {
		fmt.Println(color.YellowString("🔐 Revoking authentication..."))
		if err := application.RevokeAuth(ctx); err != nil {
			return fmt.Errorf("failed to revoke authentication: %w", err)
		}
		fmt.Println(color.GreenString("✅ Authentication revoked successfully!"))
		return nil
	}

	// Check current auth status
	if application.IsAuthenticated() {
		if !forceAuth {
			fmt.Println(color.GreenString("✅ Already authenticated!"))
			fmt.Println()
			fmt.Println("You can start syncing with 'cloudpull sync'")
			fmt.Println("Use --force to re-authenticate")
			return nil
		}
		fmt.Println(color.YellowString("⚠️  Force re-authentication requested"))
	}

	// Perform authentication
	fmt.Println(color.CyanString("🔐 CloudPull Authentication"))
	fmt.Println()
	fmt.Println("Starting OAuth2 authentication flow...")
	fmt.Println()

	if err := application.Authenticate(ctx); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Println()
	fmt.Println(color.GreenString("✅ Authentication successful!"))
	fmt.Println()
	fmt.Println("CloudPull is now authorized to access your Google Drive.")
	fmt.Println("You can start syncing with 'cloudpull sync'")

	return nil
}
