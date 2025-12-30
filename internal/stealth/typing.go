// Package stealth provides realistic typing simulation.
// This technique simulates human typing patterns with variable keystroke
// intervals, occasional typos with corrections, and natural rhythm variations.
package stealth

import (
	"math/rand"
	"strings"
	"time"
	"unicode"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
)

// TypingConfig holds configuration for typing behavior
type TypingConfig struct {
	MinInterval     int     // Minimum ms between keystrokes
	MaxInterval     int     // Maximum ms between keystrokes
	TypoProbability float64 // Probability of making a typo (0.0-1.0)
	CorrectionDelay int     // Delay before correcting typo in ms
}

// DefaultTypingConfig returns sensible default typing configuration
func DefaultTypingConfig() TypingConfig {
	return TypingConfig{
		MinInterval:     50,
		MaxInterval:     200,
		TypoProbability: 0.02,
		CorrectionDelay: 500,
	}
}

// Typer handles realistic typing simulation
type Typer struct {
	config TypingConfig
	rng    *rand.Rand
}

// NewTyper creates a new Typer with the given configuration
func NewTyper(config TypingConfig) *Typer {
	return &Typer{
		config: config,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Type types the given text into the currently focused element with human-like behavior
func (t *Typer) Type(page *rod.Page, text string) error {
	keyboard := page.Keyboard

	for i, char := range text {
		// Calculate delay before this keystroke
		delay := t.calculateKeystrokeDelay(i, len(text), char)
		time.Sleep(time.Duration(delay) * time.Millisecond)

		// Check if we should make a typo
		if t.shouldMakeTypo() && i < len(text)-1 && unicode.IsLetter(char) {
			// Make a typo and correct it
			if err := t.makeTypoAndCorrect(page, char); err != nil {
				return err
			}
		} else {
			// Type the character normally
			if err := t.typeChar(keyboard, char); err != nil {
				return err
			}
		}

		// Occasional longer pause (thinking)
		if t.rng.Float64() < 0.05 {
			time.Sleep(time.Duration(200+t.rng.Intn(500)) * time.Millisecond)
		}
	}

	return nil
}

// TypeInElement types text into a specific element
func (t *Typer) TypeInElement(element *rod.Element, text string) error {
	// Focus the element first
	if err := element.Focus(); err != nil {
		return err
	}

	// Small delay after focusing
	time.Sleep(time.Duration(100+t.rng.Intn(200)) * time.Millisecond)

	// Get the page from element
	page := element.Page()

	return t.Type(page, text)
}

// TypeWithVariation types with additional variation in rhythm
func (t *Typer) TypeWithVariation(page *rod.Page, text string) error {
	// Split into words and type with pauses
	words := strings.Fields(text)

	for i, word := range words {
		// Type the word
		if err := t.Type(page, word); err != nil {
			return err
		}

		// Add space if not last word
		if i < len(words)-1 {
			// Pause before space
			time.Sleep(time.Duration(t.rng.Intn(100)) * time.Millisecond)

			if err := page.Keyboard.Type(input.Space); err != nil {
				return err
			}

			// Variable pause between words
			pauseTime := 50 + t.rng.Intn(150)
			if t.rng.Float64() < 0.1 {
				pauseTime += 200 + t.rng.Intn(300) // Longer pause occasionally
			}
			time.Sleep(time.Duration(pauseTime) * time.Millisecond)
		}
	}

	return nil
}

// calculateKeystrokeDelay calculates the delay before a keystroke
func (t *Typer) calculateKeystrokeDelay(index, total int, char rune) int {
	// Base delay
	baseDelay := t.config.MinInterval + t.rng.Intn(t.config.MaxInterval-t.config.MinInterval)

	// Faster in the middle of typing (momentum)
	progress := float64(index) / float64(total)
	if progress > 0.2 && progress < 0.8 {
		baseDelay = int(float64(baseDelay) * 0.8)
	}

	// Slower for special characters or after spaces
	if !unicode.IsLetter(char) && !unicode.IsDigit(char) {
		baseDelay = int(float64(baseDelay) * 1.3)
	}

	// Simulate different typing speeds for different fingers
	// Home row keys are faster
	if t.isHomeRowKey(char) {
		baseDelay = int(float64(baseDelay) * 0.9)
	}

	// Add randomness
	variation := float64(baseDelay) * 0.3
	baseDelay += int((t.rng.Float64() - 0.5) * variation)

	return max(t.config.MinInterval, baseDelay)
}

// isHomeRowKey checks if a character is on the home row (faster to type)
func (t *Typer) isHomeRowKey(char rune) bool {
	homeRow := "asdfghjkl;ASDFGHJKL:"
	return strings.ContainsRune(homeRow, char)
}

// shouldMakeTypo determines if a typo should be made
func (t *Typer) shouldMakeTypo() bool {
	return t.rng.Float64() < t.config.TypoProbability
}

// makeTypoAndCorrect makes a typo and then corrects it
func (t *Typer) makeTypoAndCorrect(page *rod.Page, correctChar rune) error {
	keyboard := page.Keyboard

	// Get a nearby incorrect character
	typoChar := t.getNearbyKey(correctChar)

	// Type the wrong character
	if err := t.typeChar(keyboard, typoChar); err != nil {
		return err
	}

	// Brief pause (noticing the mistake)
	time.Sleep(time.Duration(t.config.CorrectionDelay+t.rng.Intn(300)) * time.Millisecond)

	// Delete the wrong character
	if err := keyboard.Type(input.Backspace); err != nil {
		return err
	}

	// Pause after backspace
	time.Sleep(time.Duration(50+t.rng.Intn(100)) * time.Millisecond)

	// Type the correct character
	return t.typeChar(keyboard, correctChar)
}

// getNearbyKey returns a key near the given key on a QWERTY keyboard
func (t *Typer) getNearbyKey(char rune) rune {
	// Map of keys to their neighbors on QWERTY keyboard
	neighbors := map[rune][]rune{
		'q': {'w', 'a'},
		'w': {'q', 'e', 's'},
		'e': {'w', 'r', 'd'},
		'r': {'e', 't', 'f'},
		't': {'r', 'y', 'g'},
		'y': {'t', 'u', 'h'},
		'u': {'y', 'i', 'j'},
		'i': {'u', 'o', 'k'},
		'o': {'i', 'p', 'l'},
		'p': {'o', 'l'},
		'a': {'q', 's', 'z'},
		's': {'a', 'd', 'w', 'x'},
		'd': {'s', 'f', 'e', 'c'},
		'f': {'d', 'g', 'r', 'v'},
		'g': {'f', 'h', 't', 'b'},
		'h': {'g', 'j', 'y', 'n'},
		'j': {'h', 'k', 'u', 'm'},
		'k': {'j', 'l', 'i'},
		'l': {'k', 'o', 'p'},
		'z': {'a', 'x'},
		'x': {'z', 'c', 's'},
		'c': {'x', 'v', 'd'},
		'v': {'c', 'b', 'f'},
		'b': {'v', 'n', 'g'},
		'n': {'b', 'm', 'h'},
		'm': {'n', 'j', 'k'},
	}

	// Convert to lowercase for lookup
	lowerChar := unicode.ToLower(char)

	if nearbyKeys, ok := neighbors[lowerChar]; ok {
		typo := nearbyKeys[t.rng.Intn(len(nearbyKeys))]
		// Preserve case
		if unicode.IsUpper(char) {
			return unicode.ToUpper(typo)
		}
		return typo
	}

	// If no neighbor found, return the character itself (no typo)
	return char
}

// typeChar types a single character
func (t *Typer) typeChar(keyboard *rod.Keyboard, char rune) error {
	// Handle special characters
	switch char {
	case '\n':
		return keyboard.Type(input.Enter)
	case '\t':
		return keyboard.Type(input.Tab)
	case ' ':
		return keyboard.Type(input.Space)
	default:
		// For regular characters, use MustType with the character string
		keyboard.MustType(input.Key(char))
		return nil
	}
}

// ClearField clears a text field with human-like behavior
func (t *Typer) ClearField(page *rod.Page, element *rod.Element) error {
	// Focus the element
	if err := element.Focus(); err != nil {
		return err
	}

	// Select all (Ctrl+A or Cmd+A)
	if err := page.Keyboard.Press(input.ControlLeft); err != nil {
		return err
	}
	time.Sleep(time.Duration(30+t.rng.Intn(50)) * time.Millisecond)
	if err := page.Keyboard.Type(input.KeyA); err != nil {
		return err
	}
	if err := page.Keyboard.Release(input.ControlLeft); err != nil {
		return err
	}

	time.Sleep(time.Duration(50+t.rng.Intn(100)) * time.Millisecond)

	// Delete
	return page.Keyboard.Type(input.Backspace)
}

// max returns the larger of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
