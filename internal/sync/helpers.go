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
