package extAuth

import (
	"context"

	"google.golang.org/grpc"
)

func NewAuthSessionServiceClient(*grpc.ClientConn) AuthSessionServiceClient {
	return &stab{}
}

type stab struct {
}

func (s stab) ValidateSession(ctx context.Context, req *ValidateSessionRequest, opt ...grpc.CallOption) (*ValidateSessionResponse, error) {
	return &ValidateSessionResponse{}, nil
}
