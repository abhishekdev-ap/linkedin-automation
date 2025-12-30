// Package auth provides LinkedIn authentication functionality.
// It handles login with credentials, detection of login failures and security checkpoints,
// and session cookie persistence for seamless reuse.
package auth

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/linkedin-automation/linkedin-bot/pkg/browser"
)

// LinkedIn URLs
const (
	LoginURL     = "https://www.linkedin.com/login"
	FeedURL      = "https://www.linkedin.com/feed/"
	CheckpointURL = "https://www.linkedin.com/checkpoint/"
)

// Selectors for login page
const (
	EmailSelector    = "#username"
	PasswordSelector = "#password"
	SubmitSelector   = "button[type='submit']"
	ErrorSelector    = "#error-for-username, #error-for-password, .form__label--error"
	CaptchaSelector  = ".captcha, #captcha, [data-test-id='captcha']"
	TwoFASelector    = "#input__phone_verification_pin, #input__email_verification_pin"
)

// Authenticator handles LinkedIn authentication
type Authenticator struct {
	browser      *browser.Browser
	sessionFile  string
	isLoggedIn   bool
}

// NewAuthenticator creates a new Authenticator
func NewAuthenticator(b *browser.Browser, sessionFile string) *Authenticator {
	return &Authenticator{
		browser:     b,
		sessionFile: sessionFile,
	}
}

// LoginResult represents the result of a login attempt
type LoginResult struct {
	Success      bool
	Error        error
	Requires2FA  bool
	RequiresCaptcha bool
	Message      string
}

// Login attempts to log in to LinkedIn
func (a *Authenticator) Login(email, password string) (*LoginResult, error) {
	// First try to restore session
	if a.sessionFile != "" {
		if err := a.browser.LoadCookies(a.sessionFile); err == nil {
			// Check if session is still valid
			if valid, _ := a.isSessionValid(); valid {
				a.isLoggedIn = true
				return &LoginResult{
					Success: true,
					Message: "Session restored from cookies",
				}, nil
			}
		}
	}

	// Navigate to login page
	if err := a.browser.Navigate(LoginURL); err != nil {
		return nil, fmt.Errorf("failed to navigate to login page: %w", err)
	}

	page := a.browser.GetPage()
	timer := a.browser.GetTimer()

	// Check if already logged in
	if a.checkAlreadyLoggedIn(page) {
		a.isLoggedIn = true
		a.saveSession()
		return &LoginResult{
			Success: true,
			Message: "Already logged in",
		}, nil
	}

	// Enter email
	timer.ThinkTime()
	if err := a.browser.Type(EmailSelector, email); err != nil {
		return nil, fmt.Errorf("failed to enter email: %w", err)
	}

	// Enter password
	timer.BetweenActions()
	if err := a.browser.Type(PasswordSelector, password); err != nil {
		return nil, fmt.Errorf("failed to enter password: %w", err)
	}

	// Click submit
	timer.BetweenActions()
	if err := a.browser.Click(SubmitSelector); err != nil {
		return nil, fmt.Errorf("failed to click submit: %w", err)
	}

	// Wait for navigation/response
	timer.PageLoadWait()

	// Check for various outcomes
	return a.checkLoginOutcome(page)
}

// checkLoginOutcome determines the result of login attempt
func (a *Authenticator) checkLoginOutcome(page *rod.Page) (*LoginResult, error) {
	// Wait a bit for the page to settle
	time.Sleep(2 * time.Second)

	currentURL := page.MustInfo().URL

	// Check if redirected to feed (success)
	if strings.Contains(currentURL, "/feed") {
		a.isLoggedIn = true
		a.saveSession()
		return &LoginResult{
			Success: true,
			Message: "Login successful",
		}, nil
	}

	// Check for checkpoint (2FA or security challenge)
	if strings.Contains(currentURL, "/checkpoint") {
		// Check for 2FA
		twoFAElement, err := page.Timeout(2 * time.Second).Element(TwoFASelector)
		if err == nil && twoFAElement != nil {
			return &LoginResult{
				Success:     false,
				Requires2FA: true,
				Message:     "Two-factor authentication required",
			}, nil
		}

		// Check for captcha
		captchaElement, err := page.Timeout(2 * time.Second).Element(CaptchaSelector)
		if err == nil && captchaElement != nil {
			return &LoginResult{
				Success:         false,
				RequiresCaptcha: true,
				Message:         "CAPTCHA verification required",
			}, nil
		}

		return &LoginResult{
			Success: false,
			Message: "Security checkpoint detected - manual verification required",
		}, nil
	}

	// Check for error messages
	errorElement, err := page.Timeout(2 * time.Second).Element(ErrorSelector)
	if err == nil && errorElement != nil {
		errorText, _ := errorElement.Text()
		return &LoginResult{
			Success: false,
			Error:   fmt.Errorf("login failed: %s", errorText),
			Message: errorText,
		}, nil
	}

	// Still on login page - probably failed
	if strings.Contains(currentURL, "/login") {
		return &LoginResult{
			Success: false,
			Error:   fmt.Errorf("login failed - still on login page"),
			Message: "Login failed - credentials may be incorrect",
		}, nil
	}

	// Unknown state
	return &LoginResult{
		Success: false,
		Message: fmt.Sprintf("Unknown login state - current URL: %s", currentURL),
	}, nil
}

// checkAlreadyLoggedIn checks if already logged in
func (a *Authenticator) checkAlreadyLoggedIn(page *rod.Page) bool {
	currentURL := page.MustInfo().URL
	return strings.Contains(currentURL, "/feed") || strings.Contains(currentURL, "/mynetwork")
}

// isSessionValid checks if the current session is valid
func (a *Authenticator) isSessionValid() (bool, error) {
	// Navigate to feed
	if err := a.browser.Navigate(FeedURL); err != nil {
		return false, err
	}

	page := a.browser.GetPage()
	time.Sleep(2 * time.Second)

	currentURL := page.MustInfo().URL

	// If redirected to login, session is invalid
	if strings.Contains(currentURL, "/login") {
		return false, nil
	}

	// If on feed, session is valid
	return strings.Contains(currentURL, "/feed"), nil
}

// saveSession saves the current session cookies
func (a *Authenticator) saveSession() {
	if a.sessionFile != "" {
		_ = a.browser.SaveCookies(a.sessionFile)
	}
}

// Handle2FA handles two-factor authentication
func (a *Authenticator) Handle2FA(code string) (*LoginResult, error) {
	page := a.browser.GetPage()
	timer := a.browser.GetTimer()

	// Find 2FA input
	twoFAElement, err := page.Element(TwoFASelector)
	if err != nil {
		return nil, fmt.Errorf("2FA input not found: %w", err)
	}

	// Enter code with human-like typing
	timer.ThinkTime()
	typer := a.browser.GetTyper()
	if err := typer.TypeInElement(twoFAElement, code); err != nil {
		return nil, fmt.Errorf("failed to enter 2FA code: %w", err)
	}

	// Submit
	timer.BetweenActions()
	submitBtn, err := page.Element("button[type='submit']")
	if err != nil {
		return nil, fmt.Errorf("submit button not found: %w", err)
	}

	if err := submitBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return nil, fmt.Errorf("failed to submit 2FA: %w", err)
	}

	timer.PageLoadWait()

	return a.checkLoginOutcome(page)
}

// IsLoggedIn returns whether currently logged in
func (a *Authenticator) IsLoggedIn() bool {
	return a.isLoggedIn
}

// Logout logs out of LinkedIn
func (a *Authenticator) Logout() error {
	if err := a.browser.Navigate("https://www.linkedin.com/m/logout/"); err != nil {
		return err
	}
	a.isLoggedIn = false
	return nil
}

// RefreshSession refreshes the session by navigating to feed
func (a *Authenticator) RefreshSession() error {
	if err := a.browser.Navigate(FeedURL); err != nil {
		return err
	}

	page := a.browser.GetPage()
	time.Sleep(2 * time.Second)

	currentURL := page.MustInfo().URL
	if strings.Contains(currentURL, "/login") {
		a.isLoggedIn = false
		return fmt.Errorf("session expired")
	}

	a.saveSession()
	return nil
}
