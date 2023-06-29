package main

import (
	"bytes"
	"os"
	"text/template"
)

var (
	envoyRouteTmpl = template.Must(template.New("routeTmpl").Parse(`
              - match: { prefix: "{{.APIRoute}}{{.APIName}}" }
                route:
                  cluster: {{.ClusterName}}
                  timeout: 0s
                  prefix_rewrite: "/{{.APIName}}"
                  max_stream_duration:
                    max_stream_duration: 600s
                    grpc_timeout_header_max: 0s
`))
	envoyClusterTmpl = template.Must(template.New("clusterTmpl").Parse(`
  - name: {{.ClusterName}}
    connect_timeout: 5s
    type: STRICT_DNS
    http2_protocol_options: {}
    lb_policy: ROUND_ROBIN
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
              routes:
{{.Routes}}
          http_filters:
          - name: envoy.filters.ext_authz
            typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz
                transport_api_version: V3
                grpc_service:
                  timeout: 30s
                  envoy_grpc:
                    cluster_name: ext_auth
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
	routesBuf := new(bytes.Buffer)
	for _, api := range cfg.APIsDescr {
		routeData := struct {
			APIRoute    string
			APIName     string
			ClusterName string
		}{
			APIRoute:    cfg.APIRoute,
			APIName:     api.Name,
			ClusterName: api.Cluster,
		}
		err := envoyRouteTmpl.Execute(routesBuf, routeData)
		if err != nil {
			return err
		}
	}

	clustersBuf := new(bytes.Buffer)
	for _, cl := range cfg.Clusters {
		clusterData := struct {
			ClusterName string
			ClusterAddr string
			ClusterPort string
		}{
			ClusterName: cl.Name,
			ClusterAddr: cl.AddrHost(),
			ClusterPort: cl.AddrPort(),
		}
		err := envoyClusterTmpl.Execute(clustersBuf, clusterData)
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
