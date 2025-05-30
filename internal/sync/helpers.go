/**
 * Helper Functions for CloudPull Sync Engine
 *
 * Features:
 * - Common utility functions
 * - ID generation
 * - Null type helpers
 *
 * Author: CloudPull Team
 * Updated: 2025-01-29
 */

package sync

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/VatsalSy/CloudPull/internal/state"
)

// NewNullString creates a valid sql.NullString.
func NewNullString(s string) sql.NullString {
	return sql.NullString{
		String: s,
		Valid:  s != "",
	}
}

// NewNullTime creates a valid sql.NullTime.
func NewNullTime(t time.Time) sql.NullTime {
	return sql.NullTime{
		Time:  t,
		Valid: !t.IsZero(),
	}
}

// generateID generates a unique session ID.
func generateID() string {
	timestamp := time.Now().Format("20060102_150405")
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	randomHex := hex.EncodeToString(randomBytes)
	return fmt.Sprintf("session_%s_%s", timestamp, randomHex)
}

// Extension method for state package compatibility.
func init() {
	// Register helper functions in state package
	state.NewNullString = NewNullString
	state.NewNullTime = NewNullTime
}
