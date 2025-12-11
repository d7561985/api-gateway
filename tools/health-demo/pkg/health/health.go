package health

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
)

type Check struct{}

func (h Check) Check(ctx context.Context, request *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	// Extract and log incoming metadata (headers)
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		headers := make(map[string][]string)
		for k, v := range md {
			headers[k] = v
		}
		jsonHeaders, _ := json.MarshalIndent(headers, "", "  ")
		log.Printf("=== gRPC Health Check - Received Headers ===\n%s\n", string(jsonHeaders))

		// Log specific auth-adapter headers
		if userID := md.Get("user-id"); len(userID) > 0 {
			log.Printf(">>> user-id: %s", userID[0])
		}
		if sessionID := md.Get("session-id"); len(sessionID) > 0 {
			log.Printf(">>> session-id: %s", sessionID[0])
		}
	} else {
		log.Printf("=== gRPC Health Check - No metadata received ===")
	}

	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

func (h Check) Watch(request *grpc_health_v1.HealthCheckRequest, server grpc_health_v1.Health_WatchServer) error {
	// Extract and log incoming metadata (headers) on stream start
	if md, ok := metadata.FromIncomingContext(server.Context()); ok {
		headers := make(map[string][]string)
		for k, v := range md {
			headers[k] = v
		}
		jsonHeaders, _ := json.MarshalIndent(headers, "", "  ")
		log.Printf("=== gRPC Health Watch Stream - Received Headers ===\n%s\n", string(jsonHeaders))

		// Log specific auth-adapter headers
		if userID := md.Get("user-id"); len(userID) > 0 {
			log.Printf(">>> user-id: %s", userID[0])
		}
		if sessionID := md.Get("session-id"); len(sessionID) > 0 {
			log.Printf(">>> session-id: %s", sessionID[0])
		}
	}

	for {
		select {
		case <-server.Context().Done():
			return nil
		case <-time.After(time.Second):
			if err := server.Send(&grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}); err != nil {
				return err
			}
		}
	}
}

var _ grpc_health_v1.HealthServer = &Check{}
