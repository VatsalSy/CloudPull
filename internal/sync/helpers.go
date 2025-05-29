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
  "database/sql"
  "time"
  
  "github.com/cloudpull/cloudpull/internal/state"
)

// NewNullString creates a valid sql.NullString
func NewNullString(s string) sql.NullString {
  return sql.NullString{
    String: s,
    Valid:  s != "",
  }
}

// NewNullTime creates a valid sql.NullTime
func NewNullTime(t time.Time) sql.NullTime {
  return sql.NullTime{
    Time:  t,
    Valid: !t.IsZero(),
  }
}

// Extension method for state package compatibility
func init() {
  // Register helper functions in state package
  state.NewNullString = NewNullString
  state.NewNullTime = NewNullTime
}