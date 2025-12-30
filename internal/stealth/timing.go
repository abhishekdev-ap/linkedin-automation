// Package stealth provides randomized timing patterns to simulate human behavior.
// This is a MANDATORY anti-detection technique that prevents detection through
// consistent timing patterns that indicate automation.
package stealth

import (
	"math"
	"math/rand"
	"time"
)

// TimingConfig holds configuration for timing delays
type TimingConfig struct {
	MinDelay     int // Minimum delay in milliseconds
	MaxDelay     int // Maximum delay in milliseconds
	ThinkTimeMin int // Minimum think time in milliseconds
	ThinkTimeMax int // Maximum think time in milliseconds
}

// DefaultTimingConfig returns sensible default timing configuration
func DefaultTimingConfig() TimingConfig {
	return TimingConfig{
		MinDelay:     500,
		MaxDelay:     3000,
		ThinkTimeMin: 1000,
		ThinkTimeMax: 5000,
	}
}

// Timer provides randomized timing delays
type Timer struct {
	config TimingConfig
	rng    *rand.Rand
}

// NewTimer creates a new Timer with the given configuration
func NewTimer(config TimingConfig) *Timer {
	return &Timer{
		config: config,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// RandomInt returns a random integer in [0, n)
func (t *Timer) RandomInt(n int) int {
	return t.rng.Intn(n)
}

// RandomDelay waits for a random duration between min and max delays
func (t *Timer) RandomDelay() {
	delay := t.gaussianDelay(t.config.MinDelay, t.config.MaxDelay)
	time.Sleep(time.Duration(delay) * time.Millisecond)
}

// ThinkTime simulates human thinking/reading time (longer delay)
func (t *Timer) ThinkTime() {
	delay := t.gaussianDelay(t.config.ThinkTimeMin, t.config.ThinkTimeMax)
	time.Sleep(time.Duration(delay) * time.Millisecond)
}

// ShortDelay provides a brief delay for quick actions
func (t *Timer) ShortDelay() {
	delay := t.gaussianDelay(100, 500)
	time.Sleep(time.Duration(delay) * time.Millisecond)
}

// MicroDelay provides a very brief delay between rapid actions
func (t *Timer) MicroDelay() {
	delay := t.gaussianDelay(20, 100)
	time.Sleep(time.Duration(delay) * time.Millisecond)
}

// PageLoadWait waits for page load with natural variation
func (t *Timer) PageLoadWait() {
	delay := t.gaussianDelay(2000, 5000)
	time.Sleep(time.Duration(delay) * time.Millisecond)
}

// AfterClick waits after clicking (human reaction time)
func (t *Timer) AfterClick() {
	delay := t.gaussianDelay(200, 800)
	time.Sleep(time.Duration(delay) * time.Millisecond)
}

// BeforeType waits before starting to type (reading/thinking)
func (t *Timer) BeforeType() {
	delay := t.gaussianDelay(500, 1500)
	time.Sleep(time.Duration(delay) * time.Millisecond)
}

// BetweenActions waits between separate actions
func (t *Timer) BetweenActions() {
	delay := t.gaussianDelay(t.config.MinDelay, t.config.MaxDelay)
	// Sometimes add extra thinking time
	if t.rng.Float64() < 0.3 {
		delay += t.gaussianDelay(1000, 3000)
	}
	time.Sleep(time.Duration(delay) * time.Millisecond)
}

// RandomPause occasionally pauses longer (simulating distraction/thinking)
func (t *Timer) RandomPause() {
	// 20% chance of a longer pause
	if t.rng.Float64() < 0.2 {
		delay := t.gaussianDelay(3000, 8000)
		time.Sleep(time.Duration(delay) * time.Millisecond)
	}
}

// ShouldTakeBreak returns true if it's time for a short break
func (t *Timer) ShouldTakeBreak() bool {
	// Random chance of break (configurable probability)
	return t.rng.Float64() < 0.05
}

// ShortBreak takes a short break (1-3 minutes)
func (t *Timer) ShortBreak() {
	duration := t.gaussianDelay(60000, 180000) // 1-3 minutes
	time.Sleep(time.Duration(duration) * time.Millisecond)
}

// gaussianDelay returns a delay with normal distribution centered between min and max
// This creates more natural timing patterns than uniform distribution
func (t *Timer) gaussianDelay(min, max int) int {
	// Calculate mean (center of range)
	mean := float64(min+max) / 2

	// Standard deviation (about 1/6 of range for 99.7% coverage)
	stdDev := float64(max-min) / 6

	// Generate Gaussian random value using Box-Muller transform
	delay := mean + stdDev*t.boxMullerTransform()

	// Clamp to min/max bounds
	if delay < float64(min) {
		delay = float64(min)
	}
	if delay > float64(max) {
		delay = float64(max)
	}

	return int(delay)
}

// boxMullerTransform generates a standard normal random variable
func (t *Timer) boxMullerTransform() float64 {
	u1 := t.rng.Float64()
	u2 := t.rng.Float64()

	// Avoid log(0)
	for u1 == 0 {
		u1 = t.rng.Float64()
	}

	z := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
	return z
}

// exponentialDelay returns a delay with exponential distribution
// Useful for modeling inter-arrival times
func (t *Timer) exponentialDelay(mean int) int {
	u := t.rng.Float64()
	for u == 0 {
		u = t.rng.Float64()
	}
	return int(-float64(mean) * math.Log(u))
}

// DelayWithProgress returns delays that vary based on progress through a task
// At start and end: slower (learning/reviewing), middle: faster (focused work)
func (t *Timer) DelayWithProgress(current, total int) time.Duration {
	progress := float64(current) / float64(total)

	// U-shaped curve: slower at start and end
	speedMultiplier := 1.0 + 0.5*(1-4*math.Pow(progress-0.5, 2))

	baseDelay := float64(t.gaussianDelay(t.config.MinDelay, t.config.MaxDelay))
	adjustedDelay := baseDelay * speedMultiplier

	return time.Duration(adjustedDelay) * time.Millisecond
}

// HumanLikeSequence applies timing to a sequence of actions
// The pattern mimics human work: start slow, speed up, occasional pauses, slow at end
type HumanLikeSequence struct {
	timer        *Timer
	actionCount  int
	currentIndex int
}

// NewHumanLikeSequence creates a new sequence timer
func NewHumanLikeSequence(timer *Timer, totalActions int) *HumanLikeSequence {
	return &HumanLikeSequence{
		timer:       timer,
		actionCount: totalActions,
	}
}

// NextDelay returns the appropriate delay for the next action
func (h *HumanLikeSequence) NextDelay() time.Duration {
	h.currentIndex++

	// Early in sequence: learning/warming up
	if h.currentIndex < 3 {
		return h.timer.DelayWithProgress(h.currentIndex, h.actionCount) * 2
	}

	// Random pause opportunity
	if h.timer.ShouldTakeBreak() {
		h.timer.ShortBreak()
	}

	// Normal delay with progress-based adjustment
	delay := h.timer.DelayWithProgress(h.currentIndex, h.actionCount)

	// Add randomness
	if h.timer.rng.Float64() < 0.1 {
		delay += time.Duration(h.timer.gaussianDelay(500, 2000)) * time.Millisecond
	}

	return delay
}

// WaitForNext waits for the appropriate delay and returns
func (h *HumanLikeSequence) WaitForNext() {
	time.Sleep(h.NextDelay())
}
