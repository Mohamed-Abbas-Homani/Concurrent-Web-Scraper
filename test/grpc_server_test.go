package test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"webscraper/internal/scraper"
	"webscraper/internal/server"
	"webscraper/pkg/types"
	pb "webscraper/pkg/types/proto"
)

// Helper to start gRPC server in a goroutine
func startTestGRPCServer(scraper *scraper.WebScraper, addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s := grpc.NewServer()
	pb.RegisterScraperServiceServer(s, server.NewGRPCServer(scraper))
	reflection.Register(s)
	go s.Serve(lis)
	return nil
}

// Retry getting results until timeout
func pollForResults(client pb.ScraperServiceClient, taskIDs []string, timeout time.Duration) ([]*pb.ScrapingResult, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.GetResultsByIds(context.Background(), &pb.GetResultsByIdsRequest{
			TaskIds: taskIDs,
		})
		if err != nil {
			return nil, err
		}
		if len(resp.Results) == len(taskIDs) {
			return resp.Results, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, fmt.Errorf("timeout waiting for results")
}

func TestGRPC_AddTasksAndGetResults(t *testing.T) {
	// Set up scraper
	cfg := &types.ScraperConfig{
		MaxConcurrency: 3,
		RateLimit:      5,
		Timeout:        10 * time.Second,
	}
	ws := scraper.NewWebScraper(cfg)
	if err := ws.Start(); err != nil {
		t.Fatalf("Failed to start scraper: %v", err)
	}
	defer ws.Stop()

	// Start gRPC server
	addr := ":9090"
	if err := startTestGRPCServer(ws, addr); err != nil {
		t.Fatalf("Failed to start test gRPC server: %v", err)
	}

	// Create gRPC client
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		t.Fatalf("Failed to connect to gRPC server: %v", err)
	}
	defer conn.Close()

	client := pb.NewScraperServiceClient(conn)

	// Add task
	task := &pb.ScrapingTask{
		Id:     "grpc-quote-1",
		Url:    "http://quotes.toscrape.com",
		Method: "GET",
		Selectors: map[string]string{
			"quote":  ".quote .text",
			"author": ".quote .author",
		},
	}

	_, err = client.AddTasks(context.Background(), &pb.AddTasksRequest{
		Tasks: []*pb.ScrapingTask{task},
	})
	if err != nil {
		t.Fatalf("AddTasks failed: %v", err)
	}

	// Get results (retry loop)
	taskIDs := []string{"grpc-quote-1"}
	results, err := pollForResults(client, taskIDs, 10*time.Second)
	if err != nil {
		t.Fatalf("Failed to get expected results: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	if results[0].StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", results[0].StatusCode)
	}

	t.Logf("Success: %+v", results[0].Data)
}
