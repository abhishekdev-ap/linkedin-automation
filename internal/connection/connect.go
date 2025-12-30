// Package connection provides LinkedIn connection request functionality.
// It handles navigating to profiles, clicking Connect, sending personalized notes,
// and tracking sent requests with daily limits.
package connection

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/linkedin-automation/linkedin-bot/internal/stealth"
	"github.com/linkedin-automation/linkedin-bot/internal/storage"
	"github.com/linkedin-automation/linkedin-bot/pkg/browser"
)

// Selectors for connection functionality - Updated with multiple fallback options
const (
	// Connect button selectors (try multiple patterns)
	ConnectButtonSelector       = "button[aria-label*='connect' i], button[aria-label*='Connect' i], button.pvs-profile-actions__action[aria-label*='connect' i]"
	ConnectButtonAltSelector    = "div.pvs-profile-actions button:has-text('Connect'), button.artdeco-button--primary:has-text('Connect')"
	ConnectButtonSearchSelector = "button.search-result__actions--primary, li button[aria-label*='connect' i]"

	// Modal selectors
	AddNoteButtonSelector   = "button[aria-label='Add a note'], button:has-text('Add a note')"
	NoteTextareaSelector    = "textarea[name='message'], textarea#custom-message, textarea.connect-button-send-invite__custom-message"
	SendButtonSelector      = "button[aria-label='Send now'], button[aria-label='Send invitation'], button[aria-label='Send'], button.artdeco-button--primary:has-text('Send')"
	SendWithoutNoteSelector = "button[aria-label='Send without a note'], button:has-text('Send without a note')"

	// Status selectors
	SuccessModalSelector   = ".artdeco-modal--layer-default, .artdeco-modal"
	DismissButtonSelector  = "button[aria-label='Dismiss'], button[aria-label='Got it'], button.artdeco-modal__dismiss"
	PendingButtonSelector  = "button[aria-label*='Pending' i], span:has-text('Pending')"
	MoreButtonSelector     = "button[aria-label='More actions'], button.artdeco-dropdown__trigger"
	WithdrawButtonSelector = "button[aria-label='Withdraw invitation']"
)

// Note character limits
const (
	MaxNoteLength = 300
)

// Connector handles LinkedIn connection requests
type Connector struct {
	browser     *browser.Browser
	store       *storage.Store
	rateLimiter *stealth.RateLimiter
	timer       *stealth.Timer
	dailyLimit  int
}

// NewConnector creates a new Connector
func NewConnector(b *browser.Browser, store *storage.Store, rateLimiter *stealth.RateLimiter, dailyLimit int) *Connector {
	return &Connector{
		browser:     b,
		store:       store,
		rateLimiter: rateLimiter,
		timer:       b.GetTimer(),
		dailyLimit:  dailyLimit,
	}
}

// ConnectionResult represents the result of a connection attempt
type ConnectionResult struct {
	Success          bool
	AlreadyConnected bool
	PendingRequest   bool
	Error            error
	Message          string
}

// Connect sends a connection request to a profile
func (c *Connector) Connect(profileURL string, note string) (*ConnectionResult, error) {
	// Check rate limits
	canConnect, reason := c.rateLimiter.CanPerformAction(stealth.ActionConnection)
	if !canConnect {
		return &ConnectionResult{
			Success: false,
			Error:   fmt.Errorf("rate limit reached: %s", reason),
			Message: reason,
		}, nil
	}

	// Check daily limit from database
	todayCount, err := c.store.GetTodayConnectionCount()
	if err == nil && todayCount >= c.dailyLimit {
		return &ConnectionResult{
			Success: false,
			Error:   fmt.Errorf("daily limit reached: %d/%d", todayCount, c.dailyLimit),
			Message: fmt.Sprintf("Daily limit reached (%d/%d)", todayCount, c.dailyLimit),
		}, nil
	}

	// Get profile from database
	profile, err := c.store.GetProfileByURL(profileURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get profile: %w", err)
	}

	// Navigate to profile
	if err := c.browser.Navigate(profileURL); err != nil {
		return nil, fmt.Errorf("failed to navigate to profile: %w", err)
	}

	page := c.browser.GetPage()
	c.timer.ThinkTime()

	// Simulate reading the profile
	scroller := c.browser.GetScroller()
	_ = scroller.RandomScroll(page)
	c.timer.ThinkTime()

	// Check if already connected or pending
	result := c.checkConnectionStatus(page)
	if result != nil {
		return result, nil
	}

	// Find and click Connect button
	if err := c.clickConnectButton(page); err != nil {
		return &ConnectionResult{
			Success: false,
			Error:   err,
			Message: "Failed to find Connect button",
		}, nil
	}

	c.timer.ShortDelay()

	// Add note if provided
	if note != "" {
		if err := c.addConnectionNote(page, note); err != nil {
			// Note failed, try to send without note
			_ = c.sendWithoutNote(page)
		}
	} else {
		// Send without note
		if err := c.sendWithoutNote(page); err != nil {
			return &ConnectionResult{
				Success: false,
				Error:   err,
				Message: "Failed to send connection request",
			}, nil
		}
	}

	c.timer.ShortDelay()

	// Dismiss any modal
	_ = c.dismissModal(page)

	// Record the connection request
	c.rateLimiter.RecordAction(stealth.ActionConnection)

	// Save to database
	request := &storage.ConnectionRequest{
		ProfileID: profile.ID,
		Status:    "pending",
		Note:      note,
		SentAt:    time.Now(),
	}
	if err := c.store.SaveConnectionRequest(request); err != nil {
		// Log but don't fail
	}

	// Update profile status
	_ = c.store.UpdateProfileStatus(profile.ID, "pending")

	// Log activity
	_ = c.store.LogActivity("connection_request", profileURL, true, "")

	return &ConnectionResult{
		Success: true,
		Message: "Connection request sent successfully",
	}, nil
}

// checkConnectionStatus checks if already connected or pending
func (c *Connector) checkConnectionStatus(page *rod.Page) *ConnectionResult {
	// Check for pending button
	pendingBtn, err := page.Timeout(2 * time.Second).Element(PendingButtonSelector)
	if err == nil && pendingBtn != nil {
		return &ConnectionResult{
			Success:        false,
			PendingRequest: true,
			Message:        "Connection request already pending",
		}
	}

	// Check for "Message" button which indicates connected
	messageBtn, err := page.Timeout(1 * time.Second).Element("button[aria-label='Message']")
	if err == nil && messageBtn != nil {
		// Check if they're connected by looking for specific indicators
		// If we can message them directly, they're likely connected
		return &ConnectionResult{
			Success:          false,
			AlreadyConnected: true,
			Message:          "Already connected",
		}
	}

	return nil
}

// clickConnectButton finds and clicks the Connect button with retry logic
func (c *Connector) clickConnectButton(page *rod.Page) error {
	mouseMover := c.browser.GetMouseMover()

	// List of selectors to try in order
	connectSelectors := []string{
		ConnectButtonSelector,
		ConnectButtonAltSelector,
		ConnectButtonSearchSelector,
		"button[aria-label*='Invite'][aria-label*='connect' i]",
		"button.artdeco-button--primary span:has-text('Connect')",
	}

	// Try each selector
	for _, selector := range connectSelectors {
		connectBtn, err := page.Timeout(2 * time.Second).Element(selector)
		if err == nil && connectBtn != nil {
			// Check if button is visible
			visible, _ := connectBtn.Visible()
			if visible {
				// Wait for button to be stable
				if err := connectBtn.WaitStable(500 * time.Millisecond); err == nil {
					c.timer.ShortDelay()
					if err := mouseMover.MoveToClick(page, connectBtn); err == nil {
						return nil
					}
				}
			}
		}
	}

	// Last resort: Try clicking More button first
	moreBtn, moreErr := page.Timeout(2 * time.Second).Element(MoreButtonSelector)
	if moreErr == nil && moreBtn != nil {
		_ = mouseMover.MoveToClick(page, moreBtn)
		c.timer.ShortDelay()

		// Look for Connect in dropdown with multiple selectors
		dropdownSelectors := []string{
			"div[role='menuitem'] span:has-text('Connect')",
			"li[role='menuitem'] span:has-text('Connect')",
			"div.artdeco-dropdown__content button:has-text('Connect')",
		}

		for _, selector := range dropdownSelectors {
			connectBtn, err := page.Timeout(2 * time.Second).Element(selector)
			if err == nil && connectBtn != nil {
				c.timer.ShortDelay()
				if err := mouseMover.MoveToClick(page, connectBtn); err == nil {
					return nil
				}
			}
		}
	}

	return fmt.Errorf("Connect button not found after trying all selectors")
}

// addConnectionNote adds a personalized note to the connection request
func (c *Connector) addConnectionNote(page *rod.Page, note string) error {
	// Click "Add a note" button
	addNoteBtn, err := page.Timeout(3 * time.Second).Element(AddNoteButtonSelector)
	if err != nil {
		return fmt.Errorf("Add note button not found: %w", err)
	}

	mouseMover := c.browser.GetMouseMover()
	if err := mouseMover.MoveToClick(page, addNoteBtn); err != nil {
		return err
	}

	c.timer.ShortDelay()

	// Find textarea
	textarea, err := page.Element(NoteTextareaSelector)
	if err != nil {
		return fmt.Errorf("Note textarea not found: %w", err)
	}

	// Truncate note if too long
	if len(note) > MaxNoteLength {
		note = note[:MaxNoteLength-3] + "..."
	}

	// Type the note with human-like behavior
	typer := c.browser.GetTyper()
	c.timer.BeforeType()
	if err := typer.TypeInElement(textarea, note); err != nil {
		return err
	}

	c.timer.ShortDelay()

	// Click Send
	sendBtn, err := page.Element(SendButtonSelector)
	if err != nil {
		return fmt.Errorf("Send button not found: %w", err)
	}

	return mouseMover.MoveToClick(page, sendBtn)
}

// sendWithoutNote sends connection request without a note
func (c *Connector) sendWithoutNote(page *rod.Page) error {
	// Try multiple selectors for Send button
	selectors := []string{
		SendButtonSelector,
		SendWithoutNoteSelector,
		"button[aria-label='Send without a note']",
		"button.artdeco-button--primary",
	}

	var sendBtn *rod.Element
	var err error

	for _, selector := range selectors {
		sendBtn, err = page.Timeout(2 * time.Second).Element(selector)
		if err == nil && sendBtn != nil {
			break
		}
	}

	if sendBtn == nil {
		return fmt.Errorf("Send button not found after trying all selectors")
	}

	mouseMover := c.browser.GetMouseMover()
	return mouseMover.MoveToClick(page, sendBtn)
}

// dismissModal dismisses any success modal
func (c *Connector) dismissModal(page *rod.Page) error {
	dismissBtn, err := page.Timeout(2 * time.Second).Element(DismissButtonSelector)
	if err != nil {
		return nil // No modal to dismiss
	}

	mouseMover := c.browser.GetMouseMover()
	return mouseMover.MoveToClick(page, dismissBtn)
}

// ConnectBatch sends connection requests to multiple profiles
func (c *Connector) ConnectBatch(profiles []*storage.Profile, noteTemplate string) ([]ConnectionResult, error) {
	var results []ConnectionResult

	for _, profile := range profiles {
		// Check if we should continue
		canConnect, reason := c.rateLimiter.CanPerformAction(stealth.ActionConnection)
		if !canConnect {
			results = append(results, ConnectionResult{
				Success: false,
				Message: reason,
			})
			break
		}

		// Personalize note
		note := c.personalizeNote(noteTemplate, profile)

		// Send connection
		result, err := c.Connect(profile.URL, note)
		if err != nil {
			results = append(results, ConnectionResult{
				Success: false,
				Error:   err,
				Message: err.Error(),
			})
		} else {
			results = append(results, *result)
		}

		// Wait before next request
		c.rateLimiter.WaitForCooldown(stealth.ActionConnection)
		c.timer.BetweenActions()

		// Occasionally take a break
		if c.timer.ShouldTakeBreak() {
			c.timer.ShortBreak()
		}
	}

	return results, nil
}

// personalizeNote replaces template variables in the note
func (c *Connector) personalizeNote(template string, profile *storage.Profile) string {
	if template == "" {
		return ""
	}

	note := template

	// Get first name
	firstName := profile.Name
	if idx := strings.Index(firstName, " "); idx > 0 {
		firstName = firstName[:idx]
	}

	// Replace template variables
	note = strings.ReplaceAll(note, "{{.FirstName}}", firstName)
	note = strings.ReplaceAll(note, "{{.Name}}", profile.Name)
	note = strings.ReplaceAll(note, "{{.Title}}", profile.Title)
	note = strings.ReplaceAll(note, "{{.Company}}", profile.Company)
	note = strings.ReplaceAll(note, "{{.Location}}", profile.Location)
	note = strings.ReplaceAll(note, "{{.Industry}}", profile.Industry)

	return note
}

// GetRemainingQuota returns remaining connection quota
func (c *Connector) GetRemainingQuota() (daily, hourly int) {
	return c.rateLimiter.GetRemainingQuota(stealth.ActionConnection)
}

// GetTodayCount returns connections sent today
func (c *Connector) GetTodayCount() (int, error) {
	return c.store.GetTodayConnectionCount()
}

// GetPendingRequests returns pending connection requests
func (c *Connector) GetPendingRequests() ([]*storage.ConnectionRequest, error) {
	return c.store.GetPendingConnectionRequests()
}
