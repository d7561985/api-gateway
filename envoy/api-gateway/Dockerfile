### Build stage
FROM golang:1.20-alpine as build
WORKDIR /app/conf-generator
RUN apk add git
COPY go.mod .
RUN go mod download
COPY ./ ./
RUN CGO_ENABLED=0 GOOS=linux go build -a -o conf-generator .

### main stage
FROM envoyproxy/envoy:v1.26.2

ENV LOG_LEVEL info

RUN apt-get update && apt-get install -y curl gettext-base && apt-get clean && rm -rf /var/lib/apt/lists/*
#RUN curl -Lo /usr/local/lib/libjaegertracing_plugin.so https://github.com/jaegertracing/jaeger-client-cpp/releases/download/v0.4.2/libjaegertracing_plugin.linux_amd64.so

COPY entrypoint.sh /
COPY --from=build  /app/conf-generator/conf-generator /opt/conf-generator/conf-generator

ENTRYPOINT ["/entrypoint.sh"]