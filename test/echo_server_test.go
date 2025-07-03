package test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"webscraper/internal/scraper"
	"webscraper/internal/server"
	"webscraper/pkg/types"
)

func TestAddTasksAndGetResults(t *testing.T) {
	// Setup scraper
	cfg := &types.ScraperConfig{
		MaxConcurrency: 5,
		RateLimit:      10,
		Timeout:        10 * time.Second,
	}
	ws := scraper.NewWebScraper(cfg)
	if err := ws.Start(); err != nil {
		t.Fatalf("Failed to start scraper: %v", err)
	}
	defer ws.Stop()

	// Start echo server
	e := echo.New()
	s := server.NewEchoServer(ws)
	e.POST("/tasks", s.AddTasks)
	e.GET("/results", s.GetResultsByIds)

	// Define test tasks
	tasks := []types.ScrapingTask{
		{
			ID:     "test-quote-1",
			URL:    "http://quotes.toscrape.com",
			Method: "GET",
			Selectors: map[string]string{
				"quote":  ".quote .text",
				"author": ".quote .author",
			},
		},
		{
			ID:     "test-quote-2",
			URL:    "http://quotes.toscrape.com",
			Method: "GET",
			Selectors: map[string]string{
				"tags": ".quote .tags",
			},
		},
	}

	// Marshal request body
	body, _ := json.Marshal(map[string]interface{}{
		"tasks": tasks,
	})

	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Send AddTasks request
	if err := s.AddTasks(c); err != nil {
		t.Fatalf("AddTasks failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("AddTasks returned status %d", rec.Code)
	}

	// Poll for results (up to 10 seconds)
	taskIDs := []string{"test-quote-1", "test-quote-2"}
	var (
		resp       map[string]interface{}
		bodyBytes  []byte
		gotResults bool
	)

	for i := 0; i < 20; i++ {
		query := "/results?task_ids=" + strings.Join(taskIDs, ",")
		req = httptest.NewRequest(http.MethodGet, query, nil)
		rec = httptest.NewRecorder()
		c = e.NewContext(req, rec)

		// Call GetResultsByIds
		if err := s.GetResultsByIds(c); err != nil {
			t.Fatalf("GetResultsByIds failed: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("GetResultsByIds returned status %d", rec.Code)
		}

		bodyBytes, _ = io.ReadAll(rec.Body)
		_ = json.Unmarshal(bodyBytes, &resp)

		if count, ok := resp["count"].(float64); ok && int(count) == len(taskIDs) {
			gotResults = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if !gotResults {
		t.Fatalf("Expected %d results, got %v", len(taskIDs), resp["count"])
	}

	t.Logf("Success: %s", string(bodyBytes))
}
