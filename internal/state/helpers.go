// Helper Functions for CloudPull State Package
//
// Features:
// - Null type helpers for database operations
//
// Author: CloudPull Team
// Updated: 2025-01-30

package state

import (
	"database/sql"
	"time"
)

// NewNullString creates a valid sql.NullString.
func NewNullString(s string) sql.NullString {
	return sql.NullString{
		String: s,
		Valid:  s != "",
	}
}

// NewNullStringAllowEmpty creates a sql.NullString that treats empty strings as valid.
func NewNullStringAllowEmpty(s string) sql.NullString {
	return sql.NullString{
		String: s,
		Valid:  true,
	}
}

// NewNullTime creates a valid sql.NullTime.
func NewNullTime(t time.Time) sql.NullTime {
	return sql.NullTime{
		Time:  t,
		Valid: !t.IsZero(),
	}
}
