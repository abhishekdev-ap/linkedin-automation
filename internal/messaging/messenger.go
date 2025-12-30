// Package messaging provides LinkedIn messaging functionality.
// It handles detecting accepted connections, sending follow-up messages,
// template support with dynamic variables, and message tracking.
package messaging

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/linkedin-automation/linkedin-bot/internal/stealth"
	"github.com/linkedin-automation/linkedin-bot/internal/storage"
	"github.com/linkedin-automation/linkedin-bot/pkg/browser"
)

// Selectors for messaging
const (
	MessageButtonSelector     = "button[aria-label='Message']"
	MessageInputSelector      = "div.msg-form__contenteditable"
	SendMessageSelector       = "button.msg-form__send-button"
	MessageModalSelector      = ".msg-overlay-conversation-bubble"
	CloseMessageSelector      = "button[aria-label='Close your conversation']"
	MessageListSelector       = ".msg-s-message-list-container"
)

// Messenger handles LinkedIn messaging
type Messenger struct {
	browser     *browser.Browser
	store       *storage.Store
	rateLimiter *stealth.RateLimiter
	timer       *stealth.Timer
	templates   map[string]*template.Template
}

// NewMessenger creates a new Messenger
func NewMessenger(b *browser.Browser, store *storage.Store, rateLimiter *stealth.RateLimiter) *Messenger {
	return &Messenger{
		browser:     b,
		store:       store,
		rateLimiter: rateLimiter,
		timer:       b.GetTimer(),
		templates:   make(map[string]*template.Template),
	}
}

// MessageResult represents the result of a message attempt
type MessageResult struct {
	Success bool
	Error   error
	Message string
}

// RegisterTemplate registers a message template
func (m *Messenger) RegisterTemplate(name, templateText string) error {
	tmpl, err := template.New(name).Parse(templateText)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}
	m.templates[name] = tmpl
	return nil
}

// TemplateData holds data for template rendering
type TemplateData struct {
	FirstName  string
	LastName   string
	Name       string
	Title      string
	Company    string
	Location   string
	Industry   string
	SenderName string
}

// SendMessage sends a message to a profile
func (m *Messenger) SendMessage(profileURL string, message string) (*MessageResult, error) {
	// Check rate limits
	canMessage, reason := m.rateLimiter.CanPerformAction(stealth.ActionMessage)
	if !canMessage {
		return &MessageResult{
			Success: false,
			Error:   fmt.Errorf("rate limit reached: %s", reason),
			Message: reason,
		}, nil
	}

	// Get profile from database
	profile, err := m.store.GetProfileByURL(profileURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get profile: %w", err)
	}

	// Check if already messaged
	hasMessaged, err := m.store.HasMessagedProfile(profile.ID)
	if err == nil && hasMessaged {
		return &MessageResult{
			Success: false,
			Message: "Already messaged this profile",
		}, nil
	}

	// Navigate to profile
	if err := m.browser.Navigate(profileURL); err != nil {
		return nil, fmt.Errorf("failed to navigate to profile: %w", err)
	}

	page := m.browser.GetPage()
	m.timer.ThinkTime()

	// Click Message button
	if err := m.clickMessageButton(page); err != nil {
		return &MessageResult{
			Success: false,
			Error:   err,
			Message: "Failed to open message dialog",
		}, nil
	}

	m.timer.ShortDelay()

	// Type the message
	if err := m.typeMessage(page, message); err != nil {
		return &MessageResult{
			Success: false,
			Error:   err,
			Message: "Failed to type message",
		}, nil
	}

	m.timer.ShortDelay()

	// Send the message
	if err := m.sendMessage(page); err != nil {
		return &MessageResult{
			Success: false,
			Error:   err,
			Message: "Failed to send message",
		}, nil
	}

	// Record the message
	m.rateLimiter.RecordAction(stealth.ActionMessage)

	// Save to database
	msg := &storage.Message{
		ProfileID: profile.ID,
		Content:   message,
		Status:    "sent",
		SentAt:    time.Now(),
	}
	if err := m.store.SaveMessage(msg); err != nil {
		// Log but don't fail
	}

	// Log activity
	_ = m.store.LogActivity("message_sent", profileURL, true, "")

	// Close message dialog
	_ = m.closeMessageDialog(page)

	return &MessageResult{
		Success: true,
		Message: "Message sent successfully",
	}, nil
}

// clickMessageButton finds and clicks the Message button
func (m *Messenger) clickMessageButton(page *rod.Page) error {
	messageBtn, err := page.Timeout(5 * time.Second).Element(MessageButtonSelector)
	if err != nil {
		return fmt.Errorf("Message button not found: %w", err)
	}

	mouseMover := m.browser.GetMouseMover()
	return mouseMover.MoveToClick(page, messageBtn)
}

// typeMessage types the message in the input field
func (m *Messenger) typeMessage(page *rod.Page, message string) error {
	// Wait for message input
	input, err := page.Timeout(5 * time.Second).Element(MessageInputSelector)
	if err != nil {
		return fmt.Errorf("Message input not found: %w", err)
	}

	// Focus the input
	if err := input.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return err
	}

	m.timer.BeforeType()

	// Type with human-like behavior
	typer := m.browser.GetTyper()
	return typer.TypeWithVariation(page, message)
}

// sendMessage clicks the send button
func (m *Messenger) sendMessage(page *rod.Page) error {
	sendBtn, err := page.Timeout(3 * time.Second).Element(SendMessageSelector)
	if err != nil {
		return fmt.Errorf("Send button not found: %w", err)
	}

	// Check if button is enabled
	disabled, _ := sendBtn.Attribute("disabled")
	if disabled != nil {
		return fmt.Errorf("Send button is disabled")
	}

	mouseMover := m.browser.GetMouseMover()
	return mouseMover.MoveToClick(page, sendBtn)
}

// closeMessageDialog closes the message dialog
func (m *Messenger) closeMessageDialog(page *rod.Page) error {
	closeBtn, err := page.Timeout(2 * time.Second).Element(CloseMessageSelector)
	if err != nil {
		return nil // No dialog to close
	}

	mouseMover := m.browser.GetMouseMover()
	return mouseMover.MoveToClick(page, closeBtn)
}

// SendMessageWithTemplate sends a message using a registered template
func (m *Messenger) SendMessageWithTemplate(profileURL string, templateName string, data TemplateData) (*MessageResult, error) {
	tmpl, ok := m.templates[templateName]
	if !ok {
		return nil, fmt.Errorf("template not found: %s", templateName)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	return m.SendMessage(profileURL, buf.String())
}

// MessageAcceptedConnections sends messages to all newly accepted connections
func (m *Messenger) MessageAcceptedConnections(message string) ([]MessageResult, error) {
	// Get accepted connections that haven't been messaged
	connections, err := m.store.GetAcceptedConnections()
	if err != nil {
		return nil, fmt.Errorf("failed to get accepted connections: %w", err)
	}

	var results []MessageResult

	for _, profile := range connections {
		// Check rate limits
		canMessage, reason := m.rateLimiter.CanPerformAction(stealth.ActionMessage)
		if !canMessage {
			results = append(results, MessageResult{
				Success: false,
				Message: reason,
			})
			break
		}

		// Personalize message
		personalizedMessage := m.personalizeMessage(message, profile)

		// Send message
		result, err := m.SendMessage(profile.URL, personalizedMessage)
		if err != nil {
			results = append(results, MessageResult{
				Success: false,
				Error:   err,
				Message: err.Error(),
			})
		} else {
			results = append(results, *result)
		}

		// Wait between messages
		m.rateLimiter.WaitForCooldown(stealth.ActionMessage)
		m.timer.BetweenActions()
	}

	return results, nil
}

// personalizeMessage replaces template variables in the message
func (m *Messenger) personalizeMessage(message string, profile *storage.Profile) string {
	// Get first and last name
	firstName := profile.Name
	lastName := ""
	if idx := strings.Index(firstName, " "); idx > 0 {
		lastName = firstName[idx+1:]
		firstName = firstName[:idx]
	}

	// Replace template variables
	result := message
	result = strings.ReplaceAll(result, "{{.FirstName}}", firstName)
	result = strings.ReplaceAll(result, "{{.LastName}}", lastName)
	result = strings.ReplaceAll(result, "{{.Name}}", profile.Name)
	result = strings.ReplaceAll(result, "{{.Title}}", profile.Title)
	result = strings.ReplaceAll(result, "{{.Company}}", profile.Company)
	result = strings.ReplaceAll(result, "{{.Location}}", profile.Location)
	result = strings.ReplaceAll(result, "{{.Industry}}", profile.Industry)

	return result
}

// GetMessageHistory returns messages sent to a profile
func (m *Messenger) GetMessageHistory(profileID int64) ([]*storage.Message, error) {
	return m.store.GetMessagesByProfile(profileID)
}

// GetTodayCount returns messages sent today
func (m *Messenger) GetTodayCount() (int, error) {
	return m.store.GetTodayMessageCount()
}

// GetRemainingQuota returns remaining message quota
func (m *Messenger) GetRemainingQuota() (daily, hourly int) {
	return m.rateLimiter.GetRemainingQuota(stealth.ActionMessage)
}

// DefaultFollowUpTemplate returns a default follow-up message template
func DefaultFollowUpTemplate() string {
	return `Hi {{.FirstName}},

Thank you for connecting! I noticed you're working as {{.Title}} {{if .Company}}at {{.Company}}{{end}} and thought it would be great to exchange ideas sometime.

Would you be open to a brief chat?

Best regards!`
}
