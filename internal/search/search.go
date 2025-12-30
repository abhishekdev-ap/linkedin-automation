// Package search provides LinkedIn search functionality.
// It handles searching users by criteria, parsing profile URLs,
// pagination, and duplicate detection.
package search

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/linkedin-automation/linkedin-bot/internal/stealth"
	"github.com/linkedin-automation/linkedin-bot/internal/storage"
	"github.com/linkedin-automation/linkedin-bot/pkg/browser"
)

// LinkedIn Search URLs
const (
	SearchPeopleURL = "https://www.linkedin.com/search/results/people/"
)

// Selectors for search page - Updated with multiple fallback options
const (
	// Primary selectors
	SearchResultsSelector   = ".search-results-container"
	ProfileCardSelector     = ".entity-result, .reusable-search__result-container, li.reusable-search__result-container"
	ProfileLinkSelector     = ".entity-result__title-text a, .app-aware-link, a[data-control-name='search_srp_result']"
	ProfileNameSelector     = ".entity-result__title-text a span[aria-hidden='true'], .entity-result__title-text span[dir='ltr'], span.entity-result__title-line"
	ProfileTitleSelector    = ".entity-result__primary-subtitle, .entity-result__summary, .linked-area span[dir='ltr']"
	ProfileLocationSelector = ".entity-result__secondary-subtitle, .entity-result__location"
	NextPageSelector        = "button[aria-label='Next'], button.artdeco-pagination__button--next"
	NoResultsSelector       = ".search-reusable-search-no-results, .search-no-results"

	// Connect button selectors from search results
	SearchConnectBtnSelector = "button[aria-label*='connect' i], button.search-result__action-button"
)

// SearchCriteria defines search parameters
type SearchCriteria struct {
	Title    string
	Company  string
	Location string
	Keywords string
	Industry string
}

// Searcher handles LinkedIn profile search
type Searcher struct {
	browser     *browser.Browser
	store       *storage.Store
	rateLimiter *stealth.RateLimiter
	timer       *stealth.Timer
}

// NewSearcher creates a new Searcher
func NewSearcher(b *browser.Browser, store *storage.Store, rateLimiter *stealth.RateLimiter) *Searcher {
	return &Searcher{
		browser:     b,
		store:       store,
		rateLimiter: rateLimiter,
		timer:       b.GetTimer(),
	}
}

// SearchResult represents a single search result
type SearchResult struct {
	ProfileURL string
	Name       string
	Title      string
	Location   string
	Company    string
}

// Search searches for profiles matching the criteria
func (s *Searcher) Search(criteria SearchCriteria, maxPages int) ([]SearchResult, error) {
	// Check rate limits
	canSearch, reason := s.rateLimiter.CanPerformAction(stealth.ActionSearch)
	if !canSearch {
		return nil, fmt.Errorf("rate limit reached: %s", reason)
	}

	// Build search URL
	searchURL := s.buildSearchURL(criteria)

	// Navigate to search
	if err := s.browser.Navigate(searchURL); err != nil {
		return nil, fmt.Errorf("failed to navigate to search: %w", err)
	}

	s.rateLimiter.RecordAction(stealth.ActionSearch)

	var results []SearchResult

	// Paginate through results
	for page := 1; page <= maxPages; page++ {
		pageResults, err := s.parseSearchResults()
		if err != nil {
			// Log but continue
			continue
		}

		results = append(results, pageResults...)

		// Check if there's a next page
		if !s.hasNextPage() {
			break
		}

		// Navigate to next page
		if err := s.navigateNextPage(); err != nil {
			break
		}

		// Record search action for rate limiting
		s.rateLimiter.WaitForCooldown(stealth.ActionSearch)
		s.rateLimiter.RecordAction(stealth.ActionSearch)
	}

	return results, nil
}

// buildSearchURL constructs the LinkedIn search URL with filters
func (s *Searcher) buildSearchURL(criteria SearchCriteria) string {
	params := url.Values{}

	// Add keywords
	var keywords []string
	if criteria.Keywords != "" {
		keywords = append(keywords, criteria.Keywords)
	}
	if criteria.Title != "" {
		keywords = append(keywords, criteria.Title)
	}
	if len(keywords) > 0 {
		params.Set("keywords", strings.Join(keywords, " "))
	}

	// Add company filter
	if criteria.Company != "" {
		// Note: LinkedIn uses company IDs, this is a simplified approach
		params.Set("company", criteria.Company)
	}

	// Add location filter
	if criteria.Location != "" {
		params.Set("geoUrn", criteria.Location)
	}

	// Origin parameter
	params.Set("origin", "GLOBAL_SEARCH_HEADER")

	return SearchPeopleURL + "?" + params.Encode()
}

// parseSearchResults extracts profile information from search results
func (s *Searcher) parseSearchResults() ([]SearchResult, error) {
	page := s.browser.GetPage()

	// Wait for results to load
	_, err := page.Timeout(10 * time.Second).Element(SearchResultsSelector)
	if err != nil {
		// Check for no results
		noResults, _ := page.Element(NoResultsSelector)
		if noResults != nil {
			return nil, nil
		}
		return nil, fmt.Errorf("search results not found: %w", err)
	}

	// Find all profile cards
	cards, err := page.Elements(ProfileCardSelector)
	if err != nil {
		return nil, fmt.Errorf("failed to find profile cards: %w", err)
	}

	var results []SearchResult

	for _, card := range cards {
		result, err := s.parseProfileCard(card)
		if err != nil {
			continue // Skip invalid cards
		}

		// Check for duplicates
		isDuplicate, _ := s.store.IsDuplicateProfile(result.ProfileURL)
		if isDuplicate {
			continue
		}

		results = append(results, *result)

		// Save to database
		profile := &storage.Profile{
			URL:              result.ProfileURL,
			Name:             result.Name,
			Title:            result.Title,
			Location:         result.Location,
			Company:          result.Company,
			ConnectionStatus: "none",
		}
		_ = s.store.SaveProfile(profile)
	}

	return results, nil
}

// parseProfileCard extracts information from a single profile card
func (s *Searcher) parseProfileCard(card *rod.Element) (*SearchResult, error) {
	result := &SearchResult{}

	// Get profile link
	linkElement, err := card.Element(ProfileLinkSelector)
	if err != nil {
		return nil, fmt.Errorf("profile link not found")
	}

	href, err := linkElement.Attribute("href")
	if err != nil || href == nil {
		return nil, fmt.Errorf("profile href not found")
	}

	result.ProfileURL = normalizeProfileURL(*href)

	// Get name
	nameElement, err := card.Element(ProfileNameSelector)
	if err == nil {
		result.Name, _ = nameElement.Text()
		result.Name = strings.TrimSpace(result.Name)
	}

	// Get title
	titleElement, err := card.Element(ProfileTitleSelector)
	if err == nil {
		result.Title, _ = titleElement.Text()
		result.Title = strings.TrimSpace(result.Title)
		// Extract company from title if present
		result.Company = extractCompanyFromTitle(result.Title)
	}

	// Get location
	locationElement, err := card.Element(ProfileLocationSelector)
	if err == nil {
		result.Location, _ = locationElement.Text()
		result.Location = strings.TrimSpace(result.Location)
	}

	return result, nil
}

// normalizeProfileURL normalizes a LinkedIn profile URL
func normalizeProfileURL(rawURL string) string {
	// Parse the URL
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	// Extract just the path (remove query params)
	path := parsed.Path

	// Normalize the path
	if strings.HasPrefix(path, "/in/") {
		// Extract username
		parts := strings.Split(path, "/")
		if len(parts) >= 3 {
			return "https://www.linkedin.com/in/" + parts[2] + "/"
		}
	}

	return "https://www.linkedin.com" + path
}

// extractCompanyFromTitle extracts company name from title string
func extractCompanyFromTitle(title string) string {
	// Title format is usually "Title at Company"
	if strings.Contains(title, " at ") {
		parts := strings.Split(title, " at ")
		if len(parts) >= 2 {
			return strings.TrimSpace(parts[1])
		}
	}
	return ""
}

// hasNextPage checks if there's a next page of results
func (s *Searcher) hasNextPage() bool {
	page := s.browser.GetPage()
	nextBtn, err := page.Element(NextPageSelector)
	if err != nil {
		return false
	}

	// Check if button is disabled
	disabled, _ := nextBtn.Attribute("disabled")
	return disabled == nil
}

// navigateNextPage clicks the next page button
func (s *Searcher) navigateNextPage() error {
	// Simulate thinking/reading before navigating
	s.timer.ThinkTime()

	// Scroll down to load more content and find next button
	scroller := s.browser.GetScroller()
	if err := scroller.ScrollDown(s.browser.GetPage()); err != nil {
		return err
	}

	s.timer.ShortDelay()

	// Click next page
	if err := s.browser.Click(NextPageSelector); err != nil {
		return fmt.Errorf("failed to click next page: %w", err)
	}

	// Wait for page load
	s.timer.PageLoadWait()

	return nil
}

// GetCollectedProfiles returns profiles collected from the database
func (s *Searcher) GetCollectedProfiles(limit int) ([]*storage.Profile, error) {
	return s.store.GetProfilesForConnection(limit)
}

// GetProfileCount returns the total number of collected profiles
func (s *Searcher) GetProfileCount() (int, error) {
	return s.store.GetProfileCount()
}

// Profile URL regex for validation
var profileURLRegex = regexp.MustCompile(`linkedin\.com/in/([a-zA-Z0-9\-]+)`)

// ValidateProfileURL validates if a URL is a valid LinkedIn profile URL
func ValidateProfileURL(url string) bool {
	return profileURLRegex.MatchString(url)
}

// ExtractUsername extracts the username from a LinkedIn profile URL
func ExtractUsername(profileURL string) string {
	matches := profileURLRegex.FindStringSubmatch(profileURL)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}
