package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
	"webscraper/internal/config"
	"webscraper/internal/scraper"
	"webscraper/pkg/types"

	"github.com/sirupsen/logrus"
)

func main() {
	// Command line flags
	var (
		configFile = flag.String("config", "", "Configuration file path")
		verbose    = flag.Bool("verbose", false, "Enable verbose logging")
		taskFile   = flag.String("tasks", "", "JSON file containing scraping tasks")
		outputFile = flag.String("output", "", "Output file for results (default: stdout)")
	)
	flag.Parse()

	// Setup logging
	logger := logrus.New()
	if *verbose {
		logger.SetLevel(logrus.DebugLevel)
	}
	logger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339,
	})

	// Load configuration
	var cfg *types.ScraperConfig
	var err error

	if *configFile != "" {
		cfg, err = config.LoadConfig(*configFile)
		if err != nil {
			logger.WithError(err).Fatal("Failed to load configuration")
		}
	} else {
		cfg = config.DefaultConfig()
	}

	// Validate configuration
	if err := config.ValidateConfig(cfg); err != nil {
		logger.WithError(err).Fatal("Invalid configuration")
	}

	logger.WithFields(logrus.Fields{
		"max_concurrency": cfg.MaxConcurrency,
		"rate_limit":      cfg.RateLimit,
		"timeout":         cfg.Timeout,
	}).Info("Starting scraper with configuration")

	// Create and start scraper
	scraperInstance := scraper.NewWebScraper(cfg)
	if err := scraperInstance.Start(); err != nil {
		logger.WithError(err).Fatal("Failed to start scraper")
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("Received shutdown signal")
		cancel()
	}()

	// Setup output
	var outputWriter *os.File
	if *outputFile != "" {
		outputWriter, err = os.Create(*outputFile)
		if err != nil {
			logger.WithError(err).Fatal("Failed to create output file")
		}
		defer outputWriter.Close()
	} else {
		outputWriter = os.Stdout
	}

	// Start result processor
	go processResults(scraperInstance.GetResults(), outputWriter, logger)

	// Load and add tasks
	if *taskFile != "" {
		tasks, err := loadTasks(*taskFile)
		if err != nil {
			logger.WithError(err).Fatal("Failed to load tasks")
		}

		logger.WithField("task_count", len(tasks)).Info("Adding tasks to scraper")
		for _, task := range tasks {
			if err := scraperInstance.AddTask(task); err != nil {
				logger.WithError(err).WithField("task_id", task.ID).Error("Failed to add task")
			}
		}
	} else {
		// Add example tasks
		exampleTasks := getExampleTasks()
		for _, task := range exampleTasks {
			if err := scraperInstance.AddTask(task); err != nil {
				logger.WithError(err).WithField("task_id", task.ID).Error("Failed to add task")
			}
		}
	}

	// Start statistics reporter
	go reportStatistics(scraperInstance, logger, ctx)

	// Wait for shutdown signal or context cancellation
	<-ctx.Done()

	// Graceful shutdown
	logger.Info("Shutting down scraper...")
	if err := scraperInstance.Stop(); err != nil {
		logger.WithError(err).Error("Error during scraper shutdown")
	}

	// Print final statistics
	stats := scraperInstance.GetStats()
	logger.WithFields(logrus.Fields{
		"tasks_processed":  stats.TasksProcessed,
		"tasks_successful": stats.TasksSuccessful,
		"tasks_failed":     stats.TasksFailed,
		"average_duration": stats.AverageDuration,
		"bytes_downloaded": stats.BytesDownloaded,
	}).Info("Final scraper statistics")

	logger.Info("Scraper shutdown complete")
}

// processResults handles scraping results
func processResults(results <-chan *types.ScrapingResult, output *os.File, logger *logrus.Logger) {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")

	for result := range results {
		if err := encoder.Encode(result); err != nil {
			logger.WithError(err).Error("Failed to write result")
		}

		if result.Error != "" {
			logger.WithFields(logrus.Fields{
				"task_id": result.TaskID,
				"url":     result.URL,
				"error":   result.Error,
			}).Error("Scraping failed")
		} else {
			logger.WithFields(logrus.Fields{
				"task_id":     result.TaskID,
				"url":         result.URL,
				"duration":    result.Duration,
				"data_points": len(result.Data),
				"from_cache":  result.FromCache,
			}).Info("Scraping completed successfully")
		}
	}
}

// loadTasks loads scraping tasks from a JSON file
func loadTasks(filename string) ([]*types.ScrapingTask, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read tasks file: %w", err)
	}

	var tasks []*types.ScrapingTask
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, fmt.Errorf("failed to parse tasks JSON: %w", err)
	}

	return tasks, nil
}

// getExampleTasks returns example scraping tasks
func getExampleTasks() []*types.ScrapingTask {
	return []*types.ScrapingTask{
		{
			ID:  "quotes-to-scrape",
			URL: "http://quotes.toscrape.com/",
			Selectors: map[string]string{
				"quotes":    ".quote .text",
				"authors":   ".quote .author",
				"tags":      ".quote .tags a",
				"next_page": ".next a",
			},
			Priority: 1,
		},
	}
}

// reportStatistics periodically reports scraper statistics
func reportStatistics(scraperInstance *scraper.WebScraper, logger *logrus.Logger, ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			stats := scraperInstance.GetStats()
			logger.WithFields(logrus.Fields{
				"tasks_processed":  stats.TasksProcessed,
				"tasks_successful": stats.TasksSuccessful,
				"tasks_failed":     stats.TasksFailed,
				"average_duration": stats.AverageDuration,
				"bytes_downloaded": stats.BytesDownloaded,
			}).Info("Scraper statistics")

		case <-ctx.Done():
			return
		}
	}
}
