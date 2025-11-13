package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type RateLimitConf struct {
	Period string `yaml:"period"`
	Count  int    `yaml:"count"`
	Delay  string `yaml:"delay"`
}

type AuthConf struct {
	Policy     string         `yaml:"policy"`
	Permission string         `yaml:"permission"`
	RateLimit  *RateLimitConf `yaml:"rate_limit"`
}

const (
	// auth policies
	apRequired = "required"
	apOptional = "optional"
	apNoNeed   = "no-need"
)

func (c *AuthConf) Validate() error {
	switch c.Policy {
	case apRequired, apOptional, apNoNeed:
		// Policy is valid, continue validation
	default:
		return fmt.Errorf("unknown auth policy %s", c.Policy)
	}

	if c.RateLimit != nil {
		if c.RateLimit.Count <= 0 {
			return fmt.Errorf("rate limit count must be positive")
		}
		if c.RateLimit.Period == "" {
			return fmt.Errorf("rate limit period cannot be empty")
		}
		// Basic period validation (could be enhanced)
		if c.RateLimit.Period != "1s" && c.RateLimit.Period != "1m" && c.RateLimit.Period != "1h" {
			return fmt.Errorf("rate limit period must be like '1s', '1m', '1h'")
		}
	}

	return nil
}

type HealthCheckConf struct {
	Path               string `yaml:"path"`                // Health check path
	IntervalSeconds    int    `yaml:"interval_seconds"`    // Check interval
	TimeoutSeconds     int    `yaml:"timeout_seconds"`     // Request timeout
	HealthyThreshold   int    `yaml:"healthy_threshold"`   // Healthy threshold
	UnhealthyThreshold int    `yaml:"unhealthy_threshold"` // Unhealthy threshold
}

type CircuitBreakerConf struct {
	MaxConnections     int `yaml:"max_connections"`      // Max connections
	MaxPendingRequests int `yaml:"max_pending_requests"` // Max pending requests
	MaxRequests        int `yaml:"max_requests"`         // Max requests
	MaxRetries         int `yaml:"max_retries"`          // Max retries
}

type ClusterConf struct {
	Name           string              `yaml:"name"`
	Addr           string              `yaml:"addr"`
	Type           string              `yaml:"type"`            // "grpc" or "http"
	HealthCheck    *HealthCheckConf    `yaml:"health_check"`    // Optional health check
	CircuitBreaker *CircuitBreakerConf `yaml:"circuit_breaker"` // Optional circuit breaker
}

func (c ClusterConf) Validate() error {
	parts := strings.Split(c.Addr, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid address %s", c.Addr)
	}
	_, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("invalid port number %s", parts[1])
	}

	// Validate cluster type
	if c.Type != "" && c.Type != "grpc" && c.Type != "http" {
		return fmt.Errorf("invalid cluster type %s, must be 'grpc' or 'http'", c.Type)
	}

	// Validate health check
	if c.HealthCheck != nil {
		if c.HealthCheck.Path == "" {
			return fmt.Errorf("health check path cannot be empty")
		}
		if c.HealthCheck.IntervalSeconds <= 0 {
			c.HealthCheck.IntervalSeconds = 30 // default
		}
		if c.HealthCheck.TimeoutSeconds <= 0 {
			c.HealthCheck.TimeoutSeconds = 5 // default
		}
		if c.HealthCheck.HealthyThreshold <= 0 {
			c.HealthCheck.HealthyThreshold = 2 // default
		}
		if c.HealthCheck.UnhealthyThreshold <= 0 {
			c.HealthCheck.UnhealthyThreshold = 3 // default
		}
	}

	// Validate circuit breaker
	if c.CircuitBreaker != nil {
		if c.CircuitBreaker.MaxConnections <= 0 {
			c.CircuitBreaker.MaxConnections = 1024 // default
		}
		if c.CircuitBreaker.MaxPendingRequests <= 0 {
			c.CircuitBreaker.MaxPendingRequests = 1024 // default
		}
		if c.CircuitBreaker.MaxRequests <= 0 {
			c.CircuitBreaker.MaxRequests = 1024 // default
		}
		if c.CircuitBreaker.MaxRetries <= 0 {
			c.CircuitBreaker.MaxRetries = 3 // default
		}
	}

	return nil
}

func (c ClusterConf) AddrHost() string {
	return strings.Split(c.Addr, ":")[0]
}

func (c ClusterConf) AddrPort() string {
	return strings.Split(c.Addr, ":")[1]
}

func (c ClusterConf) IsGRPC() bool {
	return c.Type == "" || c.Type == "grpc" // default to gRPC
}

func (c ClusterConf) IsHTTP() bool {
	return c.Type == "http"
}

func (r *RateLimitConf) GetFillIntervalSeconds() string {
	switch r.Period {
	case "1s":
		return "1s"
	case "1m":
		return "60s"
	case "1h":
		return "3600s"
	default:
		return "60s" // default fallback
	}
}

func (r *RateLimitConf) GetTokensPerFill() int {
	// For simple implementation, tokens per fill = count
	return r.Count
}

func (r *RateLimitConf) GetMaxTokens() int {
	// Max tokens = count * 2 for burst capacity
	return r.Count * 2
}

type APIConf struct {
	APIsDescr []struct {
		Name    string    `yaml:"name"`
		Cluster string    `yaml:"cluster"`
		Auth    *AuthConf `yaml:"auth"`
		Methods []struct {
			Name string    `yaml:"name"`
			Auth *AuthConf `yaml:"auth"`
		} `yaml:"methods"`
	} `yaml:"apis"`

	Clusters []ClusterConf `yaml:"clusters"`
	APIRoute string        `yaml:"api_route"`
}

func (c *APIConf) Validate() error {
	clusters := make(map[string]string)
	apis := make(map[string]string)
	methods := make(map[string]bool)

	if len(c.APIRoute) == 0 || c.APIRoute[0] != '/' {
		return fmt.Errorf("invalid api_route")
	}

	for _, cl := range c.Clusters {
		if _, ok := clusters[cl.Name]; ok {
			return fmt.Errorf("cluster %s is defined twice", cl.Name)
		}
		clusters[cl.Name] = cl.Addr
		if err := cl.Validate(); err != nil {
			return fmt.Errorf("invalid cluster %s definition: %s", cl.Name, err)
		}
	}

	for _, api := range c.APIsDescr {
		if _, ok := apis[api.Name]; ok {
			return fmt.Errorf("API %s is defined twice", api.Name)
		}
		if _, ok := clusters[api.Cluster]; !ok {
			return fmt.Errorf("cluster %s for API %s is not defined", api.Cluster, api.Name)
		}
		apis[api.Name] = api.Cluster

		if api.Auth != nil {
			if err := api.Auth.Validate(); err != nil {
				return err
			}
		}

		for _, m := range api.Methods {
			fullMethod := fmt.Sprintf("%s/%s", api.Name, m.Name)
			if _, ok := methods[fullMethod]; ok {
				return fmt.Errorf("method %s is already defined", fullMethod)
			}
			methods[fullMethod] = true

			if m.Auth != nil {
				if err := m.Auth.Validate(); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func LoadConfig(file string) (*APIConf, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	c := &APIConf{}
	err = yaml.Unmarshal(data, c)
	if err != nil {
		return nil, err
	}

	return c, nil
}
