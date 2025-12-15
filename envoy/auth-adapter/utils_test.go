package main

import (
	"testing"
)

func TestParsePath(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		wantService    string
		wantMethod     string
	}{
		{
			name:           "standard gRPC path",
			path:           "/api/FakeService/Handle",
			wantService:    "FakeService",
			wantMethod:     "FakeService/Handle",
		},
		{
			name:           "HTTP path with query string",
			path:           "/api/bonus/progress?userId=123",
			wantService:    "bonus",
			wantMethod:     "bonus/progress",
		},
		{
			name:           "HTTP path without query",
			path:           "/api/game/calculate",
			wantService:    "game",
			wantMethod:     "game/calculate",
		},
		{
			name:           "path with extra segments",
			path:           "/api/user/profile/settings",
			wantService:    "user",
			wantMethod:     "user/profile",
		},
		{
			name:           "path with query and extra segments",
			path:           "/api/bonus/progress/123?foo=bar",
			wantService:    "bonus",
			wantMethod:     "bonus/progress",
		},
		{
			name:           "non-api path",
			path:           "/health",
			wantService:    "",
			wantMethod:     "",
		},
		{
			name:           "root path",
			path:           "/",
			wantService:    "",
			wantMethod:     "",
		},
		{
			name:           "api without service",
			path:           "/api/",
			wantService:    "",
			wantMethod:     "",
		},
		{
			name:           "wrong prefix",
			path:           "/v1/service/method",
			wantService:    "",
			wantMethod:     "",
		},
		{
			name:           "empty path",
			path:           "",
			wantService:    "",
			wantMethod:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotService, gotMethod := parsePath(tt.path)
			if gotService != tt.wantService {
				t.Errorf("parsePath(%q) service = %q, want %q", tt.path, gotService, tt.wantService)
			}
			if gotMethod != tt.wantMethod {
				t.Errorf("parsePath(%q) method = %q, want %q", tt.path, gotMethod, tt.wantMethod)
			}
		})
	}
}
