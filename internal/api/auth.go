package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/VatsalSy/CloudPull/internal/errors"
	"github.com/VatsalSy/CloudPull/internal/logger"
)

/**
 * OAuth2 Authentication Manager for Google Drive API
 *
 * Features:
 * - OAuth2 flow with automatic token refresh
 * - Secure token storage with file permissions
 * - Browser-based authentication flow
 * - Token validation and expiry handling
 *
 * Author: CloudPull Team
 * Updated: 2025-01-29
 */

const (
	// OAuth2 redirect URL for local authentication
	redirectURL = "http://localhost"

	// Default token file permissions (owner read/write only)
	tokenFilePerms = 0600

	// Token refresh buffer (refresh 5 minutes before expiry)
	tokenRefreshBuffer = 5 * time.Minute
)

// AuthManager handles OAuth2 authentication for Google Drive.
type AuthManager struct {
	config     *oauth2.Config
	httpClient *http.Client
	token      *oauth2.Token
	logger     *logger.Logger
	tokenPath  string
}

// NewAuthManager creates a new authentication manager.
func NewAuthManager(credentialsPath, tokenPath string, logger *logger.Logger) (*AuthManager, error) {
	credBytes, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read credentials file")
	}

	config, err := google.ConfigFromJSON(credBytes, drive.DriveReadonlyScope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse credentials")
	}

	config.RedirectURL = redirectURL

	return &AuthManager{
		config:    config,
		tokenPath: tokenPath,
		logger:    logger,
	}, nil
}

// GetClient returns an authenticated HTTP client for Google Drive API.
func (am *AuthManager) GetClient(ctx context.Context) (*http.Client, error) {
	token, err := am.getToken(ctx)
	if err != nil {
		return nil, err
	}

	// Check if token needs refresh
	if am.shouldRefreshToken(token) {
		newToken, err := am.refreshToken(ctx, token)
		if err != nil {
			am.logger.Warn("Failed to refresh token, using existing", "error", err)
		} else {
			token = newToken
			if err := am.saveToken(token); err != nil {
				am.logger.Warn("Failed to save refreshed token", "error", err)
			}
		}
	}

	am.token = token
	am.httpClient = am.config.Client(ctx, token)
	return am.httpClient, nil
}

// GetDriveService returns an authenticated Drive service.
func (am *AuthManager) GetDriveService(ctx context.Context) (*drive.Service, error) {
	client, err := am.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	service, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create drive service")
	}

	return service, nil
}

// getToken retrieves token from file or initiates OAuth flow.
func (am *AuthManager) getToken(ctx context.Context) (*oauth2.Token, error) {
	// Try to load existing token
	token, err := am.loadToken()
	if err == nil {
		return token, nil
	}

	// No valid token found, start OAuth flow
	am.logger.Info("No valid token found, starting authentication flow")
	return am.authenticate(ctx)
}

// loadToken loads token from file.
func (am *AuthManager) loadToken() (*oauth2.Token, error) {
	tokenBytes, err := os.ReadFile(am.tokenPath)
	if err != nil {
		return nil, err
	}

	var token oauth2.Token
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return nil, errors.Wrap(err, "failed to parse token")
	}

	// Validate token has required fields
	if token.AccessToken == "" && token.RefreshToken == "" {
		return nil, errors.NewSimple("invalid token: missing access and refresh tokens")
	}

	return &token, nil
}

// saveToken saves token to file with secure permissions.
func (am *AuthManager) saveToken(token *oauth2.Token) error {
	// Ensure directory exists
	tokenDir := filepath.Dir(am.tokenPath)
	if err := os.MkdirAll(tokenDir, 0700); err != nil {
		return errors.Wrap(err, "failed to create token directory")
	}

	tokenBytes, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal token")
	}

	if err := os.WriteFile(am.tokenPath, tokenBytes, tokenFilePerms); err != nil {
		return errors.Wrap(err, "failed to write token file")
	}

	am.logger.Debug("Token saved successfully", "path", am.tokenPath)
	return nil
}

// shouldRefreshToken checks if token should be refreshed.
func (am *AuthManager) shouldRefreshToken(token *oauth2.Token) bool {
	if token.RefreshToken == "" {
		return false
	}

	if token.Expiry.IsZero() {
		return false
	}

	// Refresh if token expires within buffer time
	return time.Until(token.Expiry) < tokenRefreshBuffer
}

// refreshToken refreshes the OAuth2 token.
func (am *AuthManager) refreshToken(ctx context.Context, token *oauth2.Token) (*oauth2.Token, error) {
	tokenSource := am.config.TokenSource(ctx, token)
	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, errors.Wrap(err, "failed to refresh token")
	}

	am.logger.Info("Token refreshed successfully")
	return newToken, nil
}

// authenticate performs OAuth2 authentication flow.
func (am *AuthManager) authenticate(ctx context.Context) (*oauth2.Token, error) {
	// Generate auth URL
	authURL := am.config.AuthCodeURL("state", oauth2.AccessTypeOffline)

	fmt.Printf("\nTo authenticate, please visit:\n%s\n\n", authURL)
	fmt.Println("After clicking 'Allow', you'll be redirected to a URL starting with:")
	fmt.Println("http://localhost/?code=...")
	fmt.Println("")
	fmt.Println("If you see a browser error (This site can't be reached), that's normal!")
	fmt.Println("Look at the URL bar and copy the authorization code.")
	fmt.Println("The code is the value after 'code=' and before any '&' character.")
	fmt.Println("")
	fmt.Println("Example: If the URL is:")
	fmt.Println("http://localhost/?code=4/0AY0e-g7ABC123&scope=...")
	fmt.Println("Then copy: 4/0AY0e-g7ABC123")
	fmt.Print("\nEnter authorization code: ")

	var authCode string
	if _, err := fmt.Scanln(&authCode); err != nil {
		return nil, errors.Wrap(err, "failed to read authorization code")
	}

	// Exchange code for token
	token, err := am.config.Exchange(ctx, authCode)
	if err != nil {
		return nil, errors.Wrap(err, "failed to exchange authorization code")
	}

	// Save token for future use
	if err := am.saveToken(token); err != nil {
		am.logger.Warn("Failed to save token", "error", err)
	}

	am.logger.Info("Authentication successful")
	return token, nil
}

// RevokeToken revokes the current token.
func (am *AuthManager) RevokeToken(ctx context.Context) error {
	token, err := am.loadToken()
	if err != nil {
		return errors.Wrap(err, "no token to revoke")
	}

	// Revoke token via Google API
	revokeURL := fmt.Sprintf("https://oauth2.googleapis.com/revoke?token=%s", token.AccessToken)
	resp, err := http.Post(revokeURL, "application/x-www-form-urlencoded", nil)
	if err != nil {
		return errors.Wrap(err, "failed to revoke token")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("failed to revoke token: status %d", resp.StatusCode)
	}

	// Remove token file
	if err := os.Remove(am.tokenPath); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "failed to remove token file")
	}

	am.logger.Info("Token revoked successfully")
	return nil
}

// IsAuthenticated checks if valid authentication exists.
func (am *AuthManager) IsAuthenticated() bool {
	token, err := am.loadToken()
	if err != nil {
		return false
	}

	// Check if token is still valid
	if token.Expiry.IsZero() {
		return token.AccessToken != ""
	}

	return token.Expiry.After(time.Now())
}
