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