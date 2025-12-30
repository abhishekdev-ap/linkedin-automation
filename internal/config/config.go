// Package config provides configuration management for the LinkedIn automation tool.
// It supports YAML configuration files, environment variable overrides, and sensible defaults.
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config holds all configuration for the application
type Config struct {
	Browser    BrowserConfig    `mapstructure:"browser"`
	Stealth    StealthConfig    `mapstructure:"stealth"`
	Limits     LimitsConfig     `mapstructure:"limits"`
	Scheduling SchedulingConfig `mapstructure:"scheduling"`
	Storage    StorageConfig    `mapstructure:"storage"`
	Logging    LoggingConfig    `mapstructure:"logging"`
	Templates  TemplatesConfig  `mapstructure:"templates"`

	// Environment variables
	LinkedInEmail    string
	LinkedInPassword string
	Debug            bool
}

// BrowserConfig holds browser-related settings
type BrowserConfig struct {
	Headless    bool     `mapstructure:"headless"`
	SlowMotion  int      `mapstructure:"slow_motion"`
	DevTools    bool     `mapstructure:"devtools"`
	UserDataDir string   `mapstructure:"user_data_dir"`
	Viewport    Viewport `mapstructure:"viewport"`
	UserAgents  []string `mapstructure:"user_agents"`
}

// Viewport defines browser window dimensions
type Viewport struct {
	Width  int `mapstructure:"width"`
	Height int `mapstructure:"height"`
}

// StealthConfig holds anti-detection settings
type StealthConfig struct {
	Mouse  MouseConfig  `mapstructure:"mouse"`
	Timing TimingConfig `mapstructure:"timing"`
	Scroll ScrollConfig `mapstructure:"scroll"`
	Typing TypingConfig `mapstructure:"typing"`
}

// MouseConfig configures human-like mouse movement
type MouseConfig struct {
	MinSpeed     float64 `mapstructure:"min_speed"`
	MaxSpeed     float64 `mapstructure:"max_speed"`
	Overshoot    bool    `mapstructure:"overshoot"`
	MicroMoves   bool    `mapstructure:"micro_moves"`
	BezierPoints int     `mapstructure:"bezier_points"`
}

// TimingConfig configures randomized timing patterns
type TimingConfig struct {
	MinDelay     int       `mapstructure:"min_delay"`
	MaxDelay     int       `mapstructure:"max_delay"`
	ThinkTime    TimeRange `mapstructure:"think_time"`
	PageLoadWait TimeRange `mapstructure:"page_load_wait"`
}

// TimeRange represents a min-max time range
type TimeRange struct {
	Min int `mapstructure:"min"`
	Max int `mapstructure:"max"`
}

// ScrollConfig configures natural scrolling behavior
type ScrollConfig struct {
	MinSpeed           int     `mapstructure:"min_speed"`
	MaxSpeed           int     `mapstructure:"max_speed"`
	PauseProbability   float64 `mapstructure:"pause_probability"`
	ReverseProbability float64 `mapstructure:"reverse_probability"`
}

// TypingConfig configures realistic typing simulation
type TypingConfig struct {
	MinInterval     int     `mapstructure:"min_interval"`
	MaxInterval     int     `mapstructure:"max_interval"`
	TypoProbability float64 `mapstructure:"typo_probability"`
	CorrectionDelay int     `mapstructure:"correction_delay"`
}

// LimitsConfig holds rate limiting settings
type LimitsConfig struct {
	Connections ActionLimits `mapstructure:"connections"`
	Messages    ActionLimits `mapstructure:"messages"`
	Searches    SearchLimits `mapstructure:"searches"`
}

// ActionLimits defines rate limits for an action type
type ActionLimits struct {
	Daily    int `mapstructure:"daily"`
	Hourly   int `mapstructure:"hourly"`
	Cooldown int `mapstructure:"cooldown"`
}

// SearchLimits defines search-specific limits
type SearchLimits struct {
	Daily          int `mapstructure:"daily"`
	PagesPerSearch int `mapstructure:"pages_per_search"`
}

// SchedulingConfig holds activity scheduling settings
type SchedulingConfig struct {
	Enabled   bool        `mapstructure:"enabled"`
	Timezone  string      `mapstructure:"timezone"`
	WorkHours WorkHours   `mapstructure:"work_hours"`
	Breaks    BreakConfig `mapstructure:"breaks"`
}

// WorkHours defines operating hours
type WorkHours struct {
	Start int `mapstructure:"start"`
	End   int `mapstructure:"end"`
}

// BreakConfig defines break patterns
type BreakConfig struct {
	Enabled     bool    `mapstructure:"enabled"`
	MinDuration int     `mapstructure:"min_duration"`
	MaxDuration int     `mapstructure:"max_duration"`
	Probability float64 `mapstructure:"probability"`
}

// StorageConfig holds database settings
type StorageConfig struct {
	Type   string       `mapstructure:"type"`
	Path   string       `mapstructure:"path"`
	Backup BackupConfig `mapstructure:"backup"`
}

// BackupConfig defines backup settings
type BackupConfig struct {
	Enabled    bool `mapstructure:"enabled"`
	Interval   int  `mapstructure:"interval"`
	MaxBackups int  `mapstructure:"max_backups"`
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level  string     `mapstructure:"level"`
	Format string     `mapstructure:"format"`
	Output string     `mapstructure:"output"`
	File   FileConfig `mapstructure:"file"`
}

// FileConfig defines file logging settings
type FileConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Path       string `mapstructure:"path"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
}

// TemplatesConfig holds message templates
type TemplatesConfig struct {
	ConnectionNote  string `mapstructure:"connection_note"`
	FollowupMessage string `mapstructure:"followup_message"`
}

// Load loads configuration from file and environment
func Load(configPath string) (*Config, error) {
	// Load .env file if it exists
	_ = godotenv.Load()

	// Set up viper
	v := viper.New()
	v.SetConfigType("yaml")

	// Set defaults
	setDefaults(v)

	// Load config file if provided
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	} else {
		// Try default locations
		v.SetConfigName("config")
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
		v.AddConfigPath("$HOME/.linkedin-automation")

		// Ignore error if no config file found (use defaults)
		_ = v.ReadInConfig()
	}

	// Parse config
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Load environment variables
	cfg.LinkedInEmail = os.Getenv("LINKEDIN_EMAIL")
	cfg.LinkedInPassword = os.Getenv("LINKEDIN_PASSWORD")
	cfg.Debug = os.Getenv("DEBUG") == "true"

	// Override headless from env if set
	if headless := os.Getenv("HEADLESS"); headless != "" {
		cfg.Browser.Headless = headless == "true"
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.LinkedInEmail == "" {
		return fmt.Errorf("LINKEDIN_EMAIL is required")
	}
	if c.LinkedInPassword == "" {
		return fmt.Errorf("LINKEDIN_PASSWORD is required")
	}
	if c.Browser.Viewport.Width <= 0 || c.Browser.Viewport.Height <= 0 {
		return fmt.Errorf("viewport dimensions must be positive")
	}
	if c.Limits.Connections.Daily <= 0 {
		return fmt.Errorf("daily connection limit must be positive")
	}
	return nil
}

// GetTimezone returns the configured timezone as a *time.Location
func (c *Config) GetTimezone() (*time.Location, error) {
	return time.LoadLocation(c.Scheduling.Timezone)
}

// IsWithinWorkHours checks if the current time is within configured work hours
func (c *Config) IsWithinWorkHours() (bool, error) {
	if !c.Scheduling.Enabled {
		return true, nil
	}

	loc, err := c.GetTimezone()
	if err != nil {
		return false, err
	}

	now := time.Now().In(loc)
	hour := now.Hour()

	return hour >= c.Scheduling.WorkHours.Start && hour < c.Scheduling.WorkHours.End, nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// Browser defaults
	v.SetDefault("browser.headless", false)
	v.SetDefault("browser.slow_motion", 0)
	v.SetDefault("browser.devtools", false)
	v.SetDefault("browser.viewport.width", 1920)
	v.SetDefault("browser.viewport.height", 1080)
	v.SetDefault("browser.user_agents", []string{
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	})

	// Stealth defaults
	v.SetDefault("stealth.mouse.min_speed", 0.5)
	v.SetDefault("stealth.mouse.max_speed", 1.5)
	v.SetDefault("stealth.mouse.overshoot", true)
	v.SetDefault("stealth.mouse.micro_moves", true)
	v.SetDefault("stealth.mouse.bezier_points", 4)

	v.SetDefault("stealth.timing.min_delay", 500)
	v.SetDefault("stealth.timing.max_delay", 3000)
	v.SetDefault("stealth.timing.think_time.min", 1000)
	v.SetDefault("stealth.timing.think_time.max", 5000)
	v.SetDefault("stealth.timing.page_load_wait.min", 2000)
	v.SetDefault("stealth.timing.page_load_wait.max", 5000)

	v.SetDefault("stealth.scroll.min_speed", 100)
	v.SetDefault("stealth.scroll.max_speed", 500)
	v.SetDefault("stealth.scroll.pause_probability", 0.3)
	v.SetDefault("stealth.scroll.reverse_probability", 0.1)

	v.SetDefault("stealth.typing.min_interval", 50)
	v.SetDefault("stealth.typing.max_interval", 200)
	v.SetDefault("stealth.typing.typo_probability", 0.02)
	v.SetDefault("stealth.typing.correction_delay", 500)

	// Limits defaults
	v.SetDefault("limits.connections.daily", 50)
	v.SetDefault("limits.connections.hourly", 10)
	v.SetDefault("limits.connections.cooldown", 60)
	v.SetDefault("limits.messages.daily", 100)
	v.SetDefault("limits.messages.hourly", 20)
	v.SetDefault("limits.messages.cooldown", 30)
	v.SetDefault("limits.searches.daily", 100)
	v.SetDefault("limits.searches.pages_per_search", 10)

	// Scheduling defaults
	v.SetDefault("scheduling.enabled", true)
	v.SetDefault("scheduling.timezone", "America/New_York")
	v.SetDefault("scheduling.work_hours.start", 9)
	v.SetDefault("scheduling.work_hours.end", 18)
	v.SetDefault("scheduling.breaks.enabled", true)
	v.SetDefault("scheduling.breaks.min_duration", 10)
	v.SetDefault("scheduling.breaks.max_duration", 30)
	v.SetDefault("scheduling.breaks.probability", 0.2)

	// Storage defaults
	v.SetDefault("storage.type", "sqlite")
	v.SetDefault("storage.path", "linkedin_automation.db")
	v.SetDefault("storage.backup.enabled", true)
	v.SetDefault("storage.backup.interval", 3600)
	v.SetDefault("storage.backup.max_backups", 5)

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")
	v.SetDefault("logging.file.enabled", false)
	v.SetDefault("logging.file.path", "logs/linkedin-bot.log")
	v.SetDefault("logging.file.max_size", 100)
	v.SetDefault("logging.file.max_backups", 3)
	v.SetDefault("logging.file.max_age", 28)
}
