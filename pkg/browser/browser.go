// Package browser provides a Rod browser wrapper with stealth mode integration.
// It handles browser initialization, stealth plugin integration, and page lifecycle management.
package browser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/linkedin-automation/linkedin-bot/internal/stealth"
)

// Config holds browser configuration
type Config struct {
	Headless    bool
	SlowMotion  int
	DevTools    bool
	UserDataDir string
	UserAgents  []string
	Viewport    Viewport
}

// Viewport defines browser window dimensions
type Viewport struct {
	Width  int
	Height int
}

// DefaultConfig returns default browser configuration
func DefaultConfig() Config {
	return Config{
		Headless:   false,
		SlowMotion: 0,
		DevTools:   false,
		UserAgents: []string{
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		},
		Viewport: Viewport{
			Width:  1920,
			Height: 1080,
		},
	}
}

// Browser wraps rod.Browser with stealth capabilities
type Browser struct {
	config            Config
	browser           *rod.Browser
	page              *rod.Page
	fingerprintMasker *stealth.FingerprintMasker
	mouseMover        *stealth.MouseMover
	timer             *stealth.Timer
	typer             *stealth.Typer
	scroller          *stealth.Scroller
	hoverer           *stealth.Hoverer
}

// New creates a new Browser instance
func New(config Config) (*Browser, error) {
	// Set up launcher with stealth options
	l := launcher.New().
		Headless(config.Headless).
		Devtools(config.DevTools).
		Set("disable-blink-features", "AutomationControlled").
		Set("disable-dev-shm-usage", "").
		Set("disable-infobars", "").
		Set("disable-extensions", "").
		Set("disable-gpu", "").
		Set("no-sandbox", "").
		Set("disable-web-security", "")

	// Set user data directory if specified
	if config.UserDataDir != "" {
		l = l.UserDataDir(config.UserDataDir)
	}

	// Launch browser
	url, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("failed to launch browser: %w", err)
	}

	// Connect to browser
	browser := rod.New().ControlURL(url)
	if config.SlowMotion > 0 {
		browser = browser.SlowMotion(time.Duration(config.SlowMotion) * time.Millisecond)
	}

	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to browser: %w", err)
	}

	// Create stealth components
	fingerprintConfig := stealth.DefaultFingerprintConfig()
	if len(config.UserAgents) > 0 {
		fingerprintConfig.UserAgents = config.UserAgents
	}
	fingerprintConfig.Viewports = []stealth.Viewport{
		{Width: config.Viewport.Width, Height: config.Viewport.Height},
	}

	timer := stealth.NewTimer(stealth.DefaultTimingConfig())
	mouseMover := stealth.NewMouseMover(stealth.DefaultMouseConfig())
	scroller := stealth.NewScroller(stealth.DefaultScrollConfig(), timer)

	return &Browser{
		config:            config,
		browser:           browser,
		fingerprintMasker: stealth.NewFingerprintMasker(fingerprintConfig),
		mouseMover:        mouseMover,
		timer:             timer,
		typer:             stealth.NewTyper(stealth.DefaultTypingConfig()),
		scroller:          scroller,
		hoverer:           stealth.NewHoverer(stealth.DefaultHoverConfig(), mouseMover),
	}, nil
}

// NewPage creates a new page with stealth mode applied
func (b *Browser) NewPage() (*rod.Page, error) {
	page, err := b.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, fmt.Errorf("failed to create page: %w", err)
	}

	// Apply stealth mode
	if err := b.applyStealthMode(page); err != nil {
		return nil, fmt.Errorf("failed to apply stealth mode: %w", err)
	}

	b.page = page
	return page, nil
}

// applyStealthMode applies all stealth techniques to a page
func (b *Browser) applyStealthMode(page *rod.Page) error {
	// Set viewport
	if err := page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:             b.config.Viewport.Width,
		Height:            b.config.Viewport.Height,
		DeviceScaleFactor: 1,
		Mobile:            false,
	}); err != nil {
		return err
	}

	// Set user agent
	userAgent := b.fingerprintMasker.GetRandomUserAgent()
	if err := page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
		UserAgent: userAgent,
	}); err != nil {
		return err
	}

	// Apply fingerprint masking
	return b.fingerprintMasker.ApplyToPage(page)
}

// Navigate navigates to a URL with stealth behavior
func (b *Browser) Navigate(url string) error {
	if b.page == nil {
		page, err := b.NewPage()
		if err != nil {
			return err
		}
		b.page = page
	}

	// Navigate with wait
	if err := b.page.Navigate(url); err != nil {
		return fmt.Errorf("failed to navigate: %w", err)
	}

	// Wait for page load
	if err := b.page.WaitLoad(); err != nil {
		return fmt.Errorf("failed to wait for load: %w", err)
	}

	// Wait for network to be idle (more reliable than just load)
	b.WaitForNetworkIdle(3 * time.Second)

	// Apply stealth after navigation (some scripts inject on load)
	if err := b.fingerprintMasker.ApplyToPage(b.page); err != nil {
		return fmt.Errorf("failed to reapply stealth: %w", err)
	}

	// Random delay after navigation
	b.timer.PageLoadWait()

	return nil
}

// WaitForNetworkIdle waits for network to be idle
func (b *Browser) WaitForNetworkIdle(timeout time.Duration) {
	_ = b.page.Timeout(timeout).WaitRequestIdle(500*time.Millisecond, nil, nil, nil)
}

// Click clicks an element with human-like behavior
func (b *Browser) Click(selector string) error {
	element, err := b.page.Element(selector)
	if err != nil {
		return fmt.Errorf("element not found: %s: %w", selector, err)
	}

	// Scroll to element if needed
	if err := b.scroller.ScrollToElement(b.page, element); err != nil {
		return err
	}

	// Move to element and click
	return b.mouseMover.MoveToClick(b.page, element)
}

// ClickWithRetry clicks an element with retry logic for dynamic pages
func (b *Browser) ClickWithRetry(selector string, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		element, err := b.page.Timeout(3 * time.Second).Element(selector)
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Wait for element to be visible and stable
		if err := element.WaitVisible(); err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Scroll to element
		if err := b.scroller.ScrollToElement(b.page, element); err != nil {
			lastErr = err
			continue
		}

		// Click
		if err := b.mouseMover.MoveToClick(b.page, element); err != nil {
			lastErr = err
			continue
		}

		return nil
	}
	return fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// FindElementWithFallback tries multiple selectors and returns the first match
func (b *Browser) FindElementWithFallback(selectors []string, timeout time.Duration) (*rod.Element, error) {
	for _, selector := range selectors {
		element, err := b.page.Timeout(timeout).Element(selector)
		if err == nil && element != nil {
			// Verify element is visible
			visible, _ := element.Visible()
			if visible {
				return element, nil
			}
		}
	}
	return nil, fmt.Errorf("no matching element found for selectors: %v", selectors)
}

// ClickFirstMatch clicks the first matching element from multiple selectors
func (b *Browser) ClickFirstMatch(selectors []string) error {
	element, err := b.FindElementWithFallback(selectors, 5*time.Second)
	if err != nil {
		return err
	}

	// Scroll to element
	if err := b.scroller.ScrollToElement(b.page, element); err != nil {
		return err
	}

	// Random delay before click
	b.timer.ShortDelay()

	return b.mouseMover.MoveToClick(b.page, element)
}

// WaitAndClick waits for element to be clickable then clicks
func (b *Browser) WaitAndClick(selector string, timeout time.Duration) error {
	element, err := b.page.Timeout(timeout).Element(selector)
	if err != nil {
		return fmt.Errorf("element not found: %s: %w", selector, err)
	}

	// Wait for element to be visible
	if err := element.WaitVisible(); err != nil {
		return fmt.Errorf("element not visible: %s: %w", selector, err)
	}

	// Wait for element to be interactable
	_, err = element.WaitInteractable()
	if err != nil {
		return fmt.Errorf("element not interactable: %s: %w", selector, err)
	}

	// Scroll to element
	if err := b.scroller.ScrollToElement(b.page, element); err != nil {
		return err
	}

	// Random delay
	b.timer.ShortDelay()

	return b.mouseMover.MoveToClick(b.page, element)
}

// Type types text into an element with human-like behavior
func (b *Browser) Type(selector, text string) error {
	element, err := b.page.Element(selector)
	if err != nil {
		return fmt.Errorf("element not found: %s: %w", selector, err)
	}

	// Click to focus
	if err := b.Click(selector); err != nil {
		return err
	}

	b.timer.BeforeType()

	// Type with human-like behavior
	return b.typer.TypeInElement(element, text)
}

// TypeWithClear clears the input first then types
func (b *Browser) TypeWithClear(selector, text string) error {
	element, err := b.page.Element(selector)
	if err != nil {
		return fmt.Errorf("element not found: %s: %w", selector, err)
	}

	// Click to focus
	if err := b.Click(selector); err != nil {
		return err
	}

	// Clear existing content
	if err := element.SelectAllText(); err == nil {
		element.MustInput("")
	}

	b.timer.BeforeType()

	// Type with human-like behavior
	return b.typer.TypeInElement(element, text)
}

// WaitForElement waits for an element to appear
func (b *Browser) WaitForElement(selector string, timeout time.Duration) (*rod.Element, error) {
	return b.page.Timeout(timeout).Element(selector)
}

// WaitForElementVisible waits for element to be visible
func (b *Browser) WaitForElementVisible(selector string, timeout time.Duration) (*rod.Element, error) {
	element, err := b.page.Timeout(timeout).Element(selector)
	if err != nil {
		return nil, err
	}
	if err := element.WaitVisible(); err != nil {
		return nil, err
	}
	return element, nil
}

// IsElementPresent checks if an element exists on the page
func (b *Browser) IsElementPresent(selector string) bool {
	_, err := b.page.Timeout(2 * time.Second).Element(selector)
	return err == nil
}

// WaitForNavigation waits for page navigation to complete
func (b *Browser) WaitForNavigation(timeout time.Duration) {
	wait := b.page.Timeout(timeout).WaitNavigation(proto.PageLifecycleEventNameDOMContentLoaded)
	wait()
}

// GetPage returns the current page
func (b *Browser) GetPage() *rod.Page {
	return b.page
}

// Screenshot takes a screenshot
func (b *Browser) Screenshot(filename string) error {
	data, err := b.page.Screenshot(true, nil)
	if err != nil {
		return err
	}

	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

// Close closes the browser
func (b *Browser) Close() error {
	if b.browser != nil {
		return b.browser.Close()
	}
	return nil
}

// SaveCookies saves session cookies to a file
func (b *Browser) SaveCookies(filename string) error {
	cookies, err := b.page.Cookies(nil)
	if err != nil {
		return fmt.Errorf("failed to get cookies: %w", err)
	}

	data, err := json.MarshalIndent(cookies, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cookies: %w", err)
	}

	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

// LoadCookies loads session cookies from a file
func (b *Browser) LoadCookies(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No cookies file, that's OK
		}
		return fmt.Errorf("failed to read cookies file: %w", err)
	}

	var cookies []*proto.NetworkCookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return fmt.Errorf("failed to unmarshal cookies: %w", err)
	}

	// Convert to SetCookieParam
	for _, cookie := range cookies {
		err := b.page.SetCookies([]*proto.NetworkCookieParam{{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Domain:   cookie.Domain,
			Path:     cookie.Path,
			Secure:   cookie.Secure,
			HTTPOnly: cookie.HTTPOnly,
		}})
		if err != nil {
			return fmt.Errorf("failed to set cookie: %w", err)
		}
	}

	return nil
}

// GetMouseMover returns the mouse mover for direct access
func (b *Browser) GetMouseMover() *stealth.MouseMover {
	return b.mouseMover
}

// GetTimer returns the timer for direct access
func (b *Browser) GetTimer() *stealth.Timer {
	return b.timer
}

// GetTyper returns the typer for direct access
func (b *Browser) GetTyper() *stealth.Typer {
	return b.typer
}

// GetScroller returns the scroller for direct access
func (b *Browser) GetScroller() *stealth.Scroller {
	return b.scroller
}

// GetHoverer returns the hoverer for direct access
func (b *Browser) GetHoverer() *stealth.Hoverer {
	return b.hoverer
}

// SimulateIdleUser simulates idle user behavior
func (b *Browser) SimulateIdleUser() error {
	// Random actions: hover, scroll, idle movements
	action := b.timer.RandomInt(3)

	switch action {
	case 0:
		return b.hoverer.RandomHover(b.page)
	case 1:
		return b.scroller.RandomScroll(b.page)
	case 2:
		return b.hoverer.IdleMovement(b.page)
	}

	return nil
}

// ===============================================
// ENHANCED ACCURACY METHODS
// ===============================================

// SmartClick waits for element stability, scrolls it into view, and clicks with retry
func (b *Browser) SmartClick(selector string, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		// Find element with timeout
		element, err := b.page.Timeout(5 * time.Second).Element(selector)
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Wait for visibility
		if err := element.WaitVisible(); err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Wait for element to stop moving (stability)
		if err := element.WaitStable(300 * time.Millisecond); err != nil {
			lastErr = err
			time.Sleep(300 * time.Millisecond)
			continue
		}

		// Scroll element into center of viewport
		if err := b.ScrollIntoViewCenter(element); err != nil {
			lastErr = err
			continue
		}

		// Small delay for natural behavior
		b.timer.ShortDelay()

		// Click with human-like movement
		if err := b.mouseMover.MoveToClick(b.page, element); err != nil {
			lastErr = err
			continue
		}

		return nil
	}
	return fmt.Errorf("SmartClick failed after %d retries: %w", maxRetries, lastErr)
}

// ScrollIntoViewCenter scrolls an element to the center of the viewport
func (b *Browser) ScrollIntoViewCenter(element *rod.Element) error {
	// Get element's bounding box
	box, err := element.Shape()
	if err != nil {
		// Fallback to simple scroll
		return element.ScrollIntoView()
	}

	if box == nil || len(box.Quads) == 0 {
		return element.ScrollIntoView()
	}

	// Calculate center position
	quad := box.Quads[0]
	centerY := (quad[1] + quad[7]) / 2

	// Get viewport height
	viewportHeight := float64(b.config.Viewport.Height)

	// Scroll to put element in center
	scrollY := centerY - (viewportHeight / 2)
	if scrollY < 0 {
		scrollY = 0
	}

	_, err = b.page.Eval(fmt.Sprintf("window.scrollTo({top: %f, behavior: 'smooth'})", scrollY))
	if err != nil {
		return element.ScrollIntoView()
	}

	// Wait for smooth scroll to complete
	time.Sleep(500 * time.Millisecond)
	return nil
}

// WaitForPageStable waits for page to be fully stable (no DOM changes)
func (b *Browser) WaitForPageStable(timeout time.Duration) error {
	start := time.Now()
	var lastHTML string

	for {
		if time.Since(start) > timeout {
			return nil // Timeout reached, assume stable
		}

		// Get current page HTML signature
		html, err := b.page.HTML()
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		// Compare with previous
		if html == lastHTML {
			return nil // Page is stable
		}

		lastHTML = html
		time.Sleep(300 * time.Millisecond)
	}
}

// GetElementText safely extracts text from an element
func (b *Browser) GetElementText(selector string) (string, error) {
	element, err := b.page.Timeout(5 * time.Second).Element(selector)
	if err != nil {
		return "", err
	}
	return element.Text()
}

// GetElementAttribute gets an attribute value from an element
func (b *Browser) GetElementAttribute(selector, attribute string) (string, error) {
	element, err := b.page.Timeout(5 * time.Second).Element(selector)
	if err != nil {
		return "", err
	}
	value, err := element.Attribute(attribute)
	if err != nil {
		return "", err
	}
	if value == nil {
		return "", nil
	}
	return *value, nil
}

// CountElements returns the number of elements matching a selector
func (b *Browser) CountElements(selector string) int {
	elements, err := b.page.Timeout(3 * time.Second).Elements(selector)
	if err != nil {
		return 0
	}
	return len(elements)
}

// WaitForElementCount waits until a specific number of elements exist
func (b *Browser) WaitForElementCount(selector string, count int, timeout time.Duration) error {
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for %d elements", count)
		}
		if b.CountElements(selector) >= count {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// ClickByText finds and clicks an element containing specific text
func (b *Browser) ClickByText(text string, tag string) error {
	if tag == "" {
		tag = "*"
	}

	// XPath to find element containing text
	xpath := fmt.Sprintf("//%s[contains(text(), '%s')]", tag, text)
	element, err := b.page.Timeout(5 * time.Second).ElementX(xpath)
	if err != nil {
		// Try with normalize-space for trimmed text
		xpath = fmt.Sprintf("//%s[contains(normalize-space(.), '%s')]", tag, text)
		element, err = b.page.Timeout(3 * time.Second).ElementX(xpath)
		if err != nil {
			return fmt.Errorf("element with text '%s' not found", text)
		}
	}

	// Wait for visibility
	if err := element.WaitVisible(); err != nil {
		return err
	}

	// Scroll and click
	if err := b.scroller.ScrollToElement(b.page, element); err != nil {
		return err
	}

	b.timer.ShortDelay()
	return b.mouseMover.MoveToClick(b.page, element)
}

// WaitForURL waits for the page URL to contain a specific substring
func (b *Browser) WaitForURL(substring string, timeout time.Duration) error {
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for URL containing '%s'", substring)
		}

		info, err := b.page.Info()
		if err == nil && info != nil {
			if contains(info.URL, substring) {
				return nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// contains helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// EnsureElementClickable makes sure element is in viewport and clickable
func (b *Browser) EnsureElementClickable(element *rod.Element) error {
	// Wait for visibility
	if err := element.WaitVisible(); err != nil {
		return fmt.Errorf("element not visible: %w", err)
	}

	// Wait for interactability
	_, err := element.WaitInteractable()
	if err != nil {
		return fmt.Errorf("element not interactable: %w", err)
	}

	// Wait for stability
	if err := element.WaitStable(300 * time.Millisecond); err != nil {
		return fmt.Errorf("element not stable: %w", err)
	}

	return nil
}

// ClickElementDirect clicks an element directly without selector lookup
func (b *Browser) ClickElementDirect(element *rod.Element) error {
	// Ensure clickable
	if err := b.EnsureElementClickable(element); err != nil {
		return err
	}

	// Scroll to element
	if err := b.ScrollIntoViewCenter(element); err != nil {
		return err
	}

	b.timer.ShortDelay()
	return b.mouseMover.MoveToClick(b.page, element)
}

// GetAllElementsText returns text content of all matching elements
func (b *Browser) GetAllElementsText(selector string) ([]string, error) {
	elements, err := b.page.Timeout(5 * time.Second).Elements(selector)
	if err != nil {
		return nil, err
	}

	var texts []string
	for _, el := range elements {
		text, err := el.Text()
		if err == nil && text != "" {
			texts = append(texts, text)
		}
	}
	return texts, nil
}

// RefreshPage refreshes the current page and waits for load
func (b *Browser) RefreshPage() error {
	if err := b.page.Reload(); err != nil {
		return err
	}
	if err := b.page.WaitLoad(); err != nil {
		return err
	}
	b.WaitForNetworkIdle(3 * time.Second)
	b.timer.PageLoadWait()
	return nil
}

// GetCurrentURL returns the current page URL
func (b *Browser) GetCurrentURL() string {
	info, err := b.page.Info()
	if err != nil {
		return ""
	}
	return info.URL
}
