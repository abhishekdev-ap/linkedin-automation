// Package stealth provides rate limiting and throttling to avoid detection.
// This technique enforces connection request quotas, spaces out messaging intervals,
// implements cooldown periods, and tracks daily/hourly action limits.
package stealth

import (
	"fmt"
	"sync"
	"time"
)

// RateLimitConfig holds configuration for rate limiting
type RateLimitConfig struct {
	DailyConnectionLimit   int
	HourlyConnectionLimit  int
	ConnectionCooldownSecs int
	DailyMessageLimit      int
	HourlyMessageLimit     int
	MessageCooldownSecs    int
	DailySearchLimit       int
}

// DefaultRateLimitConfig returns sensible default rate limit configuration
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		DailyConnectionLimit:   50,
		HourlyConnectionLimit:  10,
		ConnectionCooldownSecs: 60,
		DailyMessageLimit:      100,
		HourlyMessageLimit:     20,
		MessageCooldownSecs:    30,
		DailySearchLimit:       100,
	}
}

// ActionType represents different types of rate-limited actions
type ActionType string

const (
	ActionConnection ActionType = "connection"
	ActionMessage    ActionType = "message"
	ActionSearch     ActionType = "search"
)

// RateLimiter handles rate limiting for various actions
type RateLimiter struct {
	config         RateLimitConfig
	mu             sync.RWMutex
	actionCounts   map[ActionType]*actionCounter
	lastActionTime map[ActionType]time.Time
}

// actionCounter tracks counts for an action type
type actionCounter struct {
	dailyCount      int
	hourlyCount     int
	dailyResetTime  time.Time
	hourlyResetTime time.Time
}

// NewRateLimiter creates a new RateLimiter with the given configuration
func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	now := time.Now()

	rl := &RateLimiter{
		config:         config,
		actionCounts:   make(map[ActionType]*actionCounter),
		lastActionTime: make(map[ActionType]time.Time),
	}

	// Initialize counters for each action type
	for _, actionType := range []ActionType{ActionConnection, ActionMessage, ActionSearch} {
		rl.actionCounts[actionType] = &actionCounter{
			dailyResetTime:  now.Add(24 * time.Hour),
			hourlyResetTime: now.Add(time.Hour),
		}
	}

	return rl
}

// CanPerformAction checks if an action can be performed without exceeding limits
func (r *RateLimiter) CanPerformAction(actionType ActionType) (bool, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Reset counters if needed
	r.checkAndResetCounters(actionType)

	counter := r.actionCounts[actionType]

	// Check daily limit
	dailyLimit := r.getDailyLimit(actionType)
	if counter.dailyCount >= dailyLimit {
		return false, fmt.Sprintf("daily limit reached (%d/%d)", counter.dailyCount, dailyLimit)
	}

	// Check hourly limit
	hourlyLimit := r.getHourlyLimit(actionType)
	if counter.hourlyCount >= hourlyLimit {
		return false, fmt.Sprintf("hourly limit reached (%d/%d)", counter.hourlyCount, hourlyLimit)
	}

	// Check cooldown
	cooldown := r.getCooldown(actionType)
	if lastTime, exists := r.lastActionTime[actionType]; exists {
		timeSince := time.Since(lastTime)
		if timeSince < cooldown {
			remaining := cooldown - timeSince
			return false, fmt.Sprintf("cooldown active, wait %v", remaining.Round(time.Second))
		}
	}

	return true, ""
}

// RecordAction records that an action was performed
func (r *RateLimiter) RecordAction(actionType ActionType) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Reset counters if needed
	r.checkAndResetCounters(actionType)

	counter := r.actionCounts[actionType]
	counter.dailyCount++
	counter.hourlyCount++

	r.lastActionTime[actionType] = time.Now()
}

// WaitForCooldown waits for the cooldown period to pass
func (r *RateLimiter) WaitForCooldown(actionType ActionType) {
	r.mu.RLock()
	lastTime, exists := r.lastActionTime[actionType]
	cooldown := r.getCooldown(actionType)
	r.mu.RUnlock()

	if !exists {
		return
	}

	timeSince := time.Since(lastTime)
	if timeSince < cooldown {
		time.Sleep(cooldown - timeSince)
	}
}

// GetRemainingQuota returns the remaining quota for an action type
func (r *RateLimiter) GetRemainingQuota(actionType ActionType) (dailyRemaining, hourlyRemaining int) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.checkAndResetCounters(actionType)

	counter := r.actionCounts[actionType]
	dailyLimit := r.getDailyLimit(actionType)
	hourlyLimit := r.getHourlyLimit(actionType)

	dailyRemaining = dailyLimit - counter.dailyCount
	hourlyRemaining = hourlyLimit - counter.hourlyCount

	if dailyRemaining < 0 {
		dailyRemaining = 0
	}
	if hourlyRemaining < 0 {
		hourlyRemaining = 0
	}

	return
}

// GetTimeUntilReset returns time until daily and hourly counters reset
func (r *RateLimiter) GetTimeUntilReset(actionType ActionType) (dailyReset, hourlyReset time.Duration) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	counter := r.actionCounts[actionType]
	now := time.Now()

	dailyReset = counter.dailyResetTime.Sub(now)
	hourlyReset = counter.hourlyResetTime.Sub(now)

	if dailyReset < 0 {
		dailyReset = 0
	}
	if hourlyReset < 0 {
		hourlyReset = 0
	}

	return
}

// checkAndResetCounters resets counters if their time window has passed
func (r *RateLimiter) checkAndResetCounters(actionType ActionType) {
	counter := r.actionCounts[actionType]
	now := time.Now()

	// Reset daily counter
	if now.After(counter.dailyResetTime) {
		counter.dailyCount = 0
		counter.dailyResetTime = now.Add(24 * time.Hour)
	}

	// Reset hourly counter
	if now.After(counter.hourlyResetTime) {
		counter.hourlyCount = 0
		counter.hourlyResetTime = now.Add(time.Hour)
	}
}

// getDailyLimit returns the daily limit for an action type
func (r *RateLimiter) getDailyLimit(actionType ActionType) int {
	switch actionType {
	case ActionConnection:
		return r.config.DailyConnectionLimit
	case ActionMessage:
		return r.config.DailyMessageLimit
	case ActionSearch:
		return r.config.DailySearchLimit
	default:
		return 100
	}
}

// getHourlyLimit returns the hourly limit for an action type
func (r *RateLimiter) getHourlyLimit(actionType ActionType) int {
	switch actionType {
	case ActionConnection:
		return r.config.HourlyConnectionLimit
	case ActionMessage:
		return r.config.HourlyMessageLimit
	case ActionSearch:
		return 20 // Default hourly search limit
	default:
		return 20
	}
}

// getCooldown returns the cooldown duration for an action type
func (r *RateLimiter) getCooldown(actionType ActionType) time.Duration {
	switch actionType {
	case ActionConnection:
		return time.Duration(r.config.ConnectionCooldownSecs) * time.Second
	case ActionMessage:
		return time.Duration(r.config.MessageCooldownSecs) * time.Second
	case ActionSearch:
		return 10 * time.Second // Default search cooldown
	default:
		return 30 * time.Second
	}
}

// GetStats returns current rate limiting statistics
func (r *RateLimiter) GetStats() map[ActionType]ActionStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[ActionType]ActionStats)

	for actionType, counter := range r.actionCounts {
		r.checkAndResetCounters(actionType)

		dailyLimit := r.getDailyLimit(actionType)
		hourlyLimit := r.getHourlyLimit(actionType)

		stats[actionType] = ActionStats{
			DailyCount:      counter.dailyCount,
			DailyLimit:      dailyLimit,
			DailyRemaining:  dailyLimit - counter.dailyCount,
			HourlyCount:     counter.hourlyCount,
			HourlyLimit:     hourlyLimit,
			HourlyRemaining: hourlyLimit - counter.hourlyCount,
			DailyResetIn:    time.Until(counter.dailyResetTime),
			HourlyResetIn:   time.Until(counter.hourlyResetTime),
		}
	}

	return stats
}

// ActionStats holds statistics for an action type
type ActionStats struct {
	DailyCount      int
	DailyLimit      int
	DailyRemaining  int
	HourlyCount     int
	HourlyLimit     int
	HourlyRemaining int
	DailyResetIn    time.Duration
	HourlyResetIn   time.Duration
}

// String returns a human-readable representation of action stats
func (s ActionStats) String() string {
	return fmt.Sprintf(
		"Daily: %d/%d (remaining: %d, resets in %v), Hourly: %d/%d (remaining: %d, resets in %v)",
		s.DailyCount, s.DailyLimit, s.DailyRemaining, s.DailyResetIn.Round(time.Minute),
		s.HourlyCount, s.HourlyLimit, s.HourlyRemaining, s.HourlyResetIn.Round(time.Minute),
	)
}

// SmartDelay calculates an intelligent delay based on current usage patterns
// Returns a longer delay when approaching limits
func (r *RateLimiter) SmartDelay(actionType ActionType) time.Duration {
	daily, hourly := r.GetRemainingQuota(actionType)

	// Base delay
	baseDelay := r.getCooldown(actionType)

	// Increase delay as we approach limits
	if hourly <= 2 {
		return baseDelay * 3 // Slow down a lot when almost out of hourly quota
	}
	if hourly <= 5 {
		return baseDelay * 2 // Slow down when approaching hourly limit
	}
	if daily <= 10 {
		return baseDelay * 2 // Slow down when approaching daily limit
	}

	return baseDelay
}
