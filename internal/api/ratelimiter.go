package api

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/VatsalSy/CloudPull/internal/errors"
)

/**
 * Token Bucket Rate Limiter for Google Drive API
 *
 * Features:
 * - Token bucket algorithm implementation
 * - Configurable rate limits
 * - Burst capacity handling
 * - Context-aware blocking
 * - Per-operation rate limiting
 * - Metrics collection
 *
 * Author: CloudPull Team
 * Updated: 2025-01-29
 */

const (
	// Default rate limit (requests per second)
	defaultRateLimit = 10

	// Default burst size
	defaultBurstSize = 20

	// Rate limit for batch operations
	batchRateLimit = 5

	// Rate limit for export operations (lower due to higher server load)
	exportRateLimit = 3
)

// RateLimiter manages API request rate limiting.
type RateLimiter struct {
	lastResetTime   time.Time
	limiter         *rate.Limiter
	batchLimiter    *rate.Limiter
	exportLimiter   *rate.Limiter
	totalRequests   int64
	blockedRequests int64
	mu              sync.RWMutex
}

// RateLimiterConfig holds rate limiter configuration.
type RateLimiterConfig struct {
	RateLimit       int
	BurstSize       int
	BatchRateLimit  int
	ExportRateLimit int
}

// DefaultRateLimiterConfig returns default configuration.
func DefaultRateLimiterConfig() *RateLimiterConfig {
	return &RateLimiterConfig{
		RateLimit:       defaultRateLimit,
		BurstSize:       defaultBurstSize,
		BatchRateLimit:  batchRateLimit,
		ExportRateLimit: exportRateLimit,
	}
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(config *RateLimiterConfig) *RateLimiter {
	if config == nil {
		config = DefaultRateLimiterConfig()
	}

	return &RateLimiter{
		limiter:       rate.NewLimiter(rate.Limit(config.RateLimit), config.BurstSize),
		batchLimiter:  rate.NewLimiter(rate.Limit(config.BatchRateLimit), config.BatchRateLimit*2),
		exportLimiter: rate.NewLimiter(rate.Limit(config.ExportRateLimit), config.ExportRateLimit),
		lastResetTime: time.Now(),
	}
}

// Wait blocks until a request can proceed.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	return rl.waitWithLimiter(ctx, rl.limiter)
}

// WaitForBatch blocks until a batch request can proceed.
func (rl *RateLimiter) WaitForBatch(ctx context.Context) error {
	return rl.waitWithLimiter(ctx, rl.batchLimiter)
}

// WaitForExport blocks until an export request can proceed.
func (rl *RateLimiter) WaitForExport(ctx context.Context) error {
	return rl.waitWithLimiter(ctx, rl.exportLimiter)
}

// waitWithLimiter performs rate limiting with a specific limiter.
func (rl *RateLimiter) waitWithLimiter(ctx context.Context, limiter *rate.Limiter) error {
	rl.incrementTotalRequests()

	// Try to reserve immediately
	if limiter.Allow() {
		return nil
	}

	// Need to wait - increment blocked counter
	rl.incrementBlockedRequests()

	// Create reservation
	reservation := limiter.Reserve()
	
	// Check if reservation was successful
	if !reservation.OK() {
		return errors.New("rate limiter reservation failed")
	}
	
	delay := reservation.Delay()

	// If delay is zero, we can proceed immediately
	if delay == 0 {
		return nil
	}

	// Create timer to prevent leak
	timer := time.NewTimer(delay)
	defer timer.Stop()

	// Wait for the required delay or context cancellation
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		// Cancel the reservation
		reservation.Cancel()
		return errors.Wrap(ctx.Err(), "rate limit wait canceled")
	}
}

// TryAcquire attempts to acquire a token without blocking.
func (rl *RateLimiter) TryAcquire() bool {
	rl.incrementTotalRequests()
	return rl.limiter.Allow()
}

// SetRateLimit updates the rate limit dynamically.
func (rl *RateLimiter) SetRateLimit(rateLimit int) {
	rl.limiter.SetLimit(rate.Limit(rateLimit))
	
	// Update batch and export limiters proportionally
	// Batch limiter typically has 50% of main rate limit
	batchRate := rateLimit / 2
	if batchRate < 1 {
		batchRate = 1
	}
	rl.batchLimiter.SetLimit(rate.Limit(batchRate))
	
	// Export limiter typically has 30% of main rate limit
	exportRate := rateLimit * 3 / 10
	if exportRate < 1 {
		exportRate = 1
	}
	rl.exportLimiter.SetLimit(rate.Limit(exportRate))
}

// SetBurstSize updates the burst size dynamically.
func (rl *RateLimiter) SetBurstSize(burstSize int) {
	rl.limiter.SetBurst(burstSize)
	
	// Update batch and export limiters proportionally
	// Batch limiter has double its rate limit as burst
	batchRate := int(rl.batchLimiter.Limit())
	rl.batchLimiter.SetBurst(batchRate * 2)
	
	// Export limiter has same burst as its rate limit
	exportRate := int(rl.exportLimiter.Limit())
	rl.exportLimiter.SetBurst(exportRate)
}

// GetMetrics returns current rate limiter metrics.
func (rl *RateLimiter) GetMetrics() RateLimiterMetrics {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	duration := time.Since(rl.lastResetTime)
	requestsPerSecond := float64(rl.totalRequests) / duration.Seconds()
	
	// Calculate block rate with zero check to prevent divide-by-zero
	var blockRate float64
	if rl.totalRequests > 0 {
		blockRate = float64(rl.blockedRequests) / float64(rl.totalRequests) * 100
	}

	return RateLimiterMetrics{
		TotalRequests:     rl.totalRequests,
		BlockedRequests:   rl.blockedRequests,
		RequestsPerSecond: requestsPerSecond,
		BlockRate:         blockRate,
		Duration:          duration,
	}
}

// ResetMetrics resets the metrics counters.
func (rl *RateLimiter) ResetMetrics() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.totalRequests = 0
	rl.blockedRequests = 0
	rl.lastResetTime = time.Now()
}

// incrementTotalRequests safely increments total request counter.
func (rl *RateLimiter) incrementTotalRequests() {
	rl.mu.Lock()
	rl.totalRequests++
	rl.mu.Unlock()
}

// incrementBlockedRequests safely increments blocked request counter.
func (rl *RateLimiter) incrementBlockedRequests() {
	rl.mu.Lock()
	rl.blockedRequests++
	rl.mu.Unlock()
}

// RateLimiterMetrics contains rate limiter statistics.
type RateLimiterMetrics struct {
	TotalRequests     int64
	BlockedRequests   int64
	RequestsPerSecond float64
	BlockRate         float64
	Duration          time.Duration
}

// AdaptiveRateLimiter adjusts rate limits based on API responses.
type AdaptiveRateLimiter struct {
	lastAdjustment time.Time
	*RateLimiter
	baseRateLimit     int
	currentRateLimit  int
	consecutiveErrors int
	mu                sync.RWMutex
}

// NewAdaptiveRateLimiter creates a rate limiter that adapts to API conditions.
func NewAdaptiveRateLimiter(config *RateLimiterConfig) *AdaptiveRateLimiter {
	if config == nil {
		config = DefaultRateLimiterConfig()
	}

	return &AdaptiveRateLimiter{
		RateLimiter:      NewRateLimiter(config),
		baseRateLimit:    config.RateLimit,
		currentRateLimit: config.RateLimit,
		lastAdjustment:   time.Now(),
	}
}

// RecordSuccess records a successful API call.
func (arl *AdaptiveRateLimiter) RecordSuccess() {
	arl.mu.Lock()
	defer arl.mu.Unlock()

	// Reset error counter on success
	arl.consecutiveErrors = 0

	// Gradually increase rate limit if we've been throttled
	if arl.currentRateLimit < arl.baseRateLimit &&
		time.Since(arl.lastAdjustment) > 30*time.Second {

		newLimit := arl.currentRateLimit + 1
		if newLimit > arl.baseRateLimit {
			newLimit = arl.baseRateLimit
		}

		arl.currentRateLimit = newLimit
		arl.SetRateLimit(newLimit)
		arl.lastAdjustment = time.Now()
	}
}

// RecordRateLimitError records a rate limit error.
func (arl *AdaptiveRateLimiter) RecordRateLimitError() {
	arl.mu.Lock()
	defer arl.mu.Unlock()

	arl.consecutiveErrors++

	// Reduce rate limit on rate limit errors
	if arl.consecutiveErrors >= 2 {
		newLimit := arl.currentRateLimit / 2
		if newLimit < 1 {
			newLimit = 1
		}

		arl.currentRateLimit = newLimit
		arl.SetRateLimit(newLimit)
		arl.lastAdjustment = time.Now()
		arl.consecutiveErrors = 0
	}
}

// GetCurrentRateLimit returns the current rate limit.
func (arl *AdaptiveRateLimiter) GetCurrentRateLimit() int {
	arl.mu.RLock()
	defer arl.mu.RUnlock()
	return arl.currentRateLimit
}

// MultiTenantRateLimiter manages rate limits for multiple users/tenants.
type MultiTenantRateLimiter struct {
	limiters      map[string]*RateLimiter
	defaultConfig *RateLimiterConfig
	mu            sync.RWMutex
}

// NewMultiTenantRateLimiter creates a rate limiter for multiple tenants.
func NewMultiTenantRateLimiter(defaultConfig *RateLimiterConfig) *MultiTenantRateLimiter {
	if defaultConfig == nil {
		defaultConfig = DefaultRateLimiterConfig()
	}

	return &MultiTenantRateLimiter{
		limiters:      make(map[string]*RateLimiter),
		defaultConfig: defaultConfig,
	}
}

// GetLimiter returns a rate limiter for a specific tenant.
func (mtrl *MultiTenantRateLimiter) GetLimiter(tenantID string) *RateLimiter {
	mtrl.mu.RLock()
	limiter, exists := mtrl.limiters[tenantID]
	mtrl.mu.RUnlock()

	if exists {
		return limiter
	}

	// Create new limiter for tenant
	mtrl.mu.Lock()
	defer mtrl.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists := mtrl.limiters[tenantID]; exists {
		return limiter
	}

	limiter = NewRateLimiter(mtrl.defaultConfig)
	mtrl.limiters[tenantID] = limiter
	return limiter
}

// RemoveLimiter removes a tenant's rate limiter.
func (mtrl *MultiTenantRateLimiter) RemoveLimiter(tenantID string) {
	mtrl.mu.Lock()
	defer mtrl.mu.Unlock()
	delete(mtrl.limiters, tenantID)
}
