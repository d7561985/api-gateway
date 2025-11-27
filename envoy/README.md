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
- `health-demo` - Port 8081 (health checks)
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
| Health Check | `/api/grpc.health.v1.Health/Check` | gRPC health checking |
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

gRPC-Web requires binary framing. Use file-based approach for curl:

```bash
# Create empty gRPC request (5-byte frame header)
printf '\x00\x00\x00\x00\x00' > /tmp/grpc-req.bin

# Send gRPC-Web request
curl 'http://127.0.0.1:8080/api/FakeService/Handle' \
  -H 'content-type: application/grpc-web+proto' \
  -H 'x-grpc-web: 1' \
  --data-binary @/tmp/grpc-req.bin \
  -v

# Expected: HTTP 200 with gRPC-Web response
```

### 3. Rate Limiting

```bash
# Test rate limit (10 requests per minute on FakeService/Handle)
for i in {1..12}; do
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

### 4. Unconfigured Method Routing

```bash
# Call a method not explicitly configured in config.yaml
# Should route to backend using service-level auth, backend returns UNIMPLEMENTED
printf '\x00\x00\x00\x00\x00' > /tmp/grpc-req.bin
curl 'http://127.0.0.1:8080/api/FakeService/NonExistentMethod' \
  -H 'content-type: application/grpc-web+proto' \
  -H 'x-grpc-web: 1' \
  --data-binary @/tmp/grpc-req.bin \
  -v 2>&1 | grep "grpc-status"

# Expected: grpc-status: 12 (UNIMPLEMENTED)
```

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

### 6. Health Check

```bash
# gRPC Health Check via gateway
printf '\x00\x00\x00\x00\x00' > /tmp/grpc-req.bin
curl 'http://127.0.0.1:8080/api/grpc.health.v1.Health/Check' \
  -H 'content-type: application/grpc-web+proto' \
  -H 'x-grpc-web: 1' \
  --data-binary @/tmp/grpc-req.bin \
  -v
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
      policy: "no-need"    # service-level auth (fallback for unconfigured methods)
    methods:
      - name: Handle       # only method name, without service prefix
        auth:
          policy: no-need
          rate_limit:
            period: "1m"   # 1s | 1m | 1h
            count: 10
            delay: "3s"
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
1. **curl** — use binary gRPC-Web requests (see examples above)
2. **Direct backend** — connect Evans/grpcurl to backend port (e.g., 9091)
3. **Frontend client** — production JavaScript clients work correctly

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

**Verified with:** Envoy v1.36.2, nicholasjackson/fake-service:v0.19.1
