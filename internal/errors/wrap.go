/**
 * Error Wrapping Utilities for CloudPull
 *
 * Provides convenience functions for error wrapping and creation
 *
 * Author: CloudPull Team
 * Created: 2025-01-30
 */

package errors

import (
	"errors"
	"fmt"
)

// Wrap wraps an error with additional context.
func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

// Wrapf wraps an error with a formatted message.
func Wrapf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	message := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s: %w", message, err)
}

// NewSimple creates a simple error without the full Error struct.
func NewSimple(message string) error {
	return errors.New(message)
}

// Errorf creates a formatted error.
func Errorf(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}

// WrapTyped wraps an error with a specific error type.
func WrapTyped(errorType ErrorType, op string, err error) *Error {
	return New(errorType, op, "", err)
}

// IsTemporary checks if an error is temporary/retryable.
func IsTemporary(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's our Error type
	var e *Error
	if AsError(err, &e) {
		return e.IsRetryable()
	}

	// Default to false for unknown errors
	return false
}

// AsError checks if an error is of type *Error and assigns it.
func AsError(err error, target **Error) bool {
	if err == nil {
		return false
	}

	e, ok := err.(*Error)
	if ok {
		*target = e
		return true
	}

	// Check wrapped errors
	type unwrapper interface {
		Unwrap() error
	}

	if u, ok := err.(unwrapper); ok {
		return AsError(u.Unwrap(), target)
	}

	return false
}
