// Package stealth provides natural scrolling behavior simulation.
// This technique creates realistic scroll patterns with variable speed,
// acceleration/deceleration, and occasional scroll-back movements.
package stealth

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/go-rod/rod"
)

// ScrollConfig holds configuration for scrolling behavior
type ScrollConfig struct {
	MinSpeed           int     // Minimum pixels per scroll event
	MaxSpeed           int     // Maximum pixels per scroll event
	PauseProbability   float64 // Probability of pausing during scroll
	ReverseProbability float64 // Probability of scrolling back slightly
}

// DefaultScrollConfig returns sensible default scroll configuration
func DefaultScrollConfig() ScrollConfig {
	return ScrollConfig{
		MinSpeed:           100,
		MaxSpeed:           500,
		PauseProbability:   0.3,
		ReverseProbability: 0.1,
	}
}

// Scroller handles human-like scrolling behavior
type Scroller struct {
	config ScrollConfig
	rng    *rand.Rand
	timer  *Timer
}

// NewScroller creates a new Scroller with the given configuration
func NewScroller(config ScrollConfig, timer *Timer) *Scroller {
	return &Scroller{
		config: config,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
		timer:  timer,
	}
}

// ScrollToElement scrolls the page to bring an element into view with human-like behavior
func (s *Scroller) ScrollToElement(page *rod.Page, element *rod.Element) error {
	// Get element position
	box, err := element.Shape()
	if err != nil {
		return err
	}

	// Get current scroll position
	scrollY, err := s.getScrollPosition(page)
	if err != nil {
		return err
	}

	// Calculate target scroll position (element center in viewport)
	// Use default height if we can't get viewport
	viewportHeight := 1080
	info, err := page.Eval(`window.innerHeight`)
	if err == nil {
		if h := info.Value.Int(); h > 0 {
			viewportHeight = h
		}
	}
	elementY := box.Quads[0][1]
	targetScrollY := elementY - float64(viewportHeight)/3

	// Scroll to target with natural behavior
	return s.scrollToPosition(page, scrollY, targetScrollY)
}

// ScrollDown scrolls down the page by a random amount
func (s *Scroller) ScrollDown(page *rod.Page) error {
	currentY, err := s.getScrollPosition(page)
	if err != nil {
		return err
	}

	// Random scroll distance
	distance := float64(s.config.MinSpeed + s.rng.Intn(s.config.MaxSpeed-s.config.MinSpeed))
	targetY := currentY + distance

	return s.scrollToPosition(page, currentY, targetY)
}

// ScrollUp scrolls up the page by a random amount
func (s *Scroller) ScrollUp(page *rod.Page) error {
	currentY, err := s.getScrollPosition(page)
	if err != nil {
		return err
	}

	// Random scroll distance
	distance := float64(s.config.MinSpeed + s.rng.Intn(s.config.MaxSpeed-s.config.MinSpeed))
	targetY := math.Max(0, currentY-distance)

	return s.scrollToPosition(page, currentY, targetY)
}

// ScrollToBottom scrolls to the bottom of the page with natural behavior
func (s *Scroller) ScrollToBottom(page *rod.Page) error {
	maxScroll, err := s.getMaxScroll(page)
	if err != nil {
		return err
	}

	currentY, err := s.getScrollPosition(page)
	if err != nil {
		return err
	}

	return s.scrollToPosition(page, currentY, maxScroll)
}

// ScrollToTop scrolls to the top of the page
func (s *Scroller) ScrollToTop(page *rod.Page) error {
	currentY, err := s.getScrollPosition(page)
	if err != nil {
		return err
	}

	return s.scrollToPosition(page, currentY, 0)
}

// scrollToPosition performs the actual scrolling with human-like behavior
func (s *Scroller) scrollToPosition(page *rod.Page, startY, targetY float64) error {
	currentY := startY
	direction := 1.0
	if targetY < startY {
		direction = -1.0
	}

	totalDistance := math.Abs(targetY - startY)
	if totalDistance < 10 {
		return nil // Already at target
	}

	// Calculate number of scroll steps
	steps := int(math.Max(5, totalDistance/100))

	for i := 0; i < steps; i++ {
		// Check if we've reached the target
		if (direction > 0 && currentY >= targetY) || (direction < 0 && currentY <= targetY) {
			break
		}

		// Calculate scroll amount with easing
		progress := float64(i) / float64(steps)
		scrollAmount := s.calculateScrollAmount(progress, totalDistance, direction)

		// Random chance of a reverse scroll (human-like correction)
		if s.rng.Float64() < s.config.ReverseProbability && i > 0 && i < steps-1 {
			reverseAmount := float64(20 + s.rng.Intn(50))
			if err := s.performScroll(page, -reverseAmount*direction); err != nil {
				return err
			}
			time.Sleep(time.Duration(100+s.rng.Intn(200)) * time.Millisecond)
		}

		// Perform the scroll
		if err := s.performScroll(page, scrollAmount); err != nil {
			return err
		}
		currentY += scrollAmount

		// Variable delay between scrolls
		delay := s.calculateScrollDelay(progress)
		time.Sleep(time.Duration(delay) * time.Millisecond)

		// Random chance of pausing (reading content)
		if s.rng.Float64() < s.config.PauseProbability && i > 0 {
			pauseDuration := 500 + s.rng.Intn(2000)
			time.Sleep(time.Duration(pauseDuration) * time.Millisecond)
		}
	}

	// Final adjustment to exact position
	finalY, _ := s.getScrollPosition(page)
	if math.Abs(finalY-targetY) > 10 {
		_, _ = page.Eval(fmt.Sprintf("window.scrollTo(0, %f)", targetY))
	}

	return nil
}

// calculateScrollAmount calculates how much to scroll based on progress
func (s *Scroller) calculateScrollAmount(progress, totalDistance, direction float64) float64 {
	// Use easing function for natural acceleration/deceleration
	// Fast in the middle, slower at start and end
	easedProgress := s.easeInOutCubic(progress)

	// Base scroll amount
	baseAmount := totalDistance / 10

	// Apply speed variation
	speedMultiplier := s.config.MinSpeed + s.rng.Intn(s.config.MaxSpeed-s.config.MinSpeed)
	variationMultiplier := 0.5 + s.rng.Float64()

	// Calculate final amount
	amount := baseAmount * (1 + 0.5*math.Sin(easedProgress*math.Pi)) * variationMultiplier
	amount = math.Min(amount, float64(speedMultiplier))
	amount = math.Max(amount, float64(s.config.MinSpeed))

	return amount * direction
}

// calculateScrollDelay calculates delay between scroll events
func (s *Scroller) calculateScrollDelay(progress float64) int {
	// Slower at start and end
	baseDelay := 30 + s.rng.Intn(70)

	// Add extra delay at start and end
	if progress < 0.1 || progress > 0.9 {
		baseDelay += 50 + s.rng.Intn(100)
	}

	return baseDelay
}

// easeInOutCubic applies cubic easing for smooth acceleration
func (s *Scroller) easeInOutCubic(t float64) float64 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	return 1 - math.Pow(-2*t+2, 3)/2
}

// performScroll executes a single scroll action
func (s *Scroller) performScroll(page *rod.Page, amount float64) error {
	// Use wheel event for more natural scrolling
	page.Mouse.MustScroll(0, amount)
	return nil
}

// getScrollPosition returns current scroll Y position
func (s *Scroller) getScrollPosition(page *rod.Page) (float64, error) {
	result, err := page.Eval("window.scrollY")
	if err != nil {
		return 0, err
	}
	return result.Value.Num(), nil
}

// getMaxScroll returns maximum scroll position
func (s *Scroller) getMaxScroll(page *rod.Page) (float64, error) {
	result, err := page.Eval("document.documentElement.scrollHeight - window.innerHeight")
	if err != nil {
		return 0, err
	}
	return result.Value.Num(), nil
}

// RandomScroll performs random scrolling behavior (simulating browsing)
func (s *Scroller) RandomScroll(page *rod.Page) error {
	// Random number of scroll actions
	actions := 2 + s.rng.Intn(5)

	for i := 0; i < actions; i++ {
		// Mostly scroll down, occasionally up
		if s.rng.Float64() < 0.8 {
			if err := s.ScrollDown(page); err != nil {
				return err
			}
		} else {
			if err := s.ScrollUp(page); err != nil {
				return err
			}
		}

		// Pause to "read" content
		s.timer.ThinkTime()
	}

	return nil
}
