// Package stealth provides human-like mouse movement simulation using Bézier curves.
// This is a MANDATORY anti-detection technique that creates natural, curved mouse paths
// instead of straight-line movements that are easily detected by bot-detection systems.
package stealth

import (
	"math"
	"math/rand"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// MouseConfig holds configuration for mouse movement behavior
type MouseConfig struct {
	MinSpeed     float64 // Minimum speed multiplier (0.5 = half speed)
	MaxSpeed     float64 // Maximum speed multiplier (1.5 = 1.5x speed)
	Overshoot    bool    // Whether to overshoot target and correct
	MicroMoves   bool    // Whether to add micro-movements
	BezierPoints int     // Number of control points for Bézier curves
}

// DefaultMouseConfig returns sensible default mouse configuration
func DefaultMouseConfig() MouseConfig {
	return MouseConfig{
		MinSpeed:     0.5,
		MaxSpeed:     1.5,
		Overshoot:    true,
		MicroMoves:   true,
		BezierPoints: 4,
	}
}

// Point represents a 2D coordinate
type Point struct {
	X, Y float64
}

// MouseMover handles human-like mouse movement
type MouseMover struct {
	config MouseConfig
	rng    *rand.Rand
}

// NewMouseMover creates a new MouseMover with the given configuration
func NewMouseMover(config MouseConfig) *MouseMover {
	return &MouseMover{
		config: config,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// MoveTo moves the mouse from current position to target element with human-like behavior
func (m *MouseMover) MoveTo(page *rod.Page, element *rod.Element) error {
	// Get target element position
	box, err := element.Shape()
	if err != nil {
		return err
	}

	// Calculate center of target element
	quad := box.Quads[0]
	targetX := (quad[0] + quad[2] + quad[4] + quad[6]) / 4
	targetY := (quad[1] + quad[3] + quad[5] + quad[7]) / 4

	// Get starting position (use page center if unknown)
	startX, startY := m.getStartPosition(page)

	// Generate path with Bézier curves
	path := m.generateBezierPath(
		Point{startX, startY},
		Point{targetX, targetY},
	)

	// Calculate speed based on distance
	distance := m.calculateDistance(Point{startX, startY}, Point{targetX, targetY})
	baseDelay := m.calculateBaseDelay(distance)

	// Apply overshoot if enabled
	if m.config.Overshoot && m.rng.Float64() < 0.7 {
		path = m.applyOvershoot(path, Point{targetX, targetY})
	}

	// Apply micro-movements if enabled
	if m.config.MicroMoves {
		path = m.applyMicroMovements(path)
	}

	// Execute the movement
	for i, point := range path {
		// Move to this point
		page.Mouse.MustMoveTo(point.X, point.Y)

		// Variable delay between points
		delay := m.calculatePointDelay(baseDelay, i, len(path))
		time.Sleep(time.Duration(delay) * time.Millisecond)
	}

	return nil
}

// MoveToClick moves to element and clicks with human-like behavior
func (m *MouseMover) MoveToClick(page *rod.Page, element *rod.Element) error {
	// First move to the element
	if err := m.MoveTo(page, element); err != nil {
		return err
	}

	// Small pause before clicking (human reaction time)
	time.Sleep(time.Duration(50+m.rng.Intn(150)) * time.Millisecond)

	// Click with slight randomness
	return element.Click(proto.InputMouseButtonLeft, 1)
}

// getStartPosition returns the current mouse position or a default
func (m *MouseMover) getStartPosition(page *rod.Page) (float64, float64) {
	// Default to somewhere in the viewport if position unknown
	return float64(100 + m.rng.Intn(200)), float64(100 + m.rng.Intn(200))
}

// generateBezierPath creates a curved path using cubic Bézier curves
func (m *MouseMover) generateBezierPath(start, end Point) []Point {
	// Generate control points for natural curve
	controlPoints := m.generateControlPoints(start, end)

	// Number of steps based on distance
	distance := m.calculateDistance(start, end)
	steps := int(math.Max(20, distance/10))

	// Generate points along the Bézier curve
	path := make([]Point, 0, steps)

	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)

		// Add easing for natural acceleration/deceleration
		t = m.easeInOutQuad(t)

		point := m.bezierPoint(controlPoints, t)
		path = append(path, point)
	}

	return path
}

// generateControlPoints creates control points for Bézier curve
func (m *MouseMover) generateControlPoints(start, end Point) []Point {
	// Calculate midpoint and distance
	midX := (start.X + end.X) / 2
	midY := (start.Y + end.Y) / 2
	distance := m.calculateDistance(start, end)

	// Random deviation perpendicular to the line
	deviation := distance * 0.2 * (m.rng.Float64() - 0.5)

	// Calculate perpendicular direction
	dx := end.X - start.X
	dy := end.Y - start.Y
	length := math.Sqrt(dx*dx + dy*dy)
	if length == 0 {
		length = 1
	}

	perpX := -dy / length
	perpY := dx / length

	// Generate 4 control points for cubic Bézier
	points := make([]Point, 4)
	points[0] = start

	// First control point - slight deviation from start
	points[1] = Point{
		X: start.X + (end.X-start.X)*0.25 + perpX*deviation*m.rng.Float64(),
		Y: start.Y + (end.Y-start.Y)*0.25 + perpY*deviation*m.rng.Float64(),
	}

	// Second control point - deviation at midpoint
	points[2] = Point{
		X: midX + perpX*deviation,
		Y: midY + perpY*deviation,
	}

	points[3] = end

	return points
}

// bezierPoint calculates a point on a Bézier curve at parameter t
func (m *MouseMover) bezierPoint(points []Point, t float64) Point {
	n := len(points)
	if n == 0 {
		return Point{0, 0}
	}

	// De Casteljau's algorithm
	tmp := make([]Point, n)
	copy(tmp, points)

	for i := 1; i < n; i++ {
		for j := 0; j < n-i; j++ {
			tmp[j] = Point{
				X: tmp[j].X*(1-t) + tmp[j+1].X*t,
				Y: tmp[j].Y*(1-t) + tmp[j+1].Y*t,
			}
		}
	}

	return tmp[0]
}

// easeInOutQuad applies easing function for natural acceleration
func (m *MouseMover) easeInOutQuad(t float64) float64 {
	if t < 0.5 {
		return 2 * t * t
	}
	return 1 - math.Pow(-2*t+2, 2)/2
}

// applyOvershoot adds overshoot and correction to the path
func (m *MouseMover) applyOvershoot(path []Point, target Point) []Point {
	if len(path) == 0 {
		return path
	}

	// Calculate overshoot amount (5-15% past target)
	overshootPercent := 0.05 + m.rng.Float64()*0.1
	lastPoint := path[len(path)-1]

	// Direction from last point to target
	dx := target.X - path[len(path)-2].X
	dy := target.Y - path[len(path)-2].Y

	// Add overshoot point
	overshootPoint := Point{
		X: target.X + dx*overshootPercent,
		Y: target.Y + dy*overshootPercent,
	}
	path = append(path, overshootPoint)

	// Add correction path back to target
	correctionSteps := 3 + m.rng.Intn(3)
	for i := 1; i <= correctionSteps; i++ {
		t := float64(i) / float64(correctionSteps)
		correctionPoint := Point{
			X: overshootPoint.X + (target.X-overshootPoint.X)*t,
			Y: overshootPoint.Y + (target.Y-overshootPoint.Y)*t,
		}
		path = append(path, correctionPoint)
	}

	// Ensure final point is exactly on target
	path[len(path)-1] = lastPoint

	return path
}

// applyMicroMovements adds small random deviations to simulate hand tremor
func (m *MouseMover) applyMicroMovements(path []Point) []Point {
	for i := range path {
		if i == 0 || i == len(path)-1 {
			continue // Don't modify start/end points
		}

		// Small random deviation (1-3 pixels)
		deviation := 1 + m.rng.Float64()*2
		angle := m.rng.Float64() * 2 * math.Pi

		path[i].X += deviation * math.Cos(angle)
		path[i].Y += deviation * math.Sin(angle)
	}
	return path
}

// calculateDistance calculates Euclidean distance between two points
func (m *MouseMover) calculateDistance(a, b Point) float64 {
	dx := b.X - a.X
	dy := b.Y - a.Y
	return math.Sqrt(dx*dx + dy*dy)
}

// calculateBaseDelay calculates base delay based on distance
func (m *MouseMover) calculateBaseDelay(distance float64) float64 {
	// Longer distances = faster movement (Fitts's Law approximation)
	// But with minimum speed
	baseSpeed := m.config.MinSpeed + m.rng.Float64()*(m.config.MaxSpeed-m.config.MinSpeed)
	return math.Max(5, (distance/500)*10/baseSpeed)
}

// calculatePointDelay calculates delay for a specific point in the path
func (m *MouseMover) calculatePointDelay(baseDelay float64, index, total int) float64 {
	// Vary delay along the path (slower at start/end, faster in middle)
	progress := float64(index) / float64(total)

	// Bell curve for speed variation
	speedMultiplier := 1.0 - 0.5*math.Sin(progress*math.Pi)

	// Add random variation
	randomFactor := 0.8 + m.rng.Float64()*0.4

	return baseDelay * speedMultiplier * randomFactor
}

// RandomMouseMovement performs random mouse movement to simulate idle user
func (m *MouseMover) RandomMouseMovement(page *rod.Page) error {
	// Get viewport dimensions - use fallback values
	width := 1920
	height := 1080

	// Try to get actual dimensions
	info, err := page.Eval(`({width: window.innerWidth, height: window.innerHeight})`)
	if err == nil {
		if w := info.Value.Get("width").Int(); w > 0 {
			width = w
		}
		if h := info.Value.Get("height").Int(); h > 0 {
			height = h
		}
	}

	// Generate random target within viewport
	targetX := float64(m.rng.Intn(width-100) + 50)
	targetY := float64(m.rng.Intn(height-100) + 50)

	startX, startY := m.getStartPosition(page)
	path := m.generateBezierPath(Point{startX, startY}, Point{targetX, targetY})

	// Execute slower movement
	for _, point := range path {
		page.Mouse.MustMoveTo(point.X, point.Y)
		time.Sleep(time.Duration(10+m.rng.Intn(20)) * time.Millisecond)
	}

	return nil
}
