package main

import (
	"bytes"
	"os"
	"text/template"
)

var (
	// Route template for gRPC - keeps full path (e.g., /api/FakeService/Handle -> /FakeService/Handle)
	envoyGrpcRouteTmpl = template.Must(template.New("grpcRouteTmpl").Parse(`
              - match: { prefix: "{{.APIRoute}}{{.APIName}}" }
                route:
                  cluster: {{.ClusterName}}
                  timeout: 0s
                  prefix_rewrite: "/{{.APIName}}"
                  max_stream_duration:
                    max_stream_duration: 600s
                    grpc_timeout_header_max: 0s
{{.RateLimitConfig}}
`))

	// Route template for HTTP - strips service name (e.g., /api/game/calculate -> /calculate)
	envoyHttpRouteTmpl = template.Must(template.New("httpRouteTmpl").Parse(`
              - match: { prefix: "{{.APIRoute}}{{.APIName}}" }
                route:
                  cluster: {{.ClusterName}}
                  timeout: 30s
                  prefix_rewrite: "/{{.MethodName}}"
{{.RateLimitConfig}}
`))

	// Fallback for API-level routes (matches /api/game/ prefix)
	envoyHttpApiRouteTmpl = template.Must(template.New("httpApiRouteTmpl").Parse(`
              - match: { prefix: "{{.APIRoute}}{{.ServiceName}}/" }
                route:
                  cluster: {{.ClusterName}}
                  timeout: 30s
                  regex_rewrite:
                    pattern:
                      regex: "^{{.APIRoute}}{{.ServiceName}}/(.*)"
                    substitution: "/\\1"
{{.RateLimitConfig}}
`))
	envoyGrpcClusterTmpl = template.Must(template.New("grpcClusterTmpl").Parse(`
  - name: {{.ClusterName}}
    connect_timeout: 5s
    type: STRICT_DNS
    typed_extension_protocol_options:
      envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
        "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
        explicit_http_config:
          http2_protocol_options:
            max_concurrent_streams: 1024
            initial_stream_window_size: 16777216  # 16MiB
            initial_connection_window_size: 25165824  # 24MiB
    lb_policy: ROUND_ROBIN{{.CircuitBreakerConfig}}{{.HealthCheckConfig}}
    load_assignment:
      cluster_name: {{.ClusterName}}
      endpoints:
        - lb_endpoints:
          - endpoint:
              address:
                socket_address:
                  address: "{{.ClusterAddr}}"
                  port_value: {{.ClusterPort}}
    upstream_connection_options:
        tcp_keepalive:
            keepalive_probes: 2
            keepalive_time: 10
            keepalive_interval: 10
`))

	envoyHttpClusterTmpl = template.Must(template.New("httpClusterTmpl").Parse(`
  - name: {{.ClusterName}}
    connect_timeout: 5s
    type: STRICT_DNS
    lb_policy: ROUND_ROBIN{{.CircuitBreakerConfig}}{{.HealthCheckConfig}}
    load_assignment:
      cluster_name: {{.ClusterName}}
      endpoints:
        - lb_endpoints:
          - endpoint:
              address:
                socket_address:
                  address: "{{.ClusterAddr}}"
                  port_value: {{.ClusterPort}}
    upstream_connection_options:
        tcp_keepalive:
            keepalive_probes: 2
            keepalive_time: 10
            keepalive_interval: 10
`))

	envoyRateLimitTmpl = template.Must(template.New("rateLimitTmpl").Parse(`
                typed_per_filter_config:
                  envoy.filters.http.local_ratelimit:
                    "@type": type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit
                    stat_prefix: {{.StatPrefix}}
                    token_bucket:
                      max_tokens: {{.MaxTokens}}
                      tokens_per_fill: {{.TokensPerFill}}
                      fill_interval: {{.FillInterval}}
                    filter_enabled:
                      runtime_key: local_rate_limit_enabled
                      default_value:
                        numerator: 100
                        denominator: HUNDRED
                    filter_enforced:
                      runtime_key: local_rate_limit_enforced
                      default_value:
                        numerator: 100
                        denominator: HUNDRED
`))

	envoyHealthCheckTmpl = template.Must(template.New("healthCheckTmpl").Parse(`
    health_checks:
      - timeout: {{.TimeoutSeconds}}s
        interval: {{.IntervalSeconds}}s
        unhealthy_threshold: {{.UnhealthyThreshold}}
        healthy_threshold: {{.HealthyThreshold}}
        http_health_check:
          path: "{{.Path}}"
          request_headers_to_add:
            - header:
                key: "user-agent"
                value: "envoy-health-check"
`))

	envoyCircuitBreakerTmpl = template.Must(template.New("circuitBreakerTmpl").Parse(`
    circuit_breakers:
      thresholds:
        - priority: DEFAULT
          max_connections: {{.MaxConnections}}
          max_pending_requests: {{.MaxPendingRequests}}
          max_requests: {{.MaxRequests}}
          max_retries: {{.MaxRetries}}
        - priority: HIGH
          max_connections: {{.MaxConnections}}
          max_pending_requests: {{.MaxPendingRequests}}
          max_requests: {{.MaxRequests}}
          max_retries: {{.MaxRetries}}
`))

	envoyConfTmpl = template.Must(template.New("envoyTmpl").Parse(`
admin:
  access_log_path: /dev/null
  address:
    socket_address: { address: 0.0.0.0, port_value: 8000 }

static_resources:
  listeners:
  - name: web_grpc_listener
    address:
      socket_address: { address: 0.0.0.0, port_value: 8080 }
    filter_chains:
    - filters:
      - name: envoy.filters.network.http_connection_manager
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
          tracing:
            provider:
              name: envoy.tracers.opentelemetry
              typed_config:
                "@type": type.googleapis.com/envoy.config.trace.v3.OpenTelemetryConfig
                grpc_service:
                  envoy_grpc:
                    cluster_name: opentelemetry_collector
                  timeout: 0.250s
                service_name: api-gateway
          generate_request_id: true
          codec_type: auto
          stat_prefix: ingress_http
          use_remote_address: true
          xff_num_trusted_hops: 0
          access_log:
            - name: envoy.access_loggers.stdout
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.access_loggers.stream.v3.StdoutAccessLog
          route_config:
            name: local_route
            virtual_hosts:
            - name: grpc_proxy
              domains: ["*"]
              response_headers_to_remove: ["grpc-message"]
              cors:
                allow_origin_string_match:
                - prefix: "*"
                allow_methods: "GET, PUT, DELETE, POST, OPTIONS"
                allow_headers: "keep-alive,user-agent,cache-control,content-type,content-transfer-encoding,custom-header-1,x-accept-content-transfer-encoding,x-accept-response-streaming,x-user-agent,x-grpc-web,grpc-timeout,authorization"
                max_age: "1728000"
                expose_headers: "grpc-status,grpc-message,grpc-status-details-bin,grpc-status-details-text"
              routes:
{{.Routes}}
          http_filters:
          - name: envoy.filters.http.local_ratelimit
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit
              stat_prefix: local_rate_limiter
          - name: envoy.filters.http.cors
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.http.cors.v3.Cors
          - name: envoy.filters.ext_authz
            typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz
                transport_api_version: V3
                grpc_service:
                  timeout: 30s
                  envoy_grpc:
                    cluster_name: ext_auth
                with_request_body:
                  max_request_bytes: 1024
                  allow_partial_message: true
                allowed_headers:
                  patterns:
                    - exact: "cookie"
                    - exact: "authorization"
                    - exact: "x-real-ip"
                    - exact: "x-forwarded-for"
                    - exact: "x-rc-token"
                    - exact: "x-rc-token-2"
          - name: envoy.filters.http.grpc_web
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.http.grpc_web.v3.GrpcWeb
          - name: envoy.filters.http.router
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

  clusters:
  - name: ext_auth
    connect_timeout: 2s
    type: STRICT_DNS
    typed_extension_protocol_options:
      envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
        "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
        explicit_http_config:
          http2_protocol_options: {}
    lb_policy: ROUND_ROBIN
    load_assignment:
      cluster_name: ext_auth
      endpoints:
        - lb_endpoints:
          - endpoint:
              address:
                socket_address:
                  address: {{.AuthAdapterHost}}
                  port_value: 9000
  - name: opentelemetry_collector
    type: STRICT_DNS
    lb_policy: ROUND_ROBIN
    typed_extension_protocol_options:
      envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
        "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
        explicit_http_config:
          http2_protocol_options: {}
    load_assignment:
      cluster_name: opentelemetry_collector
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: {{.OpenTelemetryHost}}
                port_value: {{.OpenTelemetryPort}}
{{.Clusters}}
`))
)

func GenerateEnvoyConfig(cfg *APIConf, outFile string) error {
	// Build cluster type map for quick lookup
	clusterTypes := make(map[string]bool) // true = HTTP, false = gRPC
	for _, cl := range cfg.Clusters {
		clusterTypes[cl.Name] = cl.IsHTTP()
	}

	routesBuf := new(bytes.Buffer)
	for _, api := range cfg.APIsDescr {
		isHTTPCluster := clusterTypes[api.Cluster]

		// Generate route for each method with potential rate limiting
		for _, method := range api.Methods {
			routePath := api.Name + "/" + method.Name

			// Generate rate limit config if specified
			rateLimitConfig := ""
			if method.Auth != nil && method.Auth.RateLimit != nil {
				rlData := struct {
					StatPrefix    string
					MaxTokens     int
					TokensPerFill int
					FillInterval  string
				}{
					StatPrefix:    "rate_limit_" + api.Name + "_" + method.Name,
					MaxTokens:     method.Auth.RateLimit.GetMaxTokens(),
					TokensPerFill: method.Auth.RateLimit.GetTokensPerFill(),
					FillInterval:  method.Auth.RateLimit.GetFillIntervalSeconds(),
				}

				rlBuf := new(bytes.Buffer)
				err := envoyRateLimitTmpl.Execute(rlBuf, rlData)
				if err != nil {
					return err
				}
				rateLimitConfig = rlBuf.String()
			}

			// Choose template based on cluster type
			var routeTmpl *template.Template
			if isHTTPCluster {
				routeTmpl = envoyHttpRouteTmpl
			} else {
				routeTmpl = envoyGrpcRouteTmpl
			}

			routeData := struct {
				APIRoute        string
				APIName         string
				MethodName      string
				ServiceName     string
				ClusterName     string
				RateLimitConfig string
			}{
				APIRoute:        cfg.APIRoute,
				APIName:         routePath,
				MethodName:      method.Name,
				ServiceName:     api.Name,
				ClusterName:     api.Cluster,
				RateLimitConfig: rateLimitConfig,
			}

			err := routeTmpl.Execute(routesBuf, routeData)
			if err != nil {
				return err
			}
		}

		// Also generate route for the API itself (without method) - catch-all for HTTP
		if isHTTPCluster {
			// For HTTP clusters, use regex rewrite to strip service name
			routeData := struct {
				APIRoute        string
				ServiceName     string
				ClusterName     string
				RateLimitConfig string
			}{
				APIRoute:        cfg.APIRoute,
				ServiceName:     api.Name,
				ClusterName:     api.Cluster,
				RateLimitConfig: "",
			}
			err := envoyHttpApiRouteTmpl.Execute(routesBuf, routeData)
			if err != nil {
				return err
			}
		} else {
			// For gRPC clusters, keep original behavior
			routeData := struct {
				APIRoute        string
				APIName         string
				MethodName      string
				ServiceName     string
				ClusterName     string
				RateLimitConfig string
			}{
				APIRoute:        cfg.APIRoute,
				APIName:         api.Name,
				MethodName:      "",
				ServiceName:     api.Name,
				ClusterName:     api.Cluster,
				RateLimitConfig: "",
			}
			err := envoyGrpcRouteTmpl.Execute(routesBuf, routeData)
			if err != nil {
				return err
			}
		}
	}

	clustersBuf := new(bytes.Buffer)
	for _, cl := range cfg.Clusters {
		// Generate health check config if specified
		healthCheckConfig := ""
		if cl.HealthCheck != nil {
			hcData := struct {
				Path               string
				TimeoutSeconds     int
				IntervalSeconds    int
				HealthyThreshold   int
				UnhealthyThreshold int
			}{
				Path:               cl.HealthCheck.Path,
				TimeoutSeconds:     cl.HealthCheck.TimeoutSeconds,
				IntervalSeconds:    cl.HealthCheck.IntervalSeconds,
				HealthyThreshold:   cl.HealthCheck.HealthyThreshold,
				UnhealthyThreshold: cl.HealthCheck.UnhealthyThreshold,
			}

			hcBuf := new(bytes.Buffer)
			err := envoyHealthCheckTmpl.Execute(hcBuf, hcData)
			if err != nil {
				return err
			}
			healthCheckConfig = hcBuf.String()
		}

		// Generate circuit breaker config if specified
		circuitBreakerConfig := ""
		if cl.CircuitBreaker != nil {
			cbData := struct {
				MaxConnections     int
				MaxPendingRequests int
				MaxRequests        int
				MaxRetries         int
			}{
				MaxConnections:     cl.CircuitBreaker.MaxConnections,
				MaxPendingRequests: cl.CircuitBreaker.MaxPendingRequests,
				MaxRequests:        cl.CircuitBreaker.MaxRequests,
				MaxRetries:         cl.CircuitBreaker.MaxRetries,
			}

			cbBuf := new(bytes.Buffer)
			err := envoyCircuitBreakerTmpl.Execute(cbBuf, cbData)
			if err != nil {
				return err
			}
			circuitBreakerConfig = cbBuf.String()
		}

		clusterData := struct {
			ClusterName          string
			ClusterAddr          string
			ClusterPort          string
			HealthCheckConfig    string
			CircuitBreakerConfig string
		}{
			ClusterName:          cl.Name,
			ClusterAddr:          cl.AddrHost(),
			ClusterPort:          cl.AddrPort(),
			HealthCheckConfig:    healthCheckConfig,
			CircuitBreakerConfig: circuitBreakerConfig,
		}

		// Choose template based on cluster type
		var tmpl *template.Template
		if cl.IsHTTP() {
			tmpl = envoyHttpClusterTmpl
		} else {
			tmpl = envoyGrpcClusterTmpl
		}

		err := tmpl.Execute(clustersBuf, clusterData)
		if err != nil {
			return err
		}
	}

	tmplData := struct {
		Routes            string
		Clusters          string
		AuthAdapterHost   string
		OpenTelemetryHost string
		OpenTelemetryPort string
	}{
		Routes:            string(routesBuf.Bytes()),
		Clusters:          string(clustersBuf.Bytes()),
		AuthAdapterHost:   "127.0.0.1",
		OpenTelemetryHost: "127.0.0.1",
		OpenTelemetryPort: "4317",
	}

	if authAdapterHost := os.Getenv("AUTH_ADAPTER_HOST"); authAdapterHost != "" {
		tmplData.AuthAdapterHost = authAdapterHost
	}

	if otHost := os.Getenv("OPEN_TELEMETRY_HOST"); otHost != "" {
		tmplData.OpenTelemetryHost = otHost
	}

	if otPort := os.Getenv("OPEN_TELEMETRY_PORT"); otPort != "" {
		tmplData.OpenTelemetryPort = otPort
	}

	outF, err := os.Create(outFile)
	if err != nil {
		return err
	}

	return envoyConfTmpl.Execute(outF, tmplData)
}
