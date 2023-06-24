FROM launcher:latest as xxx

ADD config.yaml /opt/auth-adapter/config.yaml

ENV PROJECT_NAME namespace
ENV SERVICE_NAME service

# disable for test
ENV OTEL_ENABLE false

RUN mkdir /opt/envoy && /opt/conf-generator/conf-generator -api-conf /opt/auth-adapter/config.yaml -out-envoy-conf /opt/envoy/envoy.yaml
