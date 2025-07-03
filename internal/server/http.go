package server

import (
	"net/http"
	"strings"

	"webscraper/internal/scraper"
	"webscraper/pkg/types"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type EchoServer struct {
	scraper *scraper.WebScraper
}

func NewEchoServer(scr *scraper.WebScraper) *EchoServer {
	return &EchoServer{scraper: scr}
}

// POST /tasks
// Accepts JSON body with {"tasks": [ ... ]} and returns {"ids": [ ... ]}
func (s *EchoServer) AddTasks(c echo.Context) error {
	var req struct {
		Tasks []types.ScrapingTask `json:"tasks"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	ids := make([]string, 0, len(req.Tasks))
	for i := range req.Tasks {
		task := &req.Tasks[i]
		if err := s.scraper.AddTask(task); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		ids = append(ids, task.ID)
	}

	return c.JSON(http.StatusOK, map[string][]string{"ids": ids})
}

// GET /results
// Accepts JSON body with {"task_ids": [ ... ]} and returns {"results": [ ... ]}
// GET /results?task_ids=id1,id2,id3
func (s *EchoServer) GetResultsByIds(c echo.Context) error {
	// Get query param and split it
	taskIDsParam := c.QueryParam("task_ids")
	if taskIDsParam == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "task_ids query parameter is required")
	}

	taskIDs := make([]string, 0)
	for _, id := range strings.Split(taskIDsParam, ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			taskIDs = append(taskIDs, id)
		}
	}

	results := s.scraper.GetResultsByIds(taskIDs)

	resp := make([]map[string]any, 0, len(results))
	for _, res := range results {
		resp = append(resp, map[string]any{
			"task_id":     res.TaskID,
			"url":         res.URL,
			"data":        res.Data,
			"timestamp":   res.Timestamp.Unix(),
			"duration":    res.Duration.Milliseconds(),
			"error":       res.Error,
			"status_code": res.StatusCode,
			"from_cache":  res.FromCache,
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"results": resp,
		"count":   len(resp),
	})
}

func StartEchoServer(scraper *scraper.WebScraper) error {
	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	server := NewEchoServer(scraper)

	// API routes
	e.POST("/tasks", server.AddTasks)
	e.GET("/results", server.GetResultsByIds)

	// Serve HTML UI
	e.Static("/", "web")

	addr := ":8081"
	return e.Start(addr)
}
