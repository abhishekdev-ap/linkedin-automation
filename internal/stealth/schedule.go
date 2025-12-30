// Package stealth provides activity scheduling to simulate realistic work patterns.
// This technique operates only during business hours, implements realistic break
// patterns, and varies daily activity windows to simulate human work schedules.
package stealth

import (
	"math/rand"
	"time"
)

// ScheduleConfig holds configuration for activity scheduling
type ScheduleConfig struct {
	Enabled         bool
	Timezone        string
	WorkStartHour   int     // Hour to start (e.g., 9 for 9 AM)
	WorkEndHour     int     // Hour to end (e.g., 18 for 6 PM)
	BreakEnabled    bool
	MinBreakMinutes int
	MaxBreakMinutes int
	BreakProbability float64 // Probability of taking a break per check
}

// DefaultScheduleConfig returns sensible default schedule configuration
func DefaultScheduleConfig() ScheduleConfig {
	return ScheduleConfig{
		Enabled:         true,
		Timezone:        "America/New_York",
		WorkStartHour:   9,
		WorkEndHour:     18,
		BreakEnabled:    true,
		MinBreakMinutes: 10,
		MaxBreakMinutes: 30,
		BreakProbability: 0.1,
	}
}

// Scheduler handles activity scheduling
type Scheduler struct {
	config         ScheduleConfig
	rng            *rand.Rand
	location       *time.Location
	lastBreakTime  time.Time
	actionsToday   int
	lastActivityDay time.Time
}

// NewScheduler creates a new Scheduler with the given configuration
func NewScheduler(config ScheduleConfig) (*Scheduler, error) {
	loc, err := time.LoadLocation(config.Timezone)
	if err != nil {
		// Fall back to UTC
		loc = time.UTC
	}

	return &Scheduler{
		config:        config,
		rng:           rand.New(rand.NewSource(time.Now().UnixNano())),
		location:      loc,
		lastBreakTime: time.Now(),
	}, nil
}

// IsWorkingHours checks if the current time is within working hours
func (s *Scheduler) IsWorkingHours() bool {
	if !s.config.Enabled {
		return true // If scheduling disabled, always work
	}

	now := time.Now().In(s.location)
	hour := now.Hour()
	weekday := now.Weekday()

	// No work on weekends
	if weekday == time.Saturday || weekday == time.Sunday {
		return false
	}

	// Check if within work hours
	return hour >= s.config.WorkStartHour && hour < s.config.WorkEndHour
}

// WaitForWorkHours blocks until working hours begin
func (s *Scheduler) WaitForWorkHours() time.Duration {
	if s.IsWorkingHours() {
		return 0
	}

	now := time.Now().In(s.location)

	// Calculate next work start time
	nextWorkStart := s.getNextWorkStart(now)
	waitDuration := nextWorkStart.Sub(now)

	time.Sleep(waitDuration)
	return waitDuration
}

// getNextWorkStart calculates when work hours begin next
func (s *Scheduler) getNextWorkStart(now time.Time) time.Time {
	// Start with today at work start hour
	workStart := time.Date(
		now.Year(), now.Month(), now.Day(),
		s.config.WorkStartHour, 0, 0, 0,
		s.location,
	)

	// If we're past work end today, move to tomorrow
	if now.Hour() >= s.config.WorkEndHour {
		workStart = workStart.AddDate(0, 0, 1)
	}

	// Skip weekends
	for workStart.Weekday() == time.Saturday || workStart.Weekday() == time.Sunday {
		workStart = workStart.AddDate(0, 0, 1)
	}

	// Add some randomness to start time (don't always start exactly on time)
	randomMinutes := s.rng.Intn(15)
	workStart = workStart.Add(time.Duration(randomMinutes) * time.Minute)

	return workStart
}

// ShouldTakeBreak determines if it's time for a break
func (s *Scheduler) ShouldTakeBreak() bool {
	if !s.config.BreakEnabled {
		return false
	}

	// Don't take breaks too frequently
	timeSinceLastBreak := time.Since(s.lastBreakTime)
	if timeSinceLastBreak < 30*time.Minute {
		return false
	}

	// Random chance of break
	return s.rng.Float64() < s.config.BreakProbability
}

// TakeBreak takes a break and returns the break duration
func (s *Scheduler) TakeBreak() time.Duration {
	duration := s.config.MinBreakMinutes + s.rng.Intn(s.config.MaxBreakMinutes-s.config.MinBreakMinutes)
	breakDuration := time.Duration(duration) * time.Minute

	s.lastBreakTime = time.Now()

	time.Sleep(breakDuration)
	return breakDuration
}

// GetVariedWorkWindow returns a slightly varied work window for the day
// This prevents the exact same activity times every day
func (s *Scheduler) GetVariedWorkWindow() (startHour, endHour int) {
	// Vary start time by -30 to +30 minutes
	startVariation := s.rng.Intn(60) - 30

	// Vary end time by -30 to +30 minutes
	endVariation := s.rng.Intn(60) - 30

	startHour = s.config.WorkStartHour
	if startVariation > 30 {
		startHour++
	} else if startVariation < -30 {
		startHour--
	}

	endHour = s.config.WorkEndHour
	if endVariation > 30 {
		endHour++
	} else if endVariation < -30 {
		endHour--
	}

	return startHour, endHour
}

// RecordActivity records that an activity was performed
func (s *Scheduler) RecordActivity() {
	now := time.Now().In(s.location)

	// Reset counter if it's a new day
	if now.YearDay() != s.lastActivityDay.YearDay() || now.Year() != s.lastActivityDay.Year() {
		s.actionsToday = 0
		s.lastActivityDay = now
	}

	s.actionsToday++
}

// GetTodayActivityCount returns the number of activities performed today
func (s *Scheduler) GetTodayActivityCount() int {
	return s.actionsToday
}

// ShouldSlowDown checks if activity should slow down (approaching end of day)
func (s *Scheduler) ShouldSlowDown() bool {
	now := time.Now().In(s.location)
	hour := now.Hour()

	// Slow down in the last hour of work
	return hour >= s.config.WorkEndHour-1
}

// GetDelayMultiplier returns a delay multiplier based on time of day
// Slower at start/end of day, faster in the middle
func (s *Scheduler) GetDelayMultiplier() float64 {
	if !s.config.Enabled {
		return 1.0
	}

	now := time.Now().In(s.location)
	hour := now.Hour()

	// Calculate position in workday
	workdayLength := float64(s.config.WorkEndHour - s.config.WorkStartHour)
	currentPosition := float64(hour - s.config.WorkStartHour)
	progress := currentPosition / workdayLength

	// Slower at start and end (warming up / winding down)
	if progress < 0.1 || progress > 0.9 {
		return 1.5 + s.rng.Float64()*0.5
	}

	// Slightly slower after lunch (2-3 PM)
	if hour >= 14 && hour <= 15 {
		return 1.2 + s.rng.Float64()*0.3
	}

	return 1.0
}

// SimulateLunchBreak checks if it's lunch time and takes a break
func (s *Scheduler) SimulateLunchBreak() (bool, time.Duration) {
	now := time.Now().In(s.location)
	hour := now.Hour()
	minute := now.Minute()

	// Lunch time is roughly 12:00 - 13:30
	isLunchTime := hour == 12 || (hour == 13 && minute < 30)

	if !isLunchTime {
		return false, 0
	}

	// Take a longer break for lunch (30-60 minutes)
	duration := 30 + s.rng.Intn(30)
	breakDuration := time.Duration(duration) * time.Minute

	s.lastBreakTime = time.Now()
	time.Sleep(breakDuration)

	return true, breakDuration
}

// GetNextActivityTime returns when the next activity should occur
// based on current state and scheduling rules
func (s *Scheduler) GetNextActivityTime() time.Time {
	now := time.Now()

	// If not working hours, return next work start
	if !s.IsWorkingHours() {
		return s.getNextWorkStart(now.In(s.location))
	}

	// If should take a break, return time after break
	if s.ShouldTakeBreak() {
		duration := s.config.MinBreakMinutes + s.rng.Intn(s.config.MaxBreakMinutes-s.config.MinBreakMinutes)
		return now.Add(time.Duration(duration) * time.Minute)
	}

	// Otherwise, return now (can proceed immediately)
	return now
}
