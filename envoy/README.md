# API Gateway Testing Guide

Testing guide for API Gateway built on Envoy v1.36.2 with gRPC-Web transpilation, HTTP proxy, authentication, rate limiting, and observability.

## Quick Start

### 1. Build and Run Services

```bash
docker-compose up --build
```

**Services Started:**
- `api-gateway` (Envoy) - Port 8080 (main), 8000 (admin)
- `auth-adapter` - Port 9000 (authentication service)
- `web` (fake-service gRPC) - Port 9091
- `web-http` (fake-service HTTP) - Port 9092
- `health-demo` - Port 8081 (gRPC health + streaming + header logging)
- `http-echo` - Port 8888 (header inspection service)
- `opentelemetry` - Port 55679 (tracing UI)

### 2. Verify Services Are Running

```bash
# Check all containers
docker-compose ps

# Check Envoy admin interface
curl http://localhost:8000/ready

# Check HTTP service directly
curl http://localhost:9092/
```

## API Gateway Features

Current configuration (`config.yaml`) provides:

| Feature | Endpoint | Description |
|---------|----------|-------------|
| gRPC-Web | `/api/FakeService/Handle` | gRPC service via gRPC-Web |
| HTTP Proxy | `/api/HttpService/*` | HTTP/REST service proxy |
| **Header Echo** | `/api/EchoPublic/*`, `/api/EchoProtected/*`, `/api/EchoOptional/*` | Returns all headers in JSON (for testing auth-adapter) |
| Health Check | `localhost:8000/clusters` | Envoy cluster health monitoring |
| Rate Limiting | FakeService/Handle | 10 req/min limit |

---

## Testing Endpoints

### 1. HTTP Service (Simple REST)

```bash
# Basic HTTP request through gateway
curl -v 'http://127.0.0.1:8080/api/HttpService/health'

# Expected: HTTP 200 with JSON response from fake-service
```

**Test echo endpoint:**
```bash
curl 'http://127.0.0.1:8080/api/HttpService/echo' \
  -H 'content-type: application/json' \
  -d '{"message": "test"}' \
  -v
```

### 2. gRPC-Web (Browser-compatible gRPC)

**Option A: Using grpcwebcli (recommended)**

Custom CLI tool with path prefix support located in `tools/grpcwebcli/`:

```bash
# From tools/grpcwebcli directory:
cd tools/grpcwebcli

# Simple call (empty request)
go run . -url http://127.0.0.1:8080/api -method FakeService/Handle

# With proto file and JSON data
go run . -url http://127.0.0.1:8080/api -method FakeService/Handle \
  -proto ../protos/api.proto -json '{"data": "dGVzdA=="}'

# gRPC Health Check with JSON response
go run . -url http://127.0.0.1:8080/api -method grpc.health.v1.Health/Check \
  -proto ../protos/health_check.proto

# Server streaming (gRPC-Web over HTTP)
go run . -url http://127.0.0.1:8080/api -method grpc.health.v1.Health/Watch \
  -proto ../protos/health_check.proto -stream -timeout 5s

# Protected endpoint (returns 401)
go run . -url http://127.0.0.1:8080/api -method ProtectedService/profile
```

**grpcwebcli options:**
| Flag | Description |
|------|-------------|
| `-url` | Base URL with path prefix (default: `http://127.0.0.1:8080`) |
| `-method` | Service/Method to call (e.g., `FakeService/Handle`) |
| `-proto` | Path to .proto file (enables JSON input/output) |
| `-json` | JSON request data (requires `-proto`) |
| `-stream` | Enable server streaming mode |
| `-timeout` | Request timeout (default: `30s`) |

**Option B: Using curl**

gRPC-Web requires binary framing:

```bash
# Create empty gRPC request (5-byte frame header)
printf '\x00\x00\x00\x00\x00' > /tmp/grpc-req.bin

# Send gRPC-Web request (check status only)
curl 'http://127.0.0.1:8080/api/FakeService/Handle' \
  -H 'content-type: application/grpc-web+proto' \
  -H 'x-grpc-web: 1' \
  --data-binary @/tmp/grpc-req.bin \
  -s -o /dev/null -w "HTTP: %{http_code}\n"

# Expected: HTTP: 200
```

### 3. Rate Limiting

```bash
# Test rate limit (10 requests per minute on FakeService/Handle)
for i in {1..20}; do
  echo "Request $i:"
  printf '\x00\x00\x00\x00\x00' > /tmp/grpc-req.bin
  curl 'http://127.0.0.1:8080/api/FakeService/Handle' \
    -H 'content-type: application/grpc-web+proto' \
    -H 'x-grpc-web: 1' \
    --data-binary @/tmp/grpc-req.bin \
    -w "HTTP: %{http_code}\n" -s -o /dev/null
  sleep 1
done

# Expected: First 10-20 succeed (200), then 429 Too Many Requests
```

### 4. Zero-Config Method Routing

**Key principle:** Methods don't need to be listed in config.yaml — they route automatically with service-level auth.

```bash
# Example 1: HTTP service
# config.yaml has only "health" and "echo" for HttpService
curl http://127.0.0.1:8080/api/HttpService/not-in-config
# Expected: HTTP 200 + JSON response

# Example 2: gRPC streaming
# config.yaml has only "Check" for grpc.health.v1.Health
cd tools/grpcwebcli
go run . -url http://127.0.0.1:8080/api \
  -method grpc.health.v1.Health/Watch \
  -proto ../protos/health_check.proto \
  -stream -timeout 3s
# Expected: HTTP 200 + streaming frames {"status":"SERVING"}
```

Both examples prove: gateway routes ALL methods, not just configured ones!

See [Zero-Config Method Routing](../README.md#zero-config-method-routing) for details.

### 5. CORS Preflight

```bash
# OPTIONS request (CORS preflight)
curl -X OPTIONS 'http://127.0.0.1:8080/api/FakeService/Handle' \
  -H 'origin: http://localhost:3000' \
  -H 'access-control-request-method: POST' \
  -H 'access-control-request-headers: content-type,x-grpc-web' \
  -v

# Expected: 200 with CORS headers (Access-Control-Allow-*)
```

### 6. Health Check (Envoy Cluster Monitoring)

```bash
# Check cluster health via Envoy admin
curl -s http://localhost:8000/clusters | grep "health_flags"

# Expected output:
# web::172.18.0.3:9091::health_flags::healthy
# web-http::172.18.0.4:9092::health_flags::healthy
# unhealthy-demo::172.18.0.4:9999::health_flags::/failed_active_hc

# Test unhealthy cluster returns 503
curl http://127.0.0.1:8080/api/UnhealthyService/test -w "\nHTTP: %{http_code}\n"
# Expected: HTTP: 503
```

### 7. Authorization Testing

Protected endpoints require authentication via cookie-based tokens.

**Valid test tokens:** `demo-token`, `test-token`, `valid-session`

```bash
# Test 1: No token → 401 Unauthorized
curl -w "\nHTTP: %{http_code}\n" 'http://127.0.0.1:8080/api/ProtectedService/profile'
# Expected: HTTP: 401

# Test 2: Invalid token → 401 Unauthorized
curl -w "\nHTTP: %{http_code}\n" \
  -H 'cookie: token=invalid-token' \
  'http://127.0.0.1:8080/api/ProtectedService/profile'
# Expected: HTTP: 401

# Test 3: Valid token → 200 OK
curl -w "\nHTTP: %{http_code}\n" \
  -H 'cookie: token=demo-token' \
  'http://127.0.0.1:8080/api/ProtectedService/profile'
# Expected: HTTP: 200 with response body

# Test 4: Optional policy - no token → 200 OK
curl -w "\nHTTP: %{http_code}\n" 'http://127.0.0.1:8080/api/ProtectedService/data'
# Expected: HTTP: 200 (auth is optional for /data endpoint)

# Test 5: Optional policy - valid token → 200 OK with user context
curl -w "\nHTTP: %{http_code}\n" \
  -H 'cookie: token=demo-token' \
  'http://127.0.0.1:8080/api/ProtectedService/data'
# Expected: HTTP: 200
```

### 8. Header Enrichment Testing (http-echo)

The `http-echo` service returns all received headers in JSON format. Use it to verify auth-adapter injects `user-id` and `session-id` headers.

```bash
# Test 1: Public endpoint (no auth headers expected)
curl -s http://127.0.0.1:8080/api/EchoPublic/headers | jq '.headers | {"user-id", "session-id"}'
# Expected: both null

# Test 2: Protected endpoint without cookie (should fail)
curl -s -w "\nHTTP: %{http_code}\n" http://127.0.0.1:8080/api/EchoProtected/headers
# Expected: HTTP: 401

# Test 3: Protected endpoint with valid cookie (should show enriched headers)
curl -s http://127.0.0.1:8080/api/EchoProtected/headers \
  -H 'cookie: token=demo-token' | jq '.headers | {"user-id", "session-id"}'
# Expected: user-id and session-id populated by auth-adapter

# Test 4: Optional auth endpoint
curl -s http://127.0.0.1:8080/api/EchoOptional/headers | jq '.headers | keys'
# Expected: HTTP 200 with all Envoy headers (no user-id without cookie)

# Test 5: View ALL headers received by backend
curl -s http://127.0.0.1:8080/api/EchoPublic/headers | jq '.headers'
```

**gRPC header logging:**
The `health-demo` service logs all gRPC metadata (headers) to stdout. Check logs after gRPC requests:
```bash
docker-compose logs -f health-demo
```

---

## Monitoring

### Envoy Admin Interface

```bash
# Cluster health status
curl http://localhost:8000/clusters | grep -E "(web|health)"

# Rate limiting stats
curl http://localhost:8000/stats | grep rate_limit

# All routes
curl http://localhost:8000/config_dump | jq '.configs[] | select(.["@type"] | contains("RoutesConfigDump"))'
```

### OpenTelemetry Tracing

```bash
# Open tracing UI
open http://localhost:55679

# Make request with trace header
printf '\x00\x00\x00\x00\x00' > /tmp/grpc-req.bin
curl 'http://127.0.0.1:8080/api/FakeService/Handle' \
  -H 'content-type: application/grpc-web+proto' \
  -H 'x-grpc-web: 1' \
  -H 'x-trace-id: test-trace-123' \
  --data-binary @/tmp/grpc-req.bin
```

---

## Configuration Reference

### config.yaml Structure

```yaml
api_route: /api/  # URL prefix for all routes

clusters:
  - name: web
    addr: "web:9091"
    type: "grpc"           # or "http"
    health_check:          # optional
      path: "/health"
      interval_seconds: 30
    circuit_breaker:       # optional
      max_connections: 100

apis:
  - name: FakeService      # gRPC service name (see naming rules below)
    cluster: web
    auth:
      policy: "no-need"    # DEFAULT for ALL methods (zero-config routing)
    methods:               # OPTIONAL! Only for method-specific overrides
      - name: Handle       # Override: add rate limit to this method
        auth:
          policy: no-need
          rate_limit:
            period: "1m"   # 1s | 1m | 1h
            count: 10
            delay: "3s"
    # All other methods (GetStatus, Process, etc.) route automatically
    # with service-level auth policy "no-need"
```

### gRPC-Web Testing Limitations with Path Prefix

When `api_route` is set (e.g., `/api/`), CLI tools like **Evans** and **grpcurl** cannot test gRPC-Web through the gateway because they don't support URL path prefixes:

```bash
# These tools construct URLs as: http://host:port/Service/Method
# But gateway expects: http://host:port/api/Service/Method
evans --web --host 127.0.0.1 --port 8080  # ✗ Won't work with /api/ prefix
grpcurl -plaintext 127.0.0.1:8080         # ✗ Won't work with /api/ prefix
```

**Testing options:**
1. **grpcwebcli** — custom tool in `tools/grpcwebcli/` with path prefix support (recommended)
2. **curl** — use binary gRPC-Web requests (see examples above)
3. **Direct backend** — connect Evans/grpcurl to backend port (e.g., 9091)
4. **Frontend client** — production JavaScript clients work correctly

**Frontend clients work fine** — just specify base URL without trailing slash:
```javascript
// ✓ Correct - grpc-web concatenates: hostname + "/Service/Method"
const client = new FakeServiceClient('http://gateway:8080/api');

// ✗ Wrong - trailing slash causes double slash
const client = new FakeServiceClient('http://gateway:8080/api/');
```

### gRPC Service Naming & Method Routing

See [main README](../README.md#grpc-service-naming) for detailed documentation on:
- Service naming conventions (`package.Service` vs `Service`)
- Method routing behavior (configured vs unconfigured methods)

### Auth Policies

| Policy | Description |
|--------|-------------|
| `no-need` | Public endpoint, no auth required |
| `optional` | Auth checked if token present |
| `required` | Auth required, permission checked |

---

## Troubleshooting

### Common Issues

**1. 404 Not Found:**
- Check `api_route` in config.yaml matches your request path
- Verify the API/method is defined in config

**2. 503 Service Unavailable:**
- Backend service not running or unhealthy
- Check: `docker-compose ps` and `curl http://localhost:8000/clusters`

**3. gRPC-Web request fails:**
- Ensure binary data is sent correctly (use file approach, not `$'...'` in zsh)
- Check headers: `content-type: application/grpc-web+proto` and `x-grpc-web: 1`

**4. Rate limit not working:**
- Envoy uses token bucket: `max_tokens = count * 2`, refills `count` per period
- First requests use burst capacity

### Debug Commands

```bash
# Check generated Envoy config
docker exec envoy-api-gateway-1 cat /etc/envoy/envoy.yaml

# View Envoy logs
docker-compose logs -f api-gateway

# Test backend directly
curl http://localhost:9092/  # HTTP service
```

---

## Development

### Regenerate Envoy Config

```bash
cd api-gateway
go run . -api-conf ../config.yaml -out-envoy-conf ../envoy.yaml
```

### Validate Config

```bash
docker run --rm \
  -v "$(pwd)/envoy.yaml:/etc/envoy/envoy.yaml:ro" \
  envoyproxy/envoy:v1.36.2 \
  --mode validate \
  --config-path /etc/envoy/envoy.yaml
```

---

## Verification Checklist

Use this checklist to verify API Gateway is fully operational after deployment or changes.

### Prerequisites
```bash
cd tools/grpcwebcli
```

| # | Feature | Command | Expected | ✓ |
|---|---------|---------|----------|---|
| **1** | **Services Running** | `docker-compose ps` | All containers "Up" | ☐ |
| **2** | **Envoy Ready** | `curl http://localhost:8000/ready` | "LIVE" | ☐ |
| **3** | **HTTP Proxy** | `curl http://127.0.0.1:8080/api/HttpService/health` | HTTP 200 + JSON | ☐ |
| **4** | **gRPC-Web (unary)** | `go run . -url http://127.0.0.1:8080/api -method FakeService/Handle` | HTTP 200 + DATA frame | ☐ |
| **5** | **gRPC-Web (streaming)** | `go run . -url http://127.0.0.1:8080/api -method grpc.health.v1.Health/Watch -proto ../protos/health_check.proto -stream -timeout 3s` | Multiple DATA frames | ☐ |
| **6** | **Auth: no token** | `curl http://127.0.0.1:8080/api/ProtectedService/profile -w "\nHTTP: %{http_code}\n"` | HTTP 401 | ☐ |
| **7** | **Auth: invalid token** | `curl -H 'cookie: token=invalid' http://127.0.0.1:8080/api/ProtectedService/profile -w "\nHTTP: %{http_code}\n"` | HTTP 401 | ☐ |
| **8** | **Auth: valid token** | `curl -H 'cookie: token=demo-token' http://127.0.0.1:8080/api/ProtectedService/profile -w "\nHTTP: %{http_code}\n"` | HTTP 200 | ☐ |
| **9** | **Auth: optional (no token)** | `curl http://127.0.0.1:8080/api/ProtectedService/data -w "\nHTTP: %{http_code}\n"` | HTTP 200 | ☐ |
| **10** | **CORS Preflight** | `curl -X OPTIONS http://127.0.0.1:8080/api/FakeService/Handle -H 'origin: http://test.com' -H 'access-control-request-method: POST' -I` | 200 + CORS headers | ☐ |
| **11** | **Rate Limiting** | Run rate limit script below | First ~10 OK, then 429 | ☐ |
| **12** | **Zero-config (HTTP)** | `curl http://127.0.0.1:8080/api/HttpService/not-in-config` | HTTP 200 + JSON | ☐ |
| **13** | **Zero-config (gRPC)** | Test #5 uses `Watch` method NOT in config | Streaming works! | ☐ |
| **14** | **Health check: healthy** | `curl -s http://localhost:8000/clusters \| grep web-http` | `health_flags::healthy` | ☐ |
| **15** | **Health check: unhealthy** | `curl http://127.0.0.1:8080/api/UnhealthyService/test -w "\nHTTP: %{http_code}\n"` | HTTP 503 | ☐ |
| **16** | **Header echo (public)** | `curl -s http://127.0.0.1:8080/api/EchoPublic/headers \| jq '.headers \| keys'` | Array of headers | ☐ |
| **17** | **Header echo (protected)** | `curl -s http://127.0.0.1:8080/api/EchoProtected/headers -w "\nHTTP: %{http_code}\n"` | HTTP 401 | ☐ |
| **18** | **Header enrichment** | `curl -s -H 'cookie: token=demo-token' http://127.0.0.1:8080/api/EchoProtected/headers \| jq '.headers \| {"user-id","session-id"}'` | user-id + session-id | ☐ |

**Zero-config explained:**
- Test #12: `/HttpService/not-in-config` is NOT in config (only `health`, `echo`)
- Test #13: `Watch` method is NOT in config (only `Check`)
Both route and return REAL data — proves zero-config routing works!

### Rate Limit Test Script
```bash
for i in {1..15}; do
  echo -n "Request $i: "
  printf '\x00\x00\x00\x00\x00' | curl -s -o /dev/null -w "%{http_code}\n" \
    'http://127.0.0.1:8080/api/FakeService/Handle' \
    -H 'content-type: application/grpc-web+proto' \
    -H 'x-grpc-web: 1' --data-binary @-
  sleep 0.5
done
```

---

**Verified with:** Envoy v1.36.2, nicholasjackson/fake-service:v0.19.1, mendhak/http-https-echo
