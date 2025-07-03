package server

import (
	"context"
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"

	"google.golang.org/grpc/reflection"
	"webscraper/internal/scraper"
	"webscraper/pkg/types"
	pb "webscraper/pkg/types/proto"
)

// grpcServer wraps the scraper logic
type grpcServer struct {
	pb.UnimplementedScraperServiceServer
	scraper *scraper.WebScraper
}

func NewGRPCServer(scraper *scraper.WebScraper) pb.ScraperServiceServer {
	return &grpcServer{scraper: scraper}
}

// AddTasks handles adding multiple scraping tasks and returns their IDs
func (s *grpcServer) AddTasks(ctx context.Context, req *pb.AddTasksRequest) (*pb.AddTasksResponse, error) {
	ids := make([]string, 0, len(req.Tasks))
	for _, t := range req.Tasks {
		task := &types.ScrapingTask{
			URL:       t.Url,
			Selectors: t.Selectors,
			ID:        t.Id,
			Headers:   t.Headers,
			Method:    t.Method,
		}
		if err := s.scraper.AddTask(task); err != nil {
			return nil, fmt.Errorf("failed to add task %s: %w", task.ID, err)
		}
		ids = append(ids, task.ID)
	}

	return &pb.AddTasksResponse{Ids: ids}, nil
}

// GetResultsByIds returns scraping results for given task IDs (no streaming)
func (s *grpcServer) GetResultsByIds(ctx context.Context, req *pb.GetResultsByIdsRequest) (*pb.GetResultsByIdsResponse, error) {
	results := s.scraper.GetResultsByIds(req.TaskIds)

	pbResults := make([]*pb.ScrapingResult, 0, len(results))
	for _, res := range results {
		pbResults = append(pbResults, &pb.ScrapingResult{
			TaskId:     res.TaskID,
			Url:        res.URL,
			Data:       convertMapToString(res.Data),
			Timestamp:  res.Timestamp.Unix(),
			Duration:   int64(res.Duration),
			Error:      res.Error,
			StatusCode: int32(res.StatusCode),
			FromCache:  res.FromCache,
		})
	}

	return &pb.GetResultsByIdsResponse{Results: pbResults}, nil
}

// convertMapToString converts a map with any values to map[string]string for protobuf compatibility
func convertMapToString(data map[string]any) map[string]string {
	out := make(map[string]string, len(data))
	for k, v := range data {
		out[k] = fmt.Sprintf("%v", v)
	}
	return out
}

func StartGRPCServer(scraper *scraper.WebScraper) error {
	addr := ":8080"
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s := grpc.NewServer()
	pb.RegisterScraperServiceServer(s, NewGRPCServer(scraper))

	// Register reflection service on gRPC server.
	reflection.Register(s)

	asciiArt := `
	
 _____ ____  ____  ____ 
/  __//  __\/  __\/   _\
| |  _|  \/||  \/||  /  
| |_//|    /|  __/|  \__
\____\\_/\_\\_/   \____/

gRPC server listening on %s
`

	log.Printf(asciiArt, addr)

	return s.Serve(lis)
}
