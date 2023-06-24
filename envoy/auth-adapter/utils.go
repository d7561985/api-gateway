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
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return
	}

	service = parts[len(parts)-2]
	method = fmt.Sprintf("%s/%s", service, parts[len(parts)-1])

	return
}

func getEnvVar(varName, defaultVal string) string {
	val := os.Getenv(varName)
	if val == "" {
		return defaultVal
	}

	return val
}
