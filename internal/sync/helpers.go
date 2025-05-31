/**
 * Helper Functions for CloudPull Sync Engine
 *
 * Features:
 * - Common utility functions
 * - ID generation
 *
 * Author: CloudPull Team
 * Updated: 2025-01-30
 */

package sync

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// generateID generates a unique session ID.
func generateID() string {
	timestamp := time.Now().Format("20060102_150405")
	randomBytes := make([]byte, 4)
	_, err := rand.Read(randomBytes)
	if err != nil {
		// Fallback to timestamp with nanoseconds for uniqueness
		return fmt.Sprintf("session_%s_%d", timestamp, time.Now().UnixNano())
	}
	randomHex := hex.EncodeToString(randomBytes)
	return fmt.Sprintf("session_%s_%s", timestamp, randomHex)
}

// formatBytes converts bytes to human-readable format.
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
