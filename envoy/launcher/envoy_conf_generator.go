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
                  max_grpc_timeout: 600s
                  prefix_rewrite: "/{{.APIName}}"
`))
	envoyClusterTmpl = template.Must(template.New("clusterTmpl").Parse(`
  - name: {{.ClusterName}}
    connect_timeout: 5s
    type: strict_dns
    http2_protocol_options: {}
    lb_policy: round_robin
    hosts: [{ socket_address: { address: "{{.ClusterAddr}}", port_value: {{.ClusterPort}} }}]
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
      - name: envoy.http_connection_manager
        config:
          generate_request_id: true
          tracing:
            operation_name: egress
          codec_type: auto
          stat_prefix: ingress_http
          route_config:
            name: local_route
            virtual_hosts:
            - name: grpc_proxy
              domains: ["*"]
              response_headers_to_remove: ["grpc-message"]
              routes:
{{.Routes}}
          http_filters:
          - name: envoy.ext_authz
            config:
                grpc_service:
                    envoy_grpc:
                        cluster_name: ext_auth
                    timeout: 30s
          - name: envoy.grpc_web
          - name: envoy.router

  clusters:
  - name: ext_auth
    connect_timeout: 2s
    type: static
    http2_protocol_options: {}
    lb_policy: round_robin
    hosts: [{ socket_address: { address: {{.AuthAdapterHost}}, port_value: 9000 }}]
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
		Routes          string
		Clusters        string
		AuthAdapterHost string
	}{
		Routes:          string(routesBuf.Bytes()),
		Clusters:        string(clustersBuf.Bytes()),
		AuthAdapterHost: "127.0.0.1",
	}

	if authAdapterHost := os.Getenv("AUTH_ADAPTER_HOST"); authAdapterHost != "" {
		tmplData.AuthAdapterHost = authAdapterHost
	}

	outF, err := os.Create(outFile)
	if err != nil {
		return err
	}

	return envoyConfTmpl.Execute(outF, tmplData)
}
