#!/bin/bash

mkdir /opt/envoy && /opt/conf-generator/conf-generator -api-conf /opt/auth-adapter/config.yaml -out-envoy-conf /opt/envoy/envoy.yaml
cat /opt/envoy/envoy.yaml | envsubst \$JAEGER_AGENT_HOST > /etc/envoy/envoy.yaml

/usr/local/bin/envoy -c /etc/envoy/envoy.yaml -l ${LOG_LEVEL}