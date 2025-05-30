/**
 * Logger Tests
 *
 * Unit tests for the zerolog-based logger implementation to ensure
 * proper logging functionality and configuration.
 *
 * Author: CloudPull Team
 * Created: 2025-01-29
 */

package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test logger creation and configuration.
func TestLoggerCreation(t *testing.T) {
	t.Run("DefaultConfig", func(t *testing.T) {
		log := New(nil)
		assert.NotNil(t, log)
		assert.NotNil(t, log.config)
	})

	t.Run("CustomConfig", func(t *testing.T) {
		buf := &bytes.Buffer{}
		config := &Config{
			Level:         "debug",
			Output:        buf,
			Pretty:        false,
			IncludeCaller: false,
			Fields: map[string]interface{}{
				"app": "cloudpull",
				"env": "test",
			},
		}

		log := New(config)
		assert.NotNil(t, log)

		// Test logging
		log.Info("test message")

		// Parse output
		var output map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &output)
		require.NoError(t, err)

		assert.Equal(t, "info", output["level"])
		assert.Equal(t, "test message", output["message"])
		assert.Equal(t, "cloudpull", output["app"])
		assert.Equal(t, "test", output["env"])
	})
}

// Test logging methods.
func TestLoggingMethods(t *testing.T) {
	testCases := []struct {
		name    string
		logFunc func(*Logger, string, ...interface{})
		level   string
	}{
		{"Debug", func(l *Logger, msg string, fields ...interface{}) { l.Debug(msg, fields...) }, "debug"},
		{"Info", func(l *Logger, msg string, fields ...interface{}) { l.Info(msg, fields...) }, "info"},
		{"Warn", func(l *Logger, msg string, fields ...interface{}) { l.Warn(msg, fields...) }, "warn"},
		{"Trace", func(l *Logger, msg string, fields ...interface{}) { l.Trace(msg, fields...) }, "trace"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			log := New(&Config{
				Level:  "trace",
				Output: buf,
			})

			tc.logFunc(log, "test message", "key1", "value1", "key2", 123)

			var output map[string]interface{}
			err := json.Unmarshal(buf.Bytes(), &output)
			require.NoError(t, err)

			assert.Equal(t, tc.level, output["level"])
			assert.Equal(t, "test message", output["message"])
			assert.Equal(t, "value1", output["key1"])
			assert.Equal(t, float64(123), output["key2"])
		})
	}
}

// Test error logging.
func TestErrorLogging(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(&Config{
		Level:  "debug",
		Output: buf,
	})

	testErr := errors.New("test error")
	log.Error(testErr, "error occurred", "operation", "test_op")

	var output map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &output)
	require.NoError(t, err)

	assert.Equal(t, "error", output["level"])
	assert.Equal(t, "error occurred", output["message"])
	assert.Equal(t, "test error", output["error"])
	assert.Equal(t, "test_op", output["operation"])
}

// Test context integration.
func TestContextIntegration(t *testing.T) {
	log := New(&Config{Level: "debug"})

	// Add logger to context
	ctx := log.WithContext(context.Background())

	// Retrieve logger from context
	retrieved := FromContext(ctx)
	assert.NotNil(t, retrieved)

	// Test with empty context
	emptyLog := FromContext(context.Background())
	assert.NotNil(t, emptyLog)
}

// Test child loggers with fields.
func TestChildLoggers(t *testing.T) {
	buf := &bytes.Buffer{}
	parent := New(&Config{
		Level:  "debug",
		Output: buf,
	})

	t.Run("WithField", func(t *testing.T) {
		buf.Reset()
		child := parent.WithField("request_id", "12345")
		child.Info("child log")

		var output map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &output)
		require.NoError(t, err)

		assert.Equal(t, "12345", output["request_id"])
	})

	t.Run("With", func(t *testing.T) {
		buf.Reset()
		child := parent.With(map[string]interface{}{
			"user_id": "user123",
			"action":  "upload",
		})
		child.Info("child log")

		var output map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &output)
		require.NoError(t, err)

		assert.Equal(t, "user123", output["user_id"])
		assert.Equal(t, "upload", output["action"])
	})
}

// Test structured error logging.
func TestStructuredError(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(&Config{
		Level:  "debug",
		Output: buf,
	})

	testErr := errors.New("structured error")
	fields := map[string]interface{}{
		"file":  "/path/to/file",
		"size":  1024,
		"retry": 3,
	}

	log.StructuredError(testErr, fields)

	var output map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &output)
	require.NoError(t, err)

	assert.Equal(t, "error", output["level"])
	assert.Equal(t, "structured error", output["error"])
	assert.Equal(t, "/path/to/file", output["file"])
	assert.Equal(t, float64(1024), output["size"])
	assert.Equal(t, float64(3), output["retry"])
	assert.Contains(t, output["error_type"].(string), "errors.errorString")
}

// Test log operation.
func TestLogOperation(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(&Config{
		Level:  "debug",
		Output: buf,
	})

	t.Run("Success", func(t *testing.T) {
		buf.Reset()
		err := log.LogOperation("test_operation", func() error {
			time.Sleep(10 * time.Millisecond)
			return nil
		})

		assert.NoError(t, err)

		logs := strings.Split(strings.TrimSpace(buf.String()), "\n")
		assert.Equal(t, 2, len(logs))

		// Check start log
		var startLog map[string]interface{}
		json.Unmarshal([]byte(logs[0]), &startLog)
		assert.Equal(t, "Operation started", startLog["message"])
		assert.Equal(t, "test_operation", startLog["operation"])

		// Check completion log
		var endLog map[string]interface{}
		json.Unmarshal([]byte(logs[1]), &endLog)
		assert.Equal(t, "Operation completed", endLog["message"])
		assert.Equal(t, "test_operation", endLog["operation"])
		assert.NotNil(t, endLog["duration"])
	})

	t.Run("Failure", func(t *testing.T) {
		buf.Reset()
		testErr := errors.New("operation failed")
		err := log.LogOperation("failing_operation", func() error {
			return testErr
		})

		assert.Equal(t, testErr, err)

		logs := strings.Split(strings.TrimSpace(buf.String()), "\n")
		assert.Equal(t, 2, len(logs))

		// Check failure log
		var failLog map[string]interface{}
		json.Unmarshal([]byte(logs[1]), &failLog)
		assert.Equal(t, "error", failLog["level"])
		assert.Equal(t, "Operation failed", failLog["message"])
		assert.Equal(t, "operation failed", failLog["error"])
	})
}

// Test log request.
func TestLogRequest(t *testing.T) {
	testCases := []struct {
		name       string
		level      string
		statusCode int
	}{
		{name: "Success", level: "info", statusCode: 200},
		{name: "ClientError", level: "error", statusCode: 400},
		{name: "ServerError", level: "error", statusCode: 500},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			log := New(&Config{
				Level:  "debug",
				Output: buf,
			})

			log.LogRequest("GET", "/api/files", tc.statusCode, 100*time.Millisecond)

			var output map[string]interface{}
			err := json.Unmarshal(buf.Bytes(), &output)
			require.NoError(t, err)

			assert.Equal(t, tc.level, output["level"])
			assert.Equal(t, "GET", output["method"])
			assert.Equal(t, "/api/files", output["path"])
			assert.Equal(t, float64(tc.statusCode), output["status"])
			assert.Equal(t, float64(100), output["duration"])
		})
	}
}

// Test level changes.
func TestSetLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(&Config{
		Level:  "info",
		Output: buf,
	})

	// Debug should not log at info level
	log.Debug("debug message")
	assert.Empty(t, buf.String())

	// Change to debug level
	err := log.SetLevel("debug")
	assert.NoError(t, err)

	// Now debug should log
	buf.Reset()
	log.Debug("debug message")
	assert.NotEmpty(t, buf.String())

	// Test invalid level
	err = log.SetLevel("invalid")
	assert.Error(t, err)
}

// Test global logger.
func TestGlobalLogger(t *testing.T) {
	buf := &bytes.Buffer{}
	Init(&Config{
		Level:  "debug",
		Output: buf,
	})

	// Test global functions
	Info("global info")

	var output map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &output)
	require.NoError(t, err)

	assert.Equal(t, "info", output["level"])
	assert.Equal(t, "global info", output["message"])

	// Test other global functions
	buf.Reset()
	Debug("debug message", "key", "value")
	assert.NotEmpty(t, buf.String())

	buf.Reset()
	Warn("warning message")
	assert.NotEmpty(t, buf.String())

	buf.Reset()
	Error(errors.New("test error"), "error message")
	assert.NotEmpty(t, buf.String())
}

// Test development and production configs.
func TestPredefinedConfigs(t *testing.T) {
	t.Run("Development", func(t *testing.T) {
		config := NewDevelopmentConfig()
		assert.Equal(t, "debug", config.Level)
		assert.True(t, config.Pretty)
		assert.True(t, config.IncludeCaller)
		assert.Equal(t, "development", config.Fields["env"])
	})

	t.Run("Production", func(t *testing.T) {
		config := NewProductionConfig()
		assert.Equal(t, "info", config.Level)
		assert.False(t, config.Pretty)
		assert.False(t, config.IncludeCaller)
		assert.Equal(t, "production", config.Fields["env"])
	})
}

// Test file writer.
func TestFileWriter(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	t.Run("BasicWrite", func(t *testing.T) {
		fw, err := NewFileWriter(logFile, 1024*1024, 3)
		require.NoError(t, err)
		defer fw.Close()

		data := []byte("test log entry\n")
		n, err := fw.Write(data)
		assert.NoError(t, err)
		assert.Equal(t, len(data), n)

		// Verify file contents
		contents, err := os.ReadFile(logFile)
		require.NoError(t, err)
		assert.Equal(t, string(data), string(contents))
	})

	t.Run("Rotation", func(t *testing.T) {
		// Create writer with small max size
		rotateFile := filepath.Join(tempDir, "rotate.log")
		fw, err := NewFileWriter(rotateFile, 50, 2)
		require.NoError(t, err)
		defer fw.Close()

		// Write data to trigger rotation
		data1 := []byte("First line of log data that is long\n")
		fw.Write(data1)

		data2 := []byte("Second line that triggers rotation\n")
		fw.Write(data2)

		// Check that rotation occurred
		_, err = os.Stat(rotateFile + ".1")
		assert.NoError(t, err)
	})
}

// Test caller information.
func TestGetCaller(t *testing.T) {
	file, line, function := GetCaller(0)

	assert.Contains(t, file, "logger_test.go")
	assert.Greater(t, line, 0)
	assert.Contains(t, function, "TestGetCaller")
}

// Test pretty printing.
func TestPrettyLogging(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(&Config{
		Level:  "info",
		Output: buf,
		Pretty: true,
	})

	log.Info("pretty message", "key", "value")

	output := buf.String()
	assert.Contains(t, output, "INF")
	assert.Contains(t, output, "pretty message")
	assert.Contains(t, output, "key=value")
}
