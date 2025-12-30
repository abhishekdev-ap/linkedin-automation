// Package stealth provides mouse hovering patterns simulation.
// This technique adds random hover events, natural cursor wandering,
// and realistic movement patterns during page interactions.
package stealth

import (
	"math"
	"math/rand"
	"time"

	"github.com/go-rod/rod"
)

// HoverConfig holds configuration for hover behavior
type HoverConfig struct {
	MinHoverTime    int     // Minimum hover time in ms
	MaxHoverTime    int     // Maximum hover time in ms
	WanderFrequency float64 // How often cursor wanders (0.0-1.0)
	WanderRadius    int     // Maximum pixels to wander from target
}

// DefaultHoverConfig returns sensible default hover configuration
func DefaultHoverConfig() HoverConfig {
	return HoverConfig{
		MinHoverTime:    200,
		MaxHoverTime:    1000,
		WanderFrequency: 0.3,
		WanderRadius:    50,
	}
}

// Hoverer handles mouse hovering behavior
type Hoverer struct {
	config       HoverConfig
	rng          *rand.Rand
	mouseMover   *MouseMover
	lastX, lastY float64
}

// NewHoverer creates a new Hoverer with the given configuration
func NewHoverer(config HoverConfig, mouseMover *MouseMover) *Hoverer {
	return &Hoverer{
		config:     config,
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
		mouseMover: mouseMover,
		lastX:      500,
		lastY:      500,
	}
}

// HoverOverElement moves to an element and hovers for a natural duration
func (h *Hoverer) HoverOverElement(page *rod.Page, element *rod.Element) error {
	// Move to the element
	if err := h.mouseMover.MoveTo(page, element); err != nil {
		return err
	}

	// Get element position for tracking
	box, err := element.Shape()
	if err != nil {
		return err
	}
	quad := box.Quads[0]
	h.lastX = (quad[0] + quad[2] + quad[4] + quad[6]) / 4
	h.lastY = (quad[1] + quad[3] + quad[5] + quad[7]) / 4

	// Hover for a random duration
	hoverTime := h.config.MinHoverTime + h.rng.Intn(h.config.MaxHoverTime-h.config.MinHoverTime)

	// During hover, occasionally make small movements
	return h.hoverWithMicroMovements(page, hoverTime)
}

// hoverWithMicroMovements hovers while making small natural movements
func (h *Hoverer) hoverWithMicroMovements(page *rod.Page, durationMs int) error {
	startTime := time.Now()
	endTime := startTime.Add(time.Duration(durationMs) * time.Millisecond)

	for time.Now().Before(endTime) {
		// Small chance of micro-movement during hover
		if h.rng.Float64() < 0.2 {
			// Small movement around current position
			dx := float64((h.rng.Intn(6) - 3))
			dy := float64((h.rng.Intn(6) - 3))

			newX := h.lastX + dx
			newY := h.lastY + dy

			page.Mouse.MustMoveTo(newX, newY)

			h.lastX = newX
			h.lastY = newY
		}

		time.Sleep(time.Duration(50+h.rng.Intn(100)) * time.Millisecond)
	}

	return nil
}

// WanderAround makes the cursor wander naturally around the page
func (h *Hoverer) WanderAround(page *rod.Page) error {
	// Get viewport dimensions - use EvalWindow for proper dimensions
	width := 1920  // Default fallback
	height := 1080

	// Try to get actual window size
	info, err := page.Eval(`({width: window.innerWidth, height: window.innerHeight})`)
	if err == nil {
		if w := info.Value.Get("width").Int(); w > 0 {
			width = w
		}
		if h := info.Value.Get("height").Int(); h > 0 {
			height = h
		}
	}

	// Generate a random target point
	targetX := float64(h.rng.Intn(width-100) + 50)
	targetY := float64(h.rng.Intn(height-100) + 50)

	// Move there with natural path
	return h.moveToWithWander(page, targetX, targetY)
}

// moveToWithWander moves to a target while wandering naturally
func (h *Hoverer) moveToWithWander(page *rod.Page, targetX, targetY float64) error {
	// Generate a wandering path
	path := h.generateWanderingPath(h.lastX, h.lastY, targetX, targetY)

	// Follow the path
	for _, point := range path {
		page.Mouse.MustMoveTo(point.X, point.Y)

		h.lastX = point.X
		h.lastY = point.Y

		// Variable speed
		time.Sleep(time.Duration(10+h.rng.Intn(30)) * time.Millisecond)
	}

	return nil
}

// generateWanderingPath creates a path with natural wandering
func (h *Hoverer) generateWanderingPath(startX, startY, endX, endY float64) []Point {
	// Calculate distance
	dx := endX - startX
	dy := endY - startY
	distance := math.Sqrt(dx*dx + dy*dy)

	// Number of steps based on distance
	steps := int(math.Max(10, distance/20))
	path := make([]Point, 0, steps)

	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)

		// Base position (linear interpolation)
		x := startX + dx*t
		y := startY + dy*t

		// Add wandering offset (decreases as we approach target)
		wanderStrength := (1 - t) * float64(h.config.WanderRadius)
		if h.rng.Float64() < h.config.WanderFrequency {
			angle := h.rng.Float64() * 2 * math.Pi
			x += math.Cos(angle) * wanderStrength * h.rng.Float64()
			y += math.Sin(angle) * wanderStrength * h.rng.Float64()
		}

		path = append(path, Point{X: x, Y: y})
	}

	return path
}

// RandomHover hovers over a random element on the page
func (h *Hoverer) RandomHover(page *rod.Page) error {
	// Find some hoverable elements
	elements, err := page.Elements("a, button, input, [role='button'], [class*='button']")
	if err != nil || len(elements) == 0 {
		// If no elements found, just wander
		return h.WanderAround(page)
	}

	// Pick a random element
	element := elements[h.rng.Intn(len(elements))]

	// Check if element is visible
	visible, err := element.Visible()
	if err != nil || !visible {
		return h.WanderAround(page)
	}

	// Hover over it
	return h.HoverOverElement(page, element)
}

// SimulateReading simulates reading behavior with corresponding cursor movements
func (h *Hoverer) SimulateReading(page *rod.Page, durationMs int) error {
	startTime := time.Now()
	endTime := startTime.Add(time.Duration(durationMs) * time.Millisecond)

	// Get viewport - use defaults with fallback
	width := 1920
	height := 1080

	info, err := page.Eval(`({width: window.innerWidth, height: window.innerHeight})`)
	if err == nil {
		if w := info.Value.Get("width").Int(); w > 0 {
			width = w
		}
		if ht := info.Value.Get("height").Int(); ht > 0 {
			height = ht
		}
	}

	// Start position (simulate reading from top-left area)
	currentX := float64(100 + h.rng.Intn(200))
	currentY := float64(100 + h.rng.Intn(100))

	for time.Now().Before(endTime) {
		// Move cursor slightly as if following text
		currentX += float64(5 + h.rng.Intn(15))

		// Occasional line break (cursor moves back and down)
		if currentX > float64(width)-200 || h.rng.Float64() < 0.05 {
			currentX = float64(100 + h.rng.Intn(100))
			currentY += float64(20 + h.rng.Intn(30))
		}

		// Keep in bounds
		currentY = math.Min(currentY, float64(height)-100)

		// Move cursor
		page.Mouse.MustMoveTo(currentX, currentY)

		h.lastX = currentX
		h.lastY = currentY

		// Reading pace delay
		time.Sleep(time.Duration(30+h.rng.Intn(70)) * time.Millisecond)
	}

	return nil
}

// IdleMovement makes small idle movements (simulating a resting hand)
func (h *Hoverer) IdleMovement(page *rod.Page) error {
	// Very small movements around current position
	for i := 0; i < 3+h.rng.Intn(5); i++ {
		dx := float64((h.rng.Intn(10) - 5))
		dy := float64((h.rng.Intn(10) - 5))

		newX := h.lastX + dx
		newY := h.lastY + dy

		page.Mouse.MustMoveTo(newX, newY)

		h.lastX = newX
		h.lastY = newY

		time.Sleep(time.Duration(100+h.rng.Intn(300)) * time.Millisecond)
	}

	return nil
}
