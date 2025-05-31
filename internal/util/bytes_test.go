package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		expected string
		bytes    int64
	}{
		{expected: "0 B", bytes: 0},
		{expected: "100 B", bytes: 100},
		{expected: "1.0 KB", bytes: 1024},
		{expected: "1.5 KB", bytes: 1536},
		{expected: "1.0 MB", bytes: 1048576},
		{expected: "1.0 GB", bytes: 1073741824},
		{expected: "1.0 TB", bytes: 1099511627776},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := FormatBytes(tt.bytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}