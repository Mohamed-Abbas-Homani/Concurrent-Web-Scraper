package test

import (
	"testing"
	"time"

	"webscraper/internal/scraper"
	"webscraper/pkg/types"
)

func TestWebScraper_FullFlow(t *testing.T) {
	cfg := &types.ScraperConfig{
		MaxConcurrency: 3,
		RateLimit:      10,
		Timeout:        10 * time.Second,
		RetryAttempts:  2,
		RetryDelay:     time.Second,
		CacheTTL:       5 * time.Minute,
		MaxBodySize:    0,
		UserAgent:      "WebScraperTest/1.0",
	}

	ws := scraper.NewWebScraper(cfg)

	if err := ws.Start(); err != nil {
		t.Fatalf("Failed to start scraper: %v", err)
	}
	defer ws.Stop()

	task := &types.ScrapingTask{
		ID:     "test-scrape-quotes",
		URL:    "http://quotes.toscrape.com",
		Method: "GET",
		Selectors: map[string]string{
			"quote":  ".quote .text",
			"author": ".quote .author",
		},
	}

	if err := ws.AddTask(task); err != nil {
		t.Fatalf("Failed to add task: %v", err)
	}

	// Wait for result with timeout
	var result *types.ScrapingResult
	const timeout = 20 * time.Second
	start := time.Now()
	for time.Since(start) < timeout {
		results := ws.GetResultsByIds([]string{task.ID})
		if len(results) == 1 {
			result = results[0]
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if result == nil {
		t.Fatalf("Expected 1 result, got 0")
	}
	if result.Error != "" {
		t.Fatalf("Scraping failed: %s", result.Error)
	}
	if len(result.Data) == 0 {
		t.Fatal("Expected non-empty result data")
	}

	t.Logf("✅ Scraping success: %+v", result.Data)
}
