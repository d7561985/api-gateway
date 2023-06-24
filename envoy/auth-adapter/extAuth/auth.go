package extAuth

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type AuthSessionServiceClient interface {
	ValidateSession(ctx context.Context, req *ValidateSessionRequest, opt ...grpc.CallOption) (
		*ValidateSessionResponse, error)
}
type ValidateSessionRequest struct {
	SessionToken string
}

type ValidateSessionResponse struct {
	UserId    string
	SessionId string
	Roles     []*Role
}

type Role struct {
	Name        string // CLIENT
	Permissions []*Permission
}

type Permission struct {
	Name string
}
