package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	// Default token file permissions (owner read/write only).
	tokenFilePerms = 0600

	// Token refresh buffer (refresh 5 minutes before expiry).
	tokenRefreshBuffer = 5 * time.Minute

	// HTTP client timeout for all requests.
	httpTimeout = 30 * time.Second
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
	// Validate logger is not nil to prevent runtime panics
	if logger == nil {
		return nil, errors.NewSimple("logger cannot be nil")
	}

	credBytes, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read credentials file")
	}

	config, err := google.ConfigFromJSON(credBytes, drive.DriveReadonlyScope)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse credentials")
	}

	// Extract redirect URL from credentials file
	redirectURL, err := extractRedirectURL(credBytes)
	if err != nil {
		// Fall back to default if extraction fails
		logger.Warn("Failed to extract redirect URL from credentials, using default", "error", err)
		redirectURL = "http://localhost"
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
	// Create HTTP client with consistent timeout
	httpClient := am.config.Client(ctx, token)
	httpClient.Timeout = httpTimeout
	am.httpClient = httpClient
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

	// No valid token found, authentication needs to be handled by caller
	return nil, errors.Wrap(err, "authentication required")
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

// GetAuthURL generates the OAuth2 authorization URL for user authentication.
func (am *AuthManager) GetAuthURL() string {
	return am.config.AuthCodeURL("state", oauth2.AccessTypeOffline)
}

// ExchangeAuthCode exchanges an authorization code for an OAuth2 token.
func (am *AuthManager) ExchangeAuthCode(ctx context.Context, authCode string) (*oauth2.Token, error) {
	if authCode == "" {
		return nil, errors.NewSimple("authorization code cannot be empty")
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

	// Update in-memory token and HTTP client
	am.token = token
	httpClient := am.config.Client(ctx, token)
	httpClient.Timeout = httpTimeout
	am.httpClient = httpClient

	am.logger.Info("Authentication successful")
	return token, nil
}

// extractRedirectURL extracts the first redirect URL from Google credentials JSON.
func extractRedirectURL(credBytes []byte) (string, error) {
	// Define structure for parsing credentials JSON
	var creds struct {
		Installed struct {
			RedirectURIs []string `json:"redirect_uris"`
		} `json:"installed"`
		Web struct {
			RedirectURIs []string `json:"redirect_uris"`
		} `json:"web"`
	}

	if err := json.Unmarshal(credBytes, &creds); err != nil {
		return "", errors.Wrap(err, "failed to parse credentials JSON")
	}

	// Check installed app credentials first (most common for CLI tools)
	if len(creds.Installed.RedirectURIs) > 0 {
		return creds.Installed.RedirectURIs[0], nil
	}

	// Check web app credentials as fallback
	if len(creds.Web.RedirectURIs) > 0 {
		return creds.Web.RedirectURIs[0], nil
	}

	return "", errors.NewSimple("no redirect URIs found in credentials")
}

// RevokeToken revokes the current token.
func (am *AuthManager) RevokeToken(ctx context.Context) error {
	token, err := am.loadToken()
	if err != nil {
		return errors.Wrap(err, "no token to revoke")
	}

	// Create HTTP client with timeout for revocation requests
	httpClient := &http.Client{
		Timeout: httpTimeout,
	}

	// Revoke access token
	if token.AccessToken != "" {
		revokeURL := fmt.Sprintf("https://oauth2.googleapis.com/revoke?token=%s", token.AccessToken)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, revokeURL, nil)
		if err != nil {
			am.logger.Warn("Failed to create access token revocation request", "error", err)
		} else {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			resp, err := httpClient.Do(req)
			if err != nil {
				am.logger.Warn("Failed to revoke access token", "error", err)
			} else {
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					// Read response body for error details
					bodyBytes, _ := io.ReadAll(resp.Body)
					am.logger.Warn("Failed to revoke access token", "status", resp.StatusCode, "response", string(bodyBytes))
				}
			}
		}
	}

	// Revoke refresh token
	if token.RefreshToken != "" {
		revokeURL := fmt.Sprintf("https://oauth2.googleapis.com/revoke?token=%s", token.RefreshToken)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, revokeURL, nil)
		if err != nil {
			am.logger.Warn("Failed to create refresh token revocation request", "error", err)
		} else {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			resp, err := httpClient.Do(req)
			if err != nil {
				am.logger.Warn("Failed to revoke refresh token", "error", err)
			} else {
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					// Read response body for error details
					bodyBytes, _ := io.ReadAll(resp.Body)
					am.logger.Warn("Failed to revoke refresh token", "status", resp.StatusCode, "response", string(bodyBytes))
				}
			}
		}
	}

	// Overwrite token file with empty token to prevent race conditions
	emptyToken := &oauth2.Token{
		AccessToken:  "",
		RefreshToken: "",
		TokenType:    "",
		Expiry:       time.Time{},
	}

	if err := am.saveToken(emptyToken); err != nil {
		return errors.Wrap(err, "failed to overwrite token file")
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

// GetRedirectURL returns the configured redirect URL.
func (am *AuthManager) GetRedirectURL() string {
	return am.config.RedirectURL
}
