/**
 * Logger Implementation for CloudPull
 *
 * Structured logging using zerolog with context awareness, error tracking,
 * and configurable output formats for development and production environments.
 *
 * Author: CloudPull Team
 * Created: 2025-01-29
 */

package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Logger wraps zerolog with additional functionality.
type Logger struct {
	logger zerolog.Logger
	config *Config
}

// Config configures the logger behavior.
type Config struct {
	Output        io.Writer
	Fields        map[string]interface{}
	Level         string
	TimeFormat    string
	Pretty        bool
	IncludeCaller bool
}

// DefaultConfig provides sensible defaults.
var DefaultConfig = &Config{
	Level:         "info",
	Output:        os.Stdout,
	Pretty:        false,
	IncludeCaller: true,
	Fields:        make(map[string]interface{}),
	TimeFormat:    time.RFC3339,
}

// contextKey is used for storing logger in context.
type contextKey struct{}

// loggerKey is the context key for logger.
var loggerKey = contextKey{}

// New creates a new logger instance.
func New(config *Config) *Logger {
	if config == nil {
		config = DefaultConfig
	}

	// Set global time format
	zerolog.TimeFieldFormat = config.TimeFormat

	// Configure output
	var output = config.Output
	if config.Pretty {
		output = zerolog.ConsoleWriter{
			Out:        config.Output,
			TimeFormat: config.TimeFormat,
			NoColor:    false,
		}
	}

	// Parse log level
	level, err := zerolog.ParseLevel(config.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}

	// Create base logger
	logger := zerolog.New(output).
		Level(level).
		With().
		Timestamp().
		Logger()

	// Add default fields
	for k, v := range config.Fields {
		logger = logger.With().Interface(k, v).Logger()
	}

	// Add caller information if requested
	if config.IncludeCaller {
		logger = logger.With().CallerWithSkipFrameCount(3).Logger()
	}

	return &Logger{
		logger: logger,
		config: config,
	}
}

// WithContext adds the logger to context.
func (l *Logger) WithContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// FromContext retrieves logger from context.
func FromContext(ctx context.Context) *Logger {
	if l, ok := ctx.Value(loggerKey).(*Logger); ok {
		return l
	}
	return New(DefaultConfig)
}

// With creates a child logger with additional fields.
func (l *Logger) With(fields ...interface{}) *Logger {
	newLogger := l.logger.With()

	// Process fields as key-value pairs
	for i := 0; i < len(fields)-1; i += 2 {
		if key, ok := fields[i].(string); ok {
			newLogger = newLogger.Interface(key, fields[i+1])
		}
	}

	return &Logger{
		logger: newLogger.Logger(),
		config: l.config,
	}
}

// WithField creates a child logger with an additional field.
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return &Logger{
		logger: l.logger.With().Interface(key, value).Logger(),
		config: l.config,
	}
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, fields ...interface{}) {
	event := l.logger.Debug()
	l.logEvent(event, msg, fields...)
}

// Info logs an info message.
func (l *Logger) Info(msg string, fields ...interface{}) {
	event := l.logger.Info()
	l.logEvent(event, msg, fields...)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, fields ...interface{}) {
	event := l.logger.Warn()
	l.logEvent(event, msg, fields...)
}

// Error logs an error message.
func (l *Logger) Error(err error, msg string, fields ...interface{}) {
	event := l.logger.Error()
	if err != nil {
		event = event.Err(err)
	}
	l.logEvent(event, msg, fields...)
}

// Fatal logs a fatal message and exits.
func (l *Logger) Fatal(err error, msg string, fields ...interface{}) {
	event := l.logger.Fatal()
	if err != nil {
		event = event.Err(err)
	}
	l.logEvent(event, msg, fields...)
}

// logEvent processes field pairs and sends the log event.
func (l *Logger) logEvent(event *zerolog.Event, msg string, fields ...interface{}) {
	// Process field pairs
	for i := 0; i < len(fields)-1; i += 2 {
		key, ok := fields[i].(string)
		if !ok {
			continue
		}
		event = event.Interface(key, fields[i+1])
	}

	event.Msg(msg)
}

// Trace logs a trace message for detailed debugging.
func (l *Logger) Trace(msg string, fields ...interface{}) {
	event := l.logger.Trace()
	l.logEvent(event, msg, fields...)
}

// LogOperation logs the start and end of an operation.
func (l *Logger) LogOperation(op string, fn func() error) error {
	start := time.Now()
	l.Info("Operation started", "operation", op)

	err := fn()

	duration := time.Since(start)
	if err != nil {
		l.Error(err, "Operation failed",
			"operation", op,
			"duration", duration,
		)
	} else {
		l.Info("Operation completed",
			"operation", op,
			"duration", duration,
		)
	}

	return err
}

// LogRequest logs HTTP-style requests.
func (l *Logger) LogRequest(method, path string, statusCode int, duration time.Duration) {
	event := l.logger.Info()
	if statusCode >= 400 {
		event = l.logger.Error()
	}

	event.
		Str("method", method).
		Str("path", path).
		Int("status", statusCode).
		Dur("duration", duration).
		Msg("Request completed")
}

// StructuredError creates a structured error log entry.
func (l *Logger) StructuredError(err error, fields map[string]interface{}) {
	event := l.logger.Error().Err(err)

	for k, v := range fields {
		event = event.Interface(k, v)
	}

	// Add error context
	event.
		Str("error_type", fmt.Sprintf("%T", err)).
		Msg("Structured error occurred")
}

// SetLevel changes the logger level dynamically.
func (l *Logger) SetLevel(level string) error {
	parsedLevel, err := zerolog.ParseLevel(level)
	if err != nil {
		return err
	}

	l.logger = l.logger.Level(parsedLevel)
	return nil
}

// Global logger instance.
var global *Logger

// Init initializes the global logger.
func Init(config *Config) {
	global = New(config)

	// Set zerolog global logger
	log.Logger = global.logger
}

// Global returns the global logger instance.
func Global() *Logger {
	if global == nil {
		Init(DefaultConfig)
	}
	return global
}

// Helper functions for global logger

// Debug logs a debug message using global logger.
func Debug(msg string, fields ...interface{}) {
	Global().Debug(msg, fields...)
}

// Info logs an info message using global logger.
func Info(msg string, fields ...interface{}) {
	Global().Info(msg, fields...)
}

// Warn logs a warning message using global logger.
func Warn(msg string, fields ...interface{}) {
	Global().Warn(msg, fields...)
}

// Error logs an error message using global logger.
func Error(err error, msg string, fields ...interface{}) {
	Global().Error(err, msg, fields...)
}

// Fatal logs a fatal message using global logger and exits.
func Fatal(err error, msg string, fields ...interface{}) {
	Global().Fatal(err, msg, fields...)
}

// WithField creates a child logger with a field using global logger.
func WithField(key string, value interface{}) *Logger {
	return Global().WithField(key, value)
}

// With creates a child logger with fields using global logger.
func With(fields map[string]interface{}) *Logger {
	return Global().With(fields)
}

// NewDevelopmentConfig creates a config suitable for development.
func NewDevelopmentConfig() *Config {
	return &Config{
		Level:         "debug",
		Output:        os.Stdout,
		Pretty:        true,
		IncludeCaller: true,
		Fields: map[string]interface{}{
			"env": "development",
		},
		TimeFormat: "15:04:05",
	}
}

// NewProductionConfig creates a config suitable for production.
func NewProductionConfig() *Config {
	return &Config{
		Level:         "info",
		Output:        os.Stdout,
		Pretty:        false,
		IncludeCaller: false,
		Fields: map[string]interface{}{
			"env": "production",
		},
		TimeFormat: time.RFC3339,
	}
}

// FileWriter creates a file writer with rotation support.
type FileWriter struct {
	file       *os.File
	filename   string
	maxSize    int64
	maxBackups int
}

// NewFileWriter creates a new file writer.
func NewFileWriter(filename string, maxSize int64, maxBackups int) (*FileWriter, error) {
	fw := &FileWriter{
		filename:   filename,
		maxSize:    maxSize,
		maxBackups: maxBackups,
	}

	if err := fw.openFile(); err != nil {
		return nil, err
	}

	return fw, nil
}

// Write implements io.Writer.
func (fw *FileWriter) Write(p []byte) (n int, err error) {
	// Check if rotation is needed
	if fw.file != nil {
		info, err := fw.file.Stat()
		if err == nil && info.Size()+int64(len(p)) > fw.maxSize {
			if err := fw.rotate(); err != nil {
				return 0, err
			}
		}
	}

	return fw.file.Write(p)
}

// Close closes the file writer.
func (fw *FileWriter) Close() error {
	if fw.file != nil {
		return fw.file.Close()
	}
	return nil
}

// openFile opens the log file.
func (fw *FileWriter) openFile() error {
	// Create directory if needed
	dir := filepath.Dir(fw.filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Open file
	file, err := os.OpenFile(fw.filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	fw.file = file
	return nil
}

// rotate performs log rotation.
func (fw *FileWriter) rotate() error {
	// Close current file
	if err := fw.file.Close(); err != nil {
		return err
	}

	// Rotate files
	for i := fw.maxBackups - 1; i > 0; i-- {
		oldName := fmt.Sprintf("%s.%d", fw.filename, i)
		newName := fmt.Sprintf("%s.%d", fw.filename, i+1)
		_ = os.Rename(oldName, newName)
	}

	// Rename current file
	if err := os.Rename(fw.filename, fw.filename+".1"); err != nil {
		return err
	}

	// Open new file
	return fw.openFile()
}

// GetCaller returns the caller information.
func GetCaller(skip int) (file string, line int, function string) {
	pc, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		return "unknown", 0, "unknown"
	}

	// Clean up file path
	if idx := strings.LastIndex(file, "/"); idx >= 0 {
		file = file[idx+1:]
	}

	// Get function name
	fn := runtime.FuncForPC(pc)
	if fn != nil {
		function = fn.Name()
		if idx := strings.LastIndex(function, "."); idx >= 0 {
			function = function[idx+1:]
		}
	}

	return file, line, function
}
