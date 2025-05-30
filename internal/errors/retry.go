/**
 * Retry Logic with Exponential Backoff
 *
 * Implements exponential backoff algorithm with jitter for retry operations,
 * preventing thundering herd problems and providing smooth retry behavior.
 *
 * Author: CloudPull Team
 * Created: 2025-01-29
 */

package errors

import (
	"context"
	"math"
	"math/rand"
	"time"
)

// BackoffConfig configures the exponential backoff behavior.
type BackoffConfig struct {
	// InitialInterval is the initial retry interval
	InitialInterval time.Duration

	// MaxInterval is the maximum retry interval
	MaxInterval time.Duration

	// Multiplier is the factor by which the retry interval increases
	Multiplier float64

	// MaxElapsedTime is the maximum total time for all retries
	MaxElapsedTime time.Duration

	// RandomizationFactor adds jitter to prevent thundering herd
	RandomizationFactor float64
}

// DefaultBackoffConfig provides sensible defaults for exponential backoff.
var DefaultBackoffConfig = &BackoffConfig{
	InitialInterval:     500 * time.Millisecond,
	MaxInterval:         60 * time.Second,
	Multiplier:          2.0,
	MaxElapsedTime:      15 * time.Minute,
	RandomizationFactor: 0.5,
}

// ExponentialBackoff implements exponential backoff with jitter.
type ExponentialBackoff struct {
	startTime       time.Time
	config          *BackoffConfig
	rand            *rand.Rand
	currentInterval time.Duration
	attempt         int
}

// NewExponentialBackoff creates a new exponential backoff instance.
func NewExponentialBackoff(config *BackoffConfig) *ExponentialBackoff {
	if config == nil {
		config = DefaultBackoffConfig
	}

	return &ExponentialBackoff{
		config:          config,
		currentInterval: config.InitialInterval,
		startTime:       time.Now(),
		rand:            rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Reset resets the backoff to initial state.
func (eb *ExponentialBackoff) Reset() {
	eb.currentInterval = eb.config.InitialInterval
	eb.startTime = time.Now()
	eb.attempt = 0
}

// NextBackOff returns the next backoff duration.
func (eb *ExponentialBackoff) NextBackOff() time.Duration {
	// Check if max elapsed time exceeded
	if eb.config.MaxElapsedTime > 0 {
		elapsed := time.Since(eb.startTime)
		if elapsed >= eb.config.MaxElapsedTime {
			return -1 // Indicates no more retries
		}
	}

	// Calculate next interval with jitter
	interval := eb.getNextInterval()

	// Update for next iteration
	eb.currentInterval = time.Duration(float64(eb.currentInterval) * eb.config.Multiplier)
	if eb.currentInterval > eb.config.MaxInterval {
		eb.currentInterval = eb.config.MaxInterval
	}

	eb.attempt++

	return interval
}

// GetAttempt returns the current attempt number.
func (eb *ExponentialBackoff) GetAttempt() int {
	return eb.attempt
}

// getNextInterval calculates the next interval with jitter.
func (eb *ExponentialBackoff) getNextInterval() time.Duration {
	if eb.config.RandomizationFactor == 0 {
		return eb.currentInterval
	}

	// Add jitter
	delta := eb.config.RandomizationFactor * float64(eb.currentInterval)
	minInterval := float64(eb.currentInterval) - delta
	maxInterval := float64(eb.currentInterval) + delta

	// Generate random interval with jitter
	interval := minInterval + (eb.rand.Float64() * (maxInterval - minInterval))

	return time.Duration(interval)
}

// calculateBackoff is a utility function for simple backoff calculation.
func calculateBackoff(
	attempt int,
	initialDelay time.Duration,
	maxDelay time.Duration,
	multiplier float64,
	jitter bool,
) time.Duration {

	if attempt <= 0 {
		return initialDelay
	}

	// Calculate exponential backoff
	backoff := float64(initialDelay) * math.Pow(multiplier, float64(attempt-1))

	// Cap at max delay
	if backoff > float64(maxDelay) {
		backoff = float64(maxDelay)
	}

	// Add jitter if requested
	if jitter {
		// Add up to 25% jitter
		jitterAmount := backoff * 0.25
		jitterValue := (rand.Float64()*2 - 1) * jitterAmount // -25% to +25%
		backoff += jitterValue
	}

	return time.Duration(backoff)
}

// RetryOperation executes an operation with exponential backoff retry.
func RetryOperation(
	ctx context.Context,
	operation func() error,
	config *BackoffConfig,
	shouldRetry func(error) bool,
) error {

	backoff := NewExponentialBackoff(config)

	var lastErr error

	for {
		// Execute operation
		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if we should retry
		if !shouldRetry(err) {
			return err
		}

		// Get next backoff duration
		interval := backoff.NextBackOff()
		if interval < 0 {
			return lastErr // Max elapsed time exceeded
		}

		// Wait with context awareness
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			// Continue to next attempt
		}
	}
}

// RetryWithBackoff is a simplified retry function with default backoff.
func RetryWithBackoff(
	ctx context.Context,
	maxAttempts int,
	operation func() error,
) error {

	config := &BackoffConfig{
		InitialInterval:     1 * time.Second,
		MaxInterval:         30 * time.Second,
		Multiplier:          2.0,
		RandomizationFactor: 0.5,
	}

	attempts := 0
	shouldRetry := func(err error) bool {
		attempts++
		return attempts < maxAttempts && err != nil
	}

	return RetryOperation(ctx, operation, config, shouldRetry)
}

// AdaptiveBackoff adjusts backoff based on error patterns.
type AdaptiveBackoff struct {
	baseBackoff    *ExponentialBackoff
	errorCounts    map[ErrorType]int
	adaptiveConfig *AdaptiveConfig
	successCount   int
}

// AdaptiveConfig configures adaptive backoff behavior.
type AdaptiveConfig struct {
	// SuccessThreshold reduces backoff after consecutive successes
	SuccessThreshold int

	// ErrorThreshold increases backoff after repeated errors
	ErrorThreshold int

	// AdaptationFactor controls how much to adjust intervals
	AdaptationFactor float64
}

// DefaultAdaptiveConfig provides default adaptive configuration.
var DefaultAdaptiveConfig = &AdaptiveConfig{
	SuccessThreshold: 3,
	ErrorThreshold:   5,
	AdaptationFactor: 0.5,
}

// NewAdaptiveBackoff creates an adaptive backoff instance.
func NewAdaptiveBackoff(
	backoffConfig *BackoffConfig,
	adaptiveConfig *AdaptiveConfig,
) *AdaptiveBackoff {

	if adaptiveConfig == nil {
		adaptiveConfig = DefaultAdaptiveConfig
	}

	return &AdaptiveBackoff{
		baseBackoff:    NewExponentialBackoff(backoffConfig),
		errorCounts:    make(map[ErrorType]int),
		adaptiveConfig: adaptiveConfig,
	}
}

// RecordSuccess records a successful operation.
func (ab *AdaptiveBackoff) RecordSuccess() {
	ab.successCount++

	// Reset error counts on success
	if ab.successCount >= ab.adaptiveConfig.SuccessThreshold {
		ab.errorCounts = make(map[ErrorType]int)

		// Reduce current interval
		newInterval := float64(ab.baseBackoff.currentInterval) *
			ab.adaptiveConfig.AdaptationFactor
		if newInterval < float64(ab.baseBackoff.config.InitialInterval) {
			newInterval = float64(ab.baseBackoff.config.InitialInterval)
		}
		ab.baseBackoff.currentInterval = time.Duration(newInterval)
	}
}

// RecordError records an error and adjusts backoff.
func (ab *AdaptiveBackoff) RecordError(errorType ErrorType) {
	ab.successCount = 0
	ab.errorCounts[errorType]++

	// Increase backoff for repeated errors
	if ab.errorCounts[errorType] >= ab.adaptiveConfig.ErrorThreshold {
		newInterval := float64(ab.baseBackoff.currentInterval) *
			(1.0 + ab.adaptiveConfig.AdaptationFactor)
		if newInterval > float64(ab.baseBackoff.config.MaxInterval) {
			newInterval = float64(ab.baseBackoff.config.MaxInterval)
		}
		ab.baseBackoff.currentInterval = time.Duration(newInterval)
	}
}

// NextBackOff returns the next adaptive backoff duration.
func (ab *AdaptiveBackoff) NextBackOff() time.Duration {
	return ab.baseBackoff.NextBackOff()
}

// Reset resets the adaptive backoff state.
func (ab *AdaptiveBackoff) Reset() {
	ab.baseBackoff.Reset()
	ab.errorCounts = make(map[ErrorType]int)
	ab.successCount = 0
}
