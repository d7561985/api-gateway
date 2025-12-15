package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"envoy.auth/extAuth"
)

func parseTokenCookie(raw string) (string, error) {
	header := http.Header{}
	header.Add("Cookie", raw)
	request := http.Request{Header: header}
	c, err := request.Cookie("token")

	if err == http.ErrNoCookie {
		return "", nil
	}

	if err != nil {
		return "", err
	}

	return c.Value, nil
}

func authorize(perm string, roles []*extAuth.Role) bool {
	for _, r := range roles {
		// hack for "CLIENT" role (user on the site)
		if perm == "" && r.Name == "CLIENT" {
			return true
		}

		for _, p := range r.Permissions {
			if p.Name == perm {
				return true
			}
		}
	}

	return false
}

func parsePath(path string) (service string, method string) {
	// Remove query string if present
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}

	parts := strings.Split(path, "/")
	// Expected format: /api/{service}/{method}/...
	// parts[0] = "", parts[1] = "api", parts[2] = service, parts[3] = method
	if len(parts) < 4 {
		return
	}

	// Verify this is an /api/ path
	if parts[1] != "api" {
		return
	}

	service = parts[2]
	method = fmt.Sprintf("%s/%s", service, parts[3])

	return
}

func getEnvVar(varName, defaultVal string) string {
	val := os.Getenv(varName)
	if val == "" {
		return defaultVal
	}

	return val
}
