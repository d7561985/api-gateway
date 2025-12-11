# API Gateway Configuration Generator

Generates optimized Envoy configurations for gRPC and HTTP services with integrated authentication, rate limiting, and observability.

## Features

### ✅ Configuration Generator
- **Declarative YAML config** → Optimized Envoy configuration
- **Mixed protocol support**: gRPC (`type: grpc`) and HTTP (`type: http`) services
- **Authentication policies**: `required`, `optional`, `no-need`
- **CORS configuration**: Production-ready with security warnings
- **OpenTelemetry integration**: Automatic distributed tracing

### ✅ Auth-Adapter Integration (External Service)

The API Gateway integrates with `auth-adapter` service via Envoy's `ext_authz` filter, providing:

#### **Header Enrichment (IMPLEMENTED)** 
```
Browser/Client → API Gateway → auth-adapter → Backend Service
                                     ↓
                            Automatic header injection:
                            • user-id: "12345"  
                            • session-id: "sess_abc123"
```

**How it works:**
1. Request hits API Gateway 
2. `ext_authz` filter calls auth-adapter for validation
3. Auth-adapter validates session and adds headers:
   - `user-id`: User identifier from session
   - `session-id`: Session identifier  
4. Headers automatically forwarded to backend services

#### **Rate Limiting (IMPLEMENTED)**
- **Per-IP rate limiting** with token bucket algorithm
- **Configurable periods**: `{period: "1m", count: 3, delay: "3s"}`
- **Method-specific limits**: Different limits per API endpoint
- **reCAPTCHA bypass**: Rate limits reset on successful reCAPTCHA

#### **reCAPTCHA Integration (IMPLEMENTED)**
- **v2 & v3 support**: Both challenge and score-based validation
- **Automatic integration**: Triggered on rate limit violations
- **Headers**: `x-rc-token` (v3) and `x-rc-token-2` (v2)

#### **Session & Permission Validation**
- **gRPC session service** integration
- **Role-based authorization**: Permission checking against user roles
- **Cookie-based sessions**: Automatic token extraction and validation

### ⚠️ Current Limitation

**Config generator does NOT process auth-adapter specific fields:**
```yaml
# These fields are IGNORED by config generator:
methods:
  - name: "LoginPublic"
    auth:
      rate_limit: {period: "1m", count: 3}  # ❌ NOT PROCESSED
      need_recaptcha: true                  # ❌ NOT PROCESSED
```

The auth-adapter reads these directly from config file, but Envoy configuration doesn't include rate limiting filters.

## Environment Variables

### Required for API Gateway
* `AUTH_ADAPTER_HOST` - Auth service host (default: 127.0.0.1)
* `OPEN_TELEMETRY_HOST` - OpenTelemetry collector host (default: 127.0.0.1)  
* `OPEN_TELEMETRY_PORT` - OpenTelemetry gRPC port (default: 4317)

### Required for Auth-Adapter
* `RECAPTCHA_URL` - Google reCAPTCHA validation URL
* `RECAPTCHA_SECRET_V2` - reCAPTCHA v2 secret key
* `RECAPTCHA_SECRET_V3` - reCAPTCHA v3 secret key  
* `AUTH_SERVICE_ADDR` - Backend auth service address

## Architecture Overview

```
┌─────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Client    │───▶│   API Gateway   │───▶│  Auth-Adapter   │
│  (Browser)  │    │    (Envoy)      │    │   (ext_authz)   │
└─────────────┘    └─────────────────┘    └─────────────────┘
                            │                        │
                            ▼                        ▼
                   ┌─────────────────┐    ┌─────────────────┐
                   │ Backend Service │    │  Auth Service   │
                   │ + enriched      │    │   (sessions)    │
                   │   headers       │    │                 │
                   └─────────────────┘    └─────────────────┘
```

## Example Configuration

```yaml
# Full integration example
api_route: /api/v1/

clusters:
  - name: user_service
    addr: "user-service:9000"
    type: "grpc"
    
apis:
  - name: "UserService"
    cluster: "user_service"
    auth:
      policy: "required"
      permission: "user:read"
    methods:
      - name: "GetProfile"
        auth:
          policy: "required"
          rate_limit: {period: "1m", count: 10, delay: "1s"}
      - name: "UpdateProfile"  
        auth:
          policy: "required"
          rate_limit: {period: "1m", count: 3, delay: "5s"}
          need_recaptcha: true
```

**Result:** Backend service receives requests with automatic headers:
```
GET /UserService/GetProfile
user-id: 12345
session-id: sess_abc123
authorization: Bearer token...
```

## Testing Header Enrichment

### HTTP Echo Service (mendhak/http-https-echo)

The `http-echo` service returns all received headers in JSON format, perfect for verifying auth-adapter header injection.

**Start services:**
```bash
cd envoy
docker-compose up -d
```

**Test 1: Public endpoint (no auth) - baseline:**
```bash
curl -s http://localhost:8080/api/EchoPublic/headers | jq '.headers'
```
Expected: No `user-id` or `session-id` headers.

**Test 2: Protected endpoint WITHOUT auth cookie - should fail:**
```bash
curl -s -w "\nHTTP Status: %{http_code}\n" http://localhost:8080/api/EchoProtected/headers
```
Expected: 401 Unauthorized.

**Test 3: Protected endpoint WITH auth cookie - should show enriched headers:**
```bash
curl -s http://localhost:8080/api/EchoProtected/headers \
  -H "Cookie: session=YOUR_VALID_SESSION_TOKEN" | jq '.headers'
```
Expected response includes:
```json
{
  "user-id": "12345",
  "session-id": "sess_abc123",
  ...
}
```

**Test 4: Optional auth - with and without cookie:**
```bash
# Without auth - should work, no user headers
curl -s http://localhost:8080/api/EchoOptional/headers | jq '.headers | {"user-id", "session-id"}'

# With auth - should show enriched headers
curl -s http://localhost:8080/api/EchoOptional/headers \
  -H "Cookie: session=YOUR_VALID_SESSION_TOKEN" | jq '.headers | {"user-id", "session-id"}'
```

### gRPC Health Demo (header logging)

The `health-demo` service logs all incoming gRPC metadata (headers) to stdout, making it easy to verify header enrichment for gRPC services.

**View health-demo logs:**
```bash
docker-compose logs -f health-demo
```

**Trigger gRPC Health Check through API Gateway:**
```bash
# Using grpcurl (requires proto file or reflection)
grpcurl -plaintext -protoset health.protoset localhost:8080 grpc.health.v1.Health/Check

# Or use grpc-web from browser/Node.js client
```

**Expected log output in health-demo (when auth headers are passed):**
```
=== gRPC Health Check - Received Headers ===
{
  "user-id": ["12345"],
  "session-id": ["sess_abc123"],
  ":authority": ["localhost:8080"],
  "content-type": ["application/grpc"],
  ...
}
>>> user-id: 12345
>>> session-id: sess_abc123
```

### Quick Verification Script

```bash
#!/bin/bash
PORT=${1:-18080}
echo "=== Header Enrichment Test Suite (port: $PORT) ==="

echo -e "\n1. Public endpoint (no auth headers expected):"
curl -s http://localhost:$PORT/api/EchoPublic/test | jq -r '.headers | to_entries | .[] | select(.key | test("user-id|session-id")) | "\(.key): \(.value)"' || echo "  (none found - expected)"

echo -e "\n2. Protected endpoint without auth (should return 401):"
curl -s -w "HTTP Status: %{http_code}\n" http://localhost:$PORT/api/EchoProtected/headers -o /dev/null

echo -e "\n3. Optional endpoint without auth:"
result=$(curl -s http://localhost:$PORT/api/EchoOptional/headers | jq -r '.headers | to_entries | .[] | select(.key | test("user-id|session-id")) | "\(.key): \(.value)"')
if [ -z "$result" ]; then
  echo "  No auth headers (expected without cookie)"
else
  echo "$result"
fi

echo -e "\n4. All headers on public endpoint:"
curl -s http://localhost:$PORT/api/EchoPublic/headers | jq '.headers | keys'
```

### Verifying Auth-Adapter Header Enrichment

To fully test header enrichment, you need a valid session. The flow is:

1. **Authenticate** with your auth service to get a session cookie
2. **Use the cookie** in requests to protected/optional endpoints
3. **Observe** `user-id` and `session-id` headers in the response

```bash
# Example with valid session
curl -s http://localhost:8080/api/EchoProtected/profile \
  -H "Cookie: session=<your-valid-session-token>" | jq '{
    path: .path,
    user_id: .headers["user-id"],
    session_id: .headers["session-id"],
    all_headers: (.headers | keys)
  }'
```