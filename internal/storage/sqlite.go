// Package storage provides SQLite-based state persistence for the LinkedIn automation tool.
// It tracks sent requests, accepted connections, and message history using SQLite.
package storage

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Store provides database operations for state persistence
type Store struct {
	db *sql.DB
}

// NewStore creates a new Store and initializes the database
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &Store{db: db}

	// Initialize tables
	if err := store.initTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize tables: %w", err)
	}

	return store, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// initTables creates the required database tables
func (s *Store) initTables() error {
	queries := []string{
		// Profiles table
		`CREATE TABLE IF NOT EXISTS profiles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url TEXT UNIQUE NOT NULL,
			name TEXT,
			title TEXT,
			company TEXT,
			location TEXT,
			industry TEXT,
			connection_status TEXT DEFAULT 'none',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		// Connection requests table
		`CREATE TABLE IF NOT EXISTS connection_requests (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			profile_id INTEGER NOT NULL,
			status TEXT DEFAULT 'pending',
			note TEXT,
			sent_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			accepted_at TIMESTAMP,
			FOREIGN KEY (profile_id) REFERENCES profiles(id)
		)`,

		// Messages table
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			profile_id INTEGER NOT NULL,
			content TEXT NOT NULL,
			template_id TEXT,
			status TEXT DEFAULT 'sent',
			sent_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (profile_id) REFERENCES profiles(id)
		)`,

		// Sessions table
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			cookies TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMP,
			is_valid BOOLEAN DEFAULT TRUE
		)`,

		// Activity log table
		`CREATE TABLE IF NOT EXISTS activity_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action_type TEXT NOT NULL,
			details TEXT,
			success BOOLEAN DEFAULT TRUE,
			error_message TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		// Indices for faster queries
		`CREATE INDEX IF NOT EXISTS idx_profiles_url ON profiles(url)`,
		`CREATE INDEX IF NOT EXISTS idx_profiles_status ON profiles(connection_status)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_requests_status ON connection_requests(status)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_profile ON messages(profile_id)`,
	}

	for _, query := range queries {
		if _, err := s.db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}
	}

	return nil
}

// Profile represents a LinkedIn profile
type Profile struct {
	ID               int64
	URL              string
	Name             string
	Title            string
	Company          string
	Location         string
	Industry         string
	ConnectionStatus string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ConnectionRequest represents a sent connection request
type ConnectionRequest struct {
	ID         int64
	ProfileID  int64
	Status     string // pending, accepted, ignored
	Note       string
	SentAt     time.Time
	AcceptedAt *time.Time
}

// Message represents a sent message
type Message struct {
	ID         int64
	ProfileID  int64
	Content    string
	TemplateID string
	Status     string
	SentAt     time.Time
}

// SaveProfile saves or updates a profile
func (s *Store) SaveProfile(profile *Profile) error {
	query := `
		INSERT INTO profiles (url, name, title, company, location, industry, connection_status)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(url) DO UPDATE SET
			name = excluded.name,
			title = excluded.title,
			company = excluded.company,
			location = excluded.location,
			industry = excluded.industry,
			updated_at = CURRENT_TIMESTAMP
	`

	result, err := s.db.Exec(query,
		profile.URL, profile.Name, profile.Title, profile.Company,
		profile.Location, profile.Industry, profile.ConnectionStatus)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err == nil && id > 0 {
		profile.ID = id
	}

	return nil
}

// GetProfileByURL retrieves a profile by URL
func (s *Store) GetProfileByURL(url string) (*Profile, error) {
	query := `
		SELECT id, url, name, title, company, location, industry, connection_status, created_at, updated_at
		FROM profiles WHERE url = ?
	`

	profile := &Profile{}
	err := s.db.QueryRow(query, url).Scan(
		&profile.ID, &profile.URL, &profile.Name, &profile.Title,
		&profile.Company, &profile.Location, &profile.Industry,
		&profile.ConnectionStatus, &profile.CreatedAt, &profile.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return profile, nil
}

// GetProfilesForConnection retrieves profiles that haven't been sent connection requests
func (s *Store) GetProfilesForConnection(limit int) ([]*Profile, error) {
	query := `
		SELECT id, url, name, title, company, location, industry, connection_status, created_at, updated_at
		FROM profiles
		WHERE connection_status = 'none'
		ORDER BY created_at ASC
		LIMIT ?
	`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []*Profile
	for rows.Next() {
		profile := &Profile{}
		err := rows.Scan(
			&profile.ID, &profile.URL, &profile.Name, &profile.Title,
			&profile.Company, &profile.Location, &profile.Industry,
			&profile.ConnectionStatus, &profile.CreatedAt, &profile.UpdatedAt)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}

	return profiles, nil
}

// UpdateProfileStatus updates the connection status of a profile
func (s *Store) UpdateProfileStatus(profileID int64, status string) error {
	query := `UPDATE profiles SET connection_status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := s.db.Exec(query, status, profileID)
	return err
}

// SaveConnectionRequest saves a connection request
func (s *Store) SaveConnectionRequest(request *ConnectionRequest) error {
	query := `
		INSERT INTO connection_requests (profile_id, status, note, sent_at)
		VALUES (?, ?, ?, ?)
	`

	result, err := s.db.Exec(query, request.ProfileID, request.Status, request.Note, request.SentAt)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err == nil {
		request.ID = id
	}

	return nil
}

// GetPendingConnectionRequests retrieves pending connection requests
func (s *Store) GetPendingConnectionRequests() ([]*ConnectionRequest, error) {
	query := `
		SELECT id, profile_id, status, note, sent_at, accepted_at
		FROM connection_requests
		WHERE status = 'pending'
		ORDER BY sent_at ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requests []*ConnectionRequest
	for rows.Next() {
		req := &ConnectionRequest{}
		var acceptedAt sql.NullTime
		err := rows.Scan(&req.ID, &req.ProfileID, &req.Status, &req.Note, &req.SentAt, &acceptedAt)
		if err != nil {
			return nil, err
		}
		if acceptedAt.Valid {
			req.AcceptedAt = &acceptedAt.Time
		}
		requests = append(requests, req)
	}

	return requests, nil
}

// MarkConnectionAccepted marks a connection request as accepted
func (s *Store) MarkConnectionAccepted(requestID int64) error {
	query := `
		UPDATE connection_requests
		SET status = 'accepted', accepted_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := s.db.Exec(query, requestID)
	return err
}

// SaveMessage saves a sent message
func (s *Store) SaveMessage(message *Message) error {
	query := `
		INSERT INTO messages (profile_id, content, template_id, status, sent_at)
		VALUES (?, ?, ?, ?, ?)
	`

	result, err := s.db.Exec(query,
		message.ProfileID, message.Content, message.TemplateID,
		message.Status, message.SentAt)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err == nil {
		message.ID = id
	}

	return nil
}

// GetMessagesByProfile retrieves messages sent to a profile
func (s *Store) GetMessagesByProfile(profileID int64) ([]*Message, error) {
	query := `
		SELECT id, profile_id, content, template_id, status, sent_at
		FROM messages
		WHERE profile_id = ?
		ORDER BY sent_at DESC
	`

	rows, err := s.db.Query(query, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		msg := &Message{}
		err := rows.Scan(&msg.ID, &msg.ProfileID, &msg.Content, &msg.TemplateID, &msg.Status, &msg.SentAt)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

// HasMessagedProfile checks if a message has been sent to a profile
func (s *Store) HasMessagedProfile(profileID int64) (bool, error) {
	query := `SELECT COUNT(*) FROM messages WHERE profile_id = ?`
	var count int
	err := s.db.QueryRow(query, profileID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetTodayConnectionCount returns the number of connections sent today
func (s *Store) GetTodayConnectionCount() (int, error) {
	query := `
		SELECT COUNT(*)
		FROM connection_requests
		WHERE date(sent_at) = date('now')
	`
	var count int
	err := s.db.QueryRow(query).Scan(&count)
	return count, err
}

// GetTodayMessageCount returns the number of messages sent today
func (s *Store) GetTodayMessageCount() (int, error) {
	query := `
		SELECT COUNT(*)
		FROM messages
		WHERE date(sent_at) = date('now')
	`
	var count int
	err := s.db.QueryRow(query).Scan(&count)
	return count, err
}

// LogActivity logs an activity
func (s *Store) LogActivity(actionType string, details string, success bool, errorMsg string) error {
	query := `
		INSERT INTO activity_log (action_type, details, success, error_message)
		VALUES (?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, actionType, details, success, errorMsg)
	return err
}

// GetRecentActivity retrieves recent activity logs
func (s *Store) GetRecentActivity(limit int) ([]ActivityLog, error) {
	query := `
		SELECT id, action_type, details, success, error_message, created_at
		FROM activity_log
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []ActivityLog
	for rows.Next() {
		var log ActivityLog
		var errorMsg sql.NullString
		err := rows.Scan(&log.ID, &log.ActionType, &log.Details, &log.Success, &errorMsg, &log.CreatedAt)
		if err != nil {
			return nil, err
		}
		if errorMsg.Valid {
			log.ErrorMessage = errorMsg.String
		}
		logs = append(logs, log)
	}

	return logs, nil
}

// ActivityLog represents an activity log entry
type ActivityLog struct {
	ID           int64
	ActionType   string
	Details      string
	Success      bool
	ErrorMessage string
	CreatedAt    time.Time
}

// GetProfileCount returns the total number of profiles
func (s *Store) GetProfileCount() (int, error) {
	query := `SELECT COUNT(*) FROM profiles`
	var count int
	err := s.db.QueryRow(query).Scan(&count)
	return count, err
}

// IsDuplicateProfile checks if a profile URL already exists
func (s *Store) IsDuplicateProfile(url string) (bool, error) {
	profile, err := s.GetProfileByURL(url)
	if err != nil {
		return false, err
	}
	return profile != nil, nil
}

// GetAcceptedConnections retrieves profiles with accepted connections that haven't been messaged
func (s *Store) GetAcceptedConnections() ([]*Profile, error) {
	query := `
		SELECT p.id, p.url, p.name, p.title, p.company, p.location, p.industry, p.connection_status, p.created_at, p.updated_at
		FROM profiles p
		INNER JOIN connection_requests cr ON p.id = cr.profile_id
		WHERE cr.status = 'accepted'
		AND p.id NOT IN (SELECT profile_id FROM messages)
		ORDER BY cr.accepted_at ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []*Profile
	for rows.Next() {
		profile := &Profile{}
		err := rows.Scan(
			&profile.ID, &profile.URL, &profile.Name, &profile.Title,
			&profile.Company, &profile.Location, &profile.Industry,
			&profile.ConnectionStatus, &profile.CreatedAt, &profile.UpdatedAt)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}

	return profiles, nil
}
