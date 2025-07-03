package scraper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"webscraper/internal/cache"
	"webscraper/internal/ratelimit"
	"webscraper/pkg/types"

	"github.com/PuerkitoBio/goquery"
	"github.com/sirupsen/logrus"
)

// WebScraper implements the main scraping functionality
type WebScraper struct {
	config      *types.ScraperConfig
	cache       *cache.Cache
	rateLimiter *ratelimit.RateLimiter
	httpClient  *http.Client
	logger      *logrus.Logger
	taskQueue   chan *types.ScrapingTask
	results     sync.Map
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	stats       ScraperStats
	statsMutex  sync.RWMutex
}

// ScraperStats holds scraper performance statistics
type ScraperStats struct {
	TasksProcessed  int64         `json:"tasks_processed"`
	TasksSuccessful int64         `json:"tasks_successful"`
	TasksFailed     int64         `json:"tasks_failed"`
	TotalDuration   time.Duration `json:"total_duration"`
	AverageDuration time.Duration `json:"average_duration"`
	BytesDownloaded int64         `json:"bytes_downloaded"`
}

// NewWebScraper creates a new web scraper instance
func NewWebScraper(config *types.ScraperConfig) *WebScraper {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339,
	})

	ctx, cancel := context.WithCancel(context.Background())

	scraper := &WebScraper{
		config:      config,
		cache:       cache.NewCache(logger),
		rateLimiter: ratelimit.NewRateLimiter(config.RateLimit, logger),
		httpClient: &http.Client{
			Timeout: config.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  false,
			},
		},
		logger:    logger,
		taskQueue: make(chan *types.ScrapingTask, config.MaxConcurrency*2),
		ctx:       ctx,
		cancel:    cancel,
		stats:     ScraperStats{},
	}

	return scraper
}

// Start initializes and starts the scraper workers
func (ws *WebScraper) Start() error {
	ws.logger.Info("Starting web scraper")

	// Start worker goroutines
	for i := range ws.config.MaxConcurrency {
		ws.wg.Add(1)
		go ws.worker(i)
	}

	ws.logger.WithField("workers", ws.config.MaxConcurrency).Info("Workers started")
	return nil
}

// Stop gracefully shuts down the scraper
func (ws *WebScraper) Stop() error {
	ws.logger.Info("Stopping web scraper")
	ws.cancel()
	close(ws.taskQueue)
	ws.wg.Wait()
	ws.logger.Info("Web scraper stopped")
	return nil
}

// AddTask adds a new scraping task to the queue
func (ws *WebScraper) AddTask(task *types.ScrapingTask) error {
	// Set default method if not specified
	if task.Method == "" {
		task.Method = "GET"
	}

	select {
	case ws.taskQueue <- task:
		ws.logger.WithFields(logrus.Fields{
			"task_id": task.ID,
			"url":     task.URL,
		}).Info("Task added to queue")
		return nil
	case <-ws.ctx.Done():
		return fmt.Errorf("scraper is shutting down")
	default:
		return fmt.Errorf("task queue is full")
	}
}

// GetResultsByIds returns the results by ids
func (ws *WebScraper) GetResultsByIds(taskIds []string) []*types.ScrapingResult {
	results := make([]*types.ScrapingResult, 0, len(taskIds))

	for _, id := range taskIds {
		if val, ok := ws.results.Load(id); ok {
			if result, ok := val.(*types.ScrapingResult); ok {
				results = append(results, result)
			}
		}
	}

	return results
}

// GetStats returns scraper statistics
func (ws *WebScraper) GetStats() ScraperStats {
	ws.statsMutex.RLock()
	defer ws.statsMutex.RUnlock()

	stats := ws.stats
	if stats.TasksProcessed > 0 {
		stats.AverageDuration = stats.TotalDuration / time.Duration(stats.TasksProcessed)
	}
	return stats
}

// worker processes tasks from the task queue
func (ws *WebScraper) worker(workerID int) {
	defer ws.wg.Done()

	logger := ws.logger.WithField("worker_id", workerID)
	logger.Info("Worker started")

	for {
		select {
		case task, ok := <-ws.taskQueue:
			if !ok {
				logger.Info("Worker shutting down")
				return
			}

			result := ws.processTask(task, logger)

			// Store result in concurrent map
			ws.results.Store(result.TaskID, result)

		case <-ws.ctx.Done():
			logger.Info("Worker interrupted")
			return
		}
	}
}

// processTask handles a single scraping task
func (ws *WebScraper) processTask(task *types.ScrapingTask, logger *logrus.Entry) *types.ScrapingResult {
	start := time.Now()

	logger = logger.WithFields(logrus.Fields{
		"task_id": task.ID,
		"url":     task.URL,
	})

	// Update statistics
	ws.updateStats(func(stats *ScraperStats) {
		stats.TasksProcessed++
	})

	// Check cache first
	cacheKey := ws.cache.GenerateKey(task)
	if cached, found := ws.cache.Get(cacheKey); found {
		logger.Info("Cache hit")
		cached.TaskID = task.ID // Update task ID for response
		return cached
	}

	// Rate limiting
	if err := ws.rateLimiter.Wait(ws.ctx); err != nil {
		ws.updateStats(func(stats *ScraperStats) {
			stats.TasksFailed++
		})
		return &types.ScrapingResult{
			TaskID:    task.ID,
			URL:       task.URL,
			Timestamp: time.Now(),
			Duration:  time.Since(start),
			Error:     fmt.Sprintf("Rate limiting error: %v", err),
		}
	}

	// Scrape with retry logic
	var result *types.ScrapingResult
	for attempt := 0; attempt <= ws.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			retryDelay := ws.config.RetryDelay * time.Duration(attempt)
			logger.WithFields(logrus.Fields{
				"attempt": attempt,
				"delay":   retryDelay,
			}).Info("Retrying scraping")

			select {
			case <-time.After(retryDelay):
			case <-ws.ctx.Done():
				result = &types.ScrapingResult{
					TaskID:    task.ID,
					URL:       task.URL,
					Timestamp: time.Now(),
					Duration:  time.Since(start),
					Error:     "Context cancelled during retry",
				}
				break
			}
		}

		result = ws.scrapeURL(task, logger)
		if result.Error == "" {
			break
		}

		if attempt == ws.config.RetryAttempts {
			logger.WithError(fmt.Errorf("%s", result.Error)).Error("Max retry attempts reached")
		}
	}

	result.Duration = time.Since(start)

	// Update statistics
	ws.updateStats(func(stats *ScraperStats) {
		stats.TotalDuration += result.Duration
		if result.Error == "" {
			stats.TasksSuccessful++
		} else {
			stats.TasksFailed++
		}
	})

	// Cache successful results
	if result.Error == "" {
		ws.cache.Set(cacheKey, result, ws.config.CacheTTL)
	}

	return result
}

// scrapeURL performs the actual HTTP request and HTML parsing
func (ws *WebScraper) scrapeURL(task *types.ScrapingTask, logger *logrus.Entry) *types.ScrapingResult {
	result := &types.ScrapingResult{
		TaskID:    task.ID,
		URL:       task.URL,
		Data:      make(map[string]interface{}),
		Timestamp: time.Now(),
	}

	// Validate URL
	parsedURL, err := url.Parse(task.URL)
	if err != nil {
		result.Error = fmt.Sprintf("Invalid URL: %v", err)
		return result
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ws.ctx, task.Method, task.URL, nil)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to create request: %v", err)
		return result
	}

	// Set headers
	req.Header.Set("User-Agent", ws.config.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("DNT", "1")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	// Add custom headers
	for key, value := range task.Headers {
		req.Header.Set(key, value)
	}

	// Make HTTP request
	resp, err := ws.httpClient.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("HTTP request failed: %v", err)
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status)
		return result
	}

	// Limit response body size
	var reader io.Reader = resp.Body
	if ws.config.MaxBodySize > 0 {
		reader = io.LimitReader(resp.Body, ws.config.MaxBodySize)
	}

	// Parse HTML
	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		result.Error = fmt.Sprintf("Failed to parse HTML: %v", err)
		return result
	}

	// Extract data using selectors
	for fieldName, selector := range task.Selectors {
		data := ws.extractData(doc, selector, logger)
		result.Data[fieldName] = data
	}

	// Update download statistics
	if resp.ContentLength > 0 {
		ws.updateStats(func(stats *ScraperStats) {
			stats.BytesDownloaded += resp.ContentLength
		})
	}

	logger.WithFields(logrus.Fields{
		"status_code":      resp.StatusCode,
		"content_length":   resp.ContentLength,
		"fields_extracted": len(result.Data),
	}).Info("Successfully scraped URL")

	// Add metadata
	result.Data["_meta"] = map[string]interface{}{
		"url":            task.URL,
		"domain":         parsedURL.Host,
		"status_code":    resp.StatusCode,
		"content_type":   resp.Header.Get("Content-Type"),
		"content_length": resp.ContentLength,
		"scraped_at":     result.Timestamp,
		"user_agent":     ws.config.UserAgent,
	}

	return result
}

// extractData extracts data from HTML using CSS selectors
func (ws *WebScraper) extractData(doc *goquery.Document, selector string, logger *logrus.Entry) any {
	selection := doc.Find(selector)

	if selection.Length() == 0 {
		logger.WithField("selector", selector).Warn("No elements found for selector")
		return nil
	}

	if selection.Length() == 1 {
		// Single element - return appropriate attribute or text
		element := selection.First()
		return ws.extractElementData(element)
	}

	// Multiple elements - return array
	var results []any
	selection.Each(func(i int, s *goquery.Selection) {
		data := ws.extractElementData(s)
		if data != nil {
			results = append(results, data)
		}
	})

	return results
}

// extractElementData extracts data from a single HTML element
func (ws *WebScraper) extractElementData(element *goquery.Selection) any {
	// Priority order for attribute extraction
	attributes := []string{"href", "src", "alt", "title", "data-value", "value"}

	for _, attr := range attributes {
		if value, exists := element.Attr(attr); exists && value != "" {
			// Make relative URLs absolute if possible
			if attr == "href" || attr == "src" {
				if strings.HasPrefix(value, "http") {
					return value
				}
				// Could add base URL resolution here
			}
			return value
		}
	}

	// Fall back to text content
	text := strings.TrimSpace(element.Text())
	if text != "" {
		return text
	}

	return nil
}

// updateStats safely updates scraper statistics
func (ws *WebScraper) updateStats(updater func(*ScraperStats)) {
	ws.statsMutex.Lock()
	defer ws.statsMutex.Unlock()
	updater(&ws.stats)
}
