package types

import (
	"time"
)

// ScrapingTask represents a web scraping task
type ScrapingTask struct {
	URL       string            `json:"url"`
	Selectors map[string]string `json:"selectors"`
	ID        string            `json:"id"`
	Priority  int               `json:"priority"`
	Headers   map[string]string `json:"headers,omitempty"`
	Method    string            `json:"method,omitempty"`
}

// ScrapingResult represents the result of a scraping operation
type ScrapingResult struct {
	TaskID     string         `json:"task_id"`
	URL        string         `json:"url"`
	Data       map[string]any `json:"data"`
	Timestamp  time.Time      `json:"timestamp"`
	Duration   time.Duration  `json:"duration"`
	Error      string         `json:"error,omitempty"`
	StatusCode int            `json:"status_code,omitempty"`
	FromCache  bool           `json:"from_cache,omitempty"`
}

// CacheEntry represents a cached scraping result
type CacheEntry struct {
	Data      *ScrapingResult
	ExpiresAt time.Time
}

// ScraperConfig holds configuration for the web scraper
type ScraperConfig struct {
	MaxConcurrency   int           `json:"max_concurrency"`
	RateLimit        float64       `json:"rate_limit"` // requests per second
	Timeout          time.Duration `json:"timeout"`
	RetryAttempts    int           `json:"retry_attempts"`
	RetryDelay       time.Duration `json:"retry_delay"`
	CacheTTL         time.Duration `json:"cache_ttl"`
	UserAgent        string        `json:"user_agent"`
	RespectRobotsTxt bool          `json:"respect_robots_txt"`
	MaxBodySize      int64         `json:"max_body_size"` // in bytes
}

// ResultProcessor interface for handling scraping results
type ResultProcessor interface {
	Process(result *ScrapingResult) error
}

// Scraper interface defines the main scraper contract
type Scraper interface {
	Start() error
	Stop() error
	AddTask(task *ScrapingTask) error
	GetResults() <-chan *ScrapingResult
}
