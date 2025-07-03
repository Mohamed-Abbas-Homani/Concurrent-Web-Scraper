package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"
	"webscraper/internal/config"
	"webscraper/internal/scraper"
	"webscraper/internal/server"
	"webscraper/pkg/types"

	"github.com/sirupsen/logrus"
)

func main() {
	// Command line flags
	var (
		configFile = flag.String("config", "", "Configuration file path")
		verbose    = flag.Bool("verbose", false, "Enable verbose logging")
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

	// Start gRPC and Echo servers in goroutines
	go func() {
		if err := server.StartGRPCServer(scraperInstance); err != nil {
			logger.Fatalf("gRPC server error: %v", err)
		}
	}()

	go func() {
		if err := server.StartEchoServer(scraperInstance); err != nil {
			logger.Fatalf("Echo server error: %v", err)
		}
	}()

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
