#!/bin/bash

cat /opt/envoy/envoy.yaml | envsubst \$JAEGER_AGENT_HOST > /etc/envoy/envoy.yaml

/usr/local/bin/envoy -c /etc/envoy/envoy.yaml -l ${LOG_LEVEL} &

/opt/auth-adapter/auth-adapter &

wait -n