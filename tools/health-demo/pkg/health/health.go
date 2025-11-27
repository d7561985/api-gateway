package health

import (
	"context"
	"time"

	"google.golang.org/grpc/health/grpc_health_v1"
)

type Check struct{}

func (h Check) Check(ctx context.Context, request *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

func (h Check) Watch(request *grpc_health_v1.HealthCheckRequest, server grpc_health_v1.Health_WatchServer) error {
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
