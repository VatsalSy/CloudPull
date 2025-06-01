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

	if err := application.InitializeForAuth(); err != nil {
		return fmt.Errorf("failed to initialize application: %w", err)
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

	// Get auth URL
	authURL, err := application.GetAuthURL()
	if err != nil {
		return fmt.Errorf("failed to get auth URL: %w", err)
	}

	// Get redirect URL
	redirectURL, err := application.GetRedirectURL()
	if err != nil {
		return fmt.Errorf("failed to get redirect URL: %w", err)
	}

	// Display instructions
	fmt.Printf("\nTo authenticate, please visit:\n%s\n\n", authURL)
	fmt.Println("After clicking 'Allow', you'll be redirected to a URL starting with:")
	fmt.Printf("%s/?code=...\n", redirectURL)
	fmt.Println("")
	fmt.Println("If you see a browser error (This site can't be reached), that's normal!")
	fmt.Println("Look at the URL bar and copy the authorization code.")
	fmt.Println("The code is the value after 'code=' and before any '&' character.")
	fmt.Println("")
	fmt.Println("Example: If the URL is:")
	fmt.Printf("%s/?code=4/0AY0e-g7ABC123&scope=...\n", redirectURL)
	fmt.Println("Then copy: 4/0AY0e-g7ABC123")
	fmt.Print("\nEnter authorization code: ")

	// Read auth code from user
	var authCode string
	if _, err := fmt.Scanln(&authCode); err != nil {
		return fmt.Errorf("failed to read authorization code: %w", err)
	}

	// Exchange auth code for token
	if err := application.ExchangeAuthCode(ctx, authCode); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Println()
	fmt.Println(color.GreenString("✅ Authentication successful!"))
	fmt.Println()
	fmt.Println("CloudPull is now authorized to access your Google Drive.")
	fmt.Println("You can start syncing with 'cloudpull sync'")

	return nil
}
