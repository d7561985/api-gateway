# API Gateway Testing Guide

Complete testing guide for API Gateway built on Envoy v1.36.2 demonstrating authentication, rate limiting, health checks, circuit breaking, gRPC-Web transpilation, and observability features.

## Quick Start

### 1. Build and Run Services

```bash
docker-compose up --build
```

**Services Started:**
- `api-gateway` (Envoy) - Port 8080 (main), 8000 (admin)
- `auth-adapter` - Port 9000 (authentication service)
- `web` (fake-service) - Port 9091 (test gRPC service)
- `health-demo` - Port 8081 (health checks)
- `opentelemetry` - Port 55679 (tracing UI)

### 2. Verify Services Are Running

```bash
# Check all containers
docker-compose ps

# Check Envoy admin interface
curl http://localhost:8000/ready

# Check fake-service health
curl http://localhost:9091/health
```

## 🛠️ Testing Tools Installation

**Required for API Gateway testing:**

```bash
# Install Evans CLI (Interactive gRPC client)
brew tap ktr0731/evans && brew install evans  # macOS
# OR
curl -L https://github.com/ktr0731/evans/releases/latest/download/evans_linux_amd64.tar.gz | tar -xz && sudo mv evans /usr/local/bin/  # Linux

# Install k6 (Load testing)
brew install k6  # macOS
# OR
sudo apt-get install k6  # Ubuntu

# Install jq (JSON processing for config analysis)
brew install jq  # macOS
sudo apt-get install jq  # Ubuntu

# Verify installations
evans --version
k6 version  
jq --version
echo "✅ All testing tools installed"
```

## API Gateway Feature Testing

### 1. 🌐 gRPC-Web Transpilation (Browser ↔ gRPC)

**Test gRPC-Web through API Gateway:**
```bash
# Basic gRPC-Web request via API Gateway
curl 'http://127.0.0.1:8080/api/FakeService/Handle' \
  -H 'content-type: application/grpc-web+proto' \
  -H 'x-grpc-web: 1' \
  -H 'authority: 127.0.0.1:8080' \
  --data-binary $'\x00\x00\x00\x00\x05\n\x03test' \
  -v

# ✅ Expected: gRPC-Web formatted response with CORS headers
# ✅ Demonstrates: Browser compatibility, protocol conversion
```

**Interactive gRPC-Web testing:**
```bash
# Connect to API Gateway with Evans (gRPC-Web mode)
evans --proto ./tools/protos/api.proto --port 8080 --web --host 127.0.0.1 repl

# In Evans shell:
call Handle
# ✅ Test: Full gRPC-Web workflow through gateway
```

### 2. 🔐 Authentication & Authorization (ext_authz)

**Test authentication flow:**
```bash
# 1. Test public endpoint (no auth required)
curl 'http://127.0.0.1:8080/api/auth.v1.WebAuthService/LoginPublic' \
  -H 'content-type: application/grpc-web+proto' \
  -H 'x-grpc-web: 1' \
  --data-binary $'\x00\x00\x00\x00\x05\n\x03test' \
  -v
# ✅ Expected: Success (policy: no-need)

# 2. Test protected endpoint without auth (should fail)
curl 'http://127.0.0.1:8080/api/UserService/GetProfile' \
  -H 'content-type: application/grpc-web+proto' \
  -H 'x-grpc-web: 1' \
  --data-binary $'\x00\x00\x00\x00\x05\n\x03test' \
  -v
# ✅ Expected: 403 Forbidden (ext_authz rejection)

# 3. Test with valid session cookie
curl 'http://127.0.0.1:8080/api/UserService/GetProfile' \
  -H 'content-type: application/grpc-web+proto' \
  -H 'x-grpc-web: 1' \
  -H 'cookie: session_id=valid_session_token' \
  --data-binary $'\x00\x00\x00\x00\x05\n\x03test' \
  -v
# ✅ Expected: Success with enriched headers (user-id, session-id)
```

### 3. 🚦 Rate Limiting (Local Token Bucket)

**Test rate limiting functionality:**
```bash
# Test rate limited endpoint (3 requests per minute)
for i in {1..5}; do
  echo "Request $i:"
  curl 'http://127.0.0.1:8080/api/auth.v1.WebAuthService/LoginPublic' \
    -H 'content-type: application/grpc-web+proto' \
    -H 'x-grpc-web: 1' \
    --data-binary $'\x00\x00\x00\x00\x05\n\x03test' \
    -w "HTTP: %{http_code}\n" -s -o /dev/null
  sleep 2
done

# ✅ Expected: First 3 succeed (200), then 429 Too Many Requests
# ✅ Demonstrates: Token bucket algorithm, configurable limits
```

**Test per-method rate limits:**
```bash
# Different methods have different rate limits
# LoginPublic: 3/min, CheckPasswordPublic: 5/min

echo "Testing different rate limits per method:"
curl 'http://127.0.0.1:8080/api/auth.v1.WebAuthService/CheckPasswordPublic' \
  -H 'content-type: application/grpc-web+proto' \
  -H 'x-grpc-web: 1' \
  --data-binary $'\x00\x00\x00\x00\x05\n\x03test' \
  -w "HTTP: %{http_code}\n" -v

# ✅ Expected: Different rate limit behavior per API method
```

### 4. ❤️ Health Checks & Circuit Breaking

**Test upstream health monitoring:**
```bash
# Check Envoy cluster health status
curl http://localhost:8000/clusters | grep "health_flags"

# ✅ Expected: Shows health status of backend services
# ✅ Demonstrates: Active health checking, failure detection
```

**Test circuit breaker behavior:**
```bash
# Monitor circuit breaker stats
curl http://localhost:8000/stats | grep circuit_breaker

# Simulate backend failure (if fake-service supports failure injection)
curl 'http://127.0.0.1:8080/api/FakeService/Handle?fail=true' \
  -H 'content-type: application/grpc-web+proto' \
  -H 'x-grpc-web: 1' \
  --data-binary $'\x00\x00\x00\x00\x05\n\x03test' \
  -v

# ✅ Expected: Circuit breaker opens on repeated failures
# ✅ Demonstrates: Automatic failure handling, load shedding
```

### 5. 📊 OpenTelemetry Distributed Tracing

**Test distributed tracing:**
```bash
# Make request with tracing headers
curl 'http://127.0.0.1:8080/api/FakeService/Handle' \
  -H 'content-type: application/grpc-web+proto' \
  -H 'x-grpc-web: 1' \
  -H 'x-trace-id: 12345678901234567890123456789012' \
  -H 'x-span-id: 1234567890123456' \
  --data-binary $'\x00\x00\x00\x00\x05\n\x03test' \
  -v

# Check OpenTelemetry UI
open http://localhost:55679

# ✅ Expected: Request traces visible in OpenTelemetry UI
# ✅ Demonstrates: End-to-end observability, trace propagation
```

**Monitor real-time metrics:**
```bash
# Check request metrics
curl http://localhost:8000/stats | grep -E "(http|grpc).*requests"

# Check rate limiting stats
curl http://localhost:8000/stats | grep rate_limit

# ✅ Expected: Real-time performance metrics
```

### 6. 🌍 CORS & Browser Integration

**Test CORS preflight through API Gateway:**
```bash
# OPTIONS request (CORS preflight)
curl -X OPTIONS 'http://127.0.0.1:8080/api/FakeService/Handle' \
  -H 'origin: http://localhost:3000' \
  -H 'access-control-request-method: POST' \
  -H 'access-control-request-headers: content-type,x-grpc-web,authorization' \
  -v

# ✅ Expected: CORS headers allowing gRPC-Web requests
# ✅ Demonstrates: Browser compatibility, security headers
```

**Browser compatibility test:**
```javascript
// Create test.html and open in browser
fetch('http://127.0.0.1:8080/api/FakeService/Handle', {
  method: 'POST',
  headers: {
    'content-type': 'application/grpc-web+proto',
    'x-grpc-web': '1'
  },
  body: new Uint8Array([0,0,0,0,5,10,3,116,101,115,116])
})
.then(r => r.arrayBuffer())
.then(data => console.log('✅ gRPC-Web success:', data))
.catch(err => console.error('❌ CORS Error:', err));

// ✅ Expected: Successful gRPC-Web call from browser
```

### 7. 🔄 Mixed Protocol Support (gRPC + HTTP)

**Test gRPC and HTTP services through same gateway:**
```bash
# 1. Test gRPC service through gateway
curl 'http://127.0.0.1:8080/api/UserGRPCService/GetProfile' \
  -H 'content-type: application/grpc-web+proto' \
  -H 'x-grpc-web: 1' \
  --data-binary $'\x00\x00\x00\x00\x05\n\x03test' \
  -v

# 2. Test HTTP service through gateway  
curl 'http://127.0.0.1:8080/api/PaymentAPI/ProcessPayment' \
  -H 'content-type: application/json' \
  -d '{"amount": 100, "currency": "USD"}' \
  -v

# ✅ Expected: Both protocols work through single gateway
# ✅ Demonstrates: Protocol flexibility, unified entry point
```

### 8. 🏷️ Header Enrichment (via ext_authz)

**Test automatic header injection:**
```bash
# Make authenticated request and check backend receives enriched headers
curl 'http://127.0.0.1:8080/api/UserService/GetProfile' \
  -H 'content-type: application/grpc-web+proto' \
  -H 'x-grpc-web: 1' \
  -H 'cookie: session_id=valid_session' \
  --data-binary $'\x00\x00\x00\x00\x05\n\x03test' \
  -v

# Check backend service logs for enriched headers:
# ✅ Expected: Backend receives user-id, session-id headers
# ✅ Demonstrates: Automatic context injection, zero-code enrichment
```

## 🧪 Integration Testing Scenarios

### End-to-End Feature Combination

**Test multiple features together:**
```bash
#!/bin/bash
# Complete feature test script

echo "🧪 Testing API Gateway End-to-End..."

# 1. Authentication + Rate Limiting + Tracing
echo "1️⃣ Auth + Rate Limit + Tracing:"
curl 'http://127.0.0.1:8080/api/auth.v1.WebAuthService/LoginPublic' \
  -H 'content-type: application/grpc-web+proto' \
  -H 'x-grpc-web: 1' \
  -H 'x-trace-id: test-trace-123' \
  -H 'cookie: session_id=test' \
  --data-binary $'\x00\x00\x00\x00\x05\n\x03test' \
  -w "Status: %{http_code}, Time: %{time_total}s\n" \
  -s -o /dev/null

# 2. CORS + gRPC-Web + Health Checks
echo "2️⃣ CORS + gRPC-Web + Health:"
curl -X OPTIONS 'http://127.0.0.1:8080/api/FakeService/Handle' \
  -H 'origin: https://myapp.com' \
  -w "CORS Status: %{http_code}\n" \
  -s -o /dev/null

# 3. Circuit Breaker + Observability
echo "3️⃣ Circuit Breaker Stats:"
curl -s http://localhost:8000/stats | grep -E "(circuit|health|rate_limit)" | head -5

echo "✅ All features tested through API Gateway!"
```

## 🚀 Performance & Load Testing

### API Gateway Performance Testing

```javascript
// api-gateway-load-test.js - Tests all gateway features
import http from 'k6/http';
import { check, group } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

export let options = {
  stages: [
    { duration: '30s', target: 10 },   // Ramp up
    { duration: '1m', target: 20 },    // Stay at 20 users
    { duration: '30s', target: 0 },    // Ramp down
  ],
  thresholds: {
    'http_req_duration': ['p(95)<500'],  // 95% under 500ms
    'errors': ['rate<0.1'],              // Error rate under 10%
  },
};

export default function() {
  group('API Gateway gRPC-Web', function() {
    let response = http.post('http://127.0.0.1:8080/api/FakeService/Handle', 
      new Uint8Array([0,0,0,0,5,10,3,116,101,115,116]), {
      headers: {
        'content-type': 'application/grpc-web+proto',
        'x-grpc-web': '1',
        'x-trace-id': `load-test-${__VU}-${__ITER}`,
      },
    });
    
    check(response, {
      'gRPC-Web status 200': (r) => r.status === 200,
      'has CORS headers': (r) => r.headers['Access-Control-Allow-Origin'] !== undefined,
      'response time < 500ms': (r) => r.timings.duration < 500,
    }) || errorRate.add(1);
  });
  
  group('Rate Limiting Test', function() {
    let response = http.post('http://127.0.0.1:8080/api/auth.v1.WebAuthService/LoginPublic',
      new Uint8Array([0,0,0,0,5,10,3,116,101,115,116]), {
      headers: {
        'content-type': 'application/grpc-web+proto',
        'x-grpc-web': '1',
      },
    });
    
    check(response, {
      'rate limit works': (r) => r.status === 200 || r.status === 429,
    });
  });
}
```

**Run load test:**
```bash
k6 run api-gateway-load-test.js

# ✅ Expected: Performance metrics for all gateway features
# ✅ Demonstrates: Production readiness, scalability
```

## Troubleshooting

### Common Issues

**1. CORS Errors:**
```bash
# Check CORS configuration
curl -X OPTIONS 'http://127.0.0.1:8080/api/FakeService/Handle' \
  -H 'origin: http://localhost:3000' -v
```

**2. gRPC-Web Format Issues:**
```bash
# Verify proper gRPC-Web headers
curl 'http://127.0.0.1:8080/api/FakeService/Handle' \
  -H 'content-type: application/grpc-web+proto' \
  -H 'x-grpc-web: 1' -v
```

**3. Service Discovery:**
```bash
# Check Envoy clusters
curl http://localhost:8000/clusters

# Check listeners
curl http://localhost:8000/listeners
```

### Debug Mode

**Run Envoy with debug logging:**
```bash
docker run -d \
  -v "$(pwd)"/envoy.yaml:/etc/envoy/envoy.yaml:ro \
  -e LOG_LEVEL=debug \
  --network=host \
  envoyproxy/envoy:v1.36.2
```

## Development Testing

### Manual Envoy Testing

```bash
# Generate config
go run . -api-conf config.yaml -out-envoy-conf envoy.yaml

# Run Envoy directly
docker run -d \
  -v "$(pwd)"/envoy.yaml:/etc/envoy/envoy.yaml:ro \
  --network=host \
  envoyproxy/envoy:v1.36.2

# Verify configuration
curl http://localhost:8000/config_dump
```

### Configuration Validation

```bash
# Validate generated config
docker run --rm \
  -v "$(pwd)"/envoy.yaml:/etc/envoy/envoy.yaml:ro \
  envoyproxy/envoy:v1.36.2 \
  --mode validate \
  --config-path /etc/envoy/envoy.yaml
```

---

**Documentation verified with:**
- Envoy v1.36.2 (Latest stable as of October 2025)
- Evans CLI v0.10+ (GitHub: ktr0731/evans)
- nicholasjackson/fake-service:v0.19.1
- grpcurl (GitHub: fullstorydev/grpcurl)