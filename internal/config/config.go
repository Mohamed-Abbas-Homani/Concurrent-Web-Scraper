package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
	"webscraper/pkg/types"
)

// DefaultConfig returns a default configuration
func DefaultConfig() *types.ScraperConfig {
	return &types.ScraperConfig{
		MaxConcurrency:   5,
		RateLimit:        2.0,
		Timeout:          30 * time.Second,
		RetryAttempts:    3,
		RetryDelay:       2 * time.Second,
		CacheTTL:         10 * time.Minute,
		UserAgent:        "WebScraper/1.0 (+https://example.com/bot)",
		RespectRobotsTxt: true,
		MaxBodySize:      10 * 1024 * 1024, // 10MB
	}
}

// LoadConfig loads configuration from a JSON file
func LoadConfig(filepath string) (*types.ScraperConfig, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config types.ScraperConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	return &config, nil
}

// SaveConfig saves configuration to a JSON file
func SaveConfig(config *types.ScraperConfig, filepath string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// ValidateConfig validates the configuration
func ValidateConfig(config *types.ScraperConfig) error {
	if config.MaxConcurrency <= 0 {
		return fmt.Errorf("max_concurrency must be positive")
	}
	if config.RateLimit <= 0 {
		return fmt.Errorf("rate_limit must be positive")
	}
	if config.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	if config.RetryAttempts < 0 {
		return fmt.Errorf("retry_attempts must be non-negative")
	}
	if config.CacheTTL <= 0 {
		return fmt.Errorf("cache_ttl must be positive")
	}
	if config.UserAgent == "" {
		return fmt.Errorf("user_agent cannot be empty")
	}
	if config.MaxBodySize <= 0 {
		return fmt.Errorf("max_body_size must be positive")
	}
	return nil
}
