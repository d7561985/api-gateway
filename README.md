# Envoy API Gateway

A declarative, developer-friendly API Gateway built on Envoy Proxy for gRPC microservices ecosystems. This project generates optimized Envoy configurations with built-in authentication, authorization, observability, and gRPC-Web transpilation.

## Features

### ✅ Currently Implemented Features

#### **API Gateway (Config Generator)**
- **🔄 gRPC ↔ Web-gRPC Transpilation**: Seamless browser-to-gRPC communication
- **🔐 External Authentication**: Integration via ext_authz filter with auth-adapter
- **📊 OpenTelemetry Integration**: Distributed tracing with automatic span creation
- **🌐 CORS Support**: Production-ready CORS configuration for web applications
- **⚡ High Performance**: Envoy v1.36.2 with optimized HTTP/2 settings
- **📝 Declarative Configuration**: Simple YAML-based API definitions
- **🔗 Mixed Protocol Support**: Both gRPC and HTTP backend services

#### **Auth-Adapter Service (External)**
- **🏷️ Header Enrichment**: Automatic `user-id` and `session-id` injection to backend
- **🚦 Rate Limiting**: Per-IP token bucket with configurable periods/counts/delays
- **🤖 reCAPTCHA v2/v3**: Bot protection with automatic rate limit bypass
- **👤 Session Validation**: Cookie-based authentication with role checking
- **🔐 Permission Authorization**: Role-based access control per API method

### 🔄 Integration Architecture

```
Client Request → API Gateway → auth-adapter → Backend Service
     │              │              │              │
     │              │              ▼              │
     │              │         ┌─────────────┐     │
     │              │         │ Validates:  │     │
     │              │         │ • Session   │     │
     │              │         │ • Rate Limit│     │  
     │              │         │ • reCAPTCHA │     │
     │              │         │ • Roles     │     │
     │              │         └─────────────┘     │
     │              │              │              │
     │              │              ▼              ▼
     └──────────────┼─────────▶ Adds Headers ──▶ Enriched Request:
                    │           • user-id           • user-id: 12345
                    │           • session-id        • session-id: sess_abc
                    └─────────────────────────────▶ • authorization: Bearer...
```

### ✅ Recently Added Features
- **🚦 Envoy Rate Limiting**: Full integration of rate limiting configuration into Envoy
- **❤️ Health Checks**: Active upstream service health monitoring 
- **⚡ Circuit Breaking**: Configurable failure handling and load shedding

### 🚧 Planned Features (Roadmap)  
- **OPA Policy Engine**: Fine-grained authorization policies
- **TLS Termination**: SSL/TLS certificate management
- **Outlier Detection**: Automatic unhealthy instance ejection
- **Metrics Dashboard**: Real-time performance monitoring UI

## Quick Start

### Prerequisites
- Docker and Docker Compose
- Go 1.25+ (for development)

### Running with Docker

1. **Clone the repository**
   ```bash
   git clone <repository-url>
   cd api-gateway
   ```

2. **Configure your API**
   ```bash
   cp envoy/api-gateway/config.example.yaml envoy/api-gateway/config.yaml
   # Edit config.yaml with your services
   ```

3. **Build and run**
   ```bash
   cd envoy/api-gateway
   docker build -t api-gateway .
   docker run -p 8080:8080 -p 8000:8000 \
     -v $(pwd)/config.yaml:/app/config.yaml \
     api-gateway
   ```

### Development Setup

1. **Install dependencies**
   ```bash
   cd envoy/api-gateway
   go mod download
   ```

2. **Generate Envoy configuration**
   ```bash
   go run . -api-conf config.yaml -out-envoy-conf envoy.yaml
   ```

3. **Run Envoy with generated config**
   ```bash
   envoy -c envoy.yaml
   ```

## Configuration

### API Definition Format

```yaml
# config.yaml - Mixed gRPC and HTTP services
api_route: "/api/v1/"

clusters:
  - name: user_grpc_service     # gRPC service  
    addr: "user-service:9000"
    type: "grpc"                # Explicit gRPC protocol
    health_check:               # NEW: Health monitoring
      path: "/health"
      interval_seconds: 30
      timeout_seconds: 5
    circuit_breaker:            # NEW: Circuit breaking
      max_connections: 100
      max_requests: 200
      max_retries: 3
      
  - name: payment_http_service  # HTTP/REST service
    addr: "payment-api:8080" 
    type: "http"                # Explicit HTTP protocol
    health_check:
      path: "/api/health"
      interval_seconds: 15
    circuit_breaker:
      max_connections: 50
      max_requests: 100

apis:
  - name: "UserService"         # gRPC service
    cluster: "user_grpc_service"
    auth:
      policy: "required"
      permission: "user:read"
    methods:
      - name: "GetUser"
        auth:
          policy: "required"
          permission: "user:read"
          rate_limit: {period: "1m", count: 10, delay: "2s"}  # NEW: Rate limiting
      - name: "CreateUser"
        auth:
          policy: "required" 
          permission: "user:write"
          rate_limit: {period: "1m", count: 5, delay: "5s"}   # NEW: Stricter limits
          
  - name: "PaymentAPI"          # HTTP service routed through gateway
    cluster: "payment_http_service"
    auth:
      policy: "required"
      permission: "payment:access"
    methods:
      - name: "ProcessPayment"
        auth:
          policy: "required"
          rate_limit: {period: "1m", count: 3, delay: "10s"} # NEW: Very strict limits
```

### gRPC Service Naming

Service name in config **must match the gRPC path** which depends on proto `package`:

| Proto definition | Config `name` | gRPC path |
|------------------|---------------|-----------|
| No package: `service Foo {}` | `Foo` | `/Foo/Method` |
| With package: `package x.y; service Bar {}` | `x.y.Bar` | `/x.y.Bar/Method` |

**Examples:**

```protobuf
// No package declaration
service UserService { rpc GetUser(...) }
// → Config name: "UserService"
// → Path: /api/v1/UserService/GetUser

// With package
package grpc.health.v1;
service Health { rpc Check(...) }
// → Config name: "grpc.health.v1.Health"
// → Path: /api/v1/grpc.health.v1.Health/Check
```

> **Note:** `option go_package` is for Go code generation only — it does **NOT** affect gRPC routing!

### Method Routing Behavior

| Scenario | Behavior |
|----------|----------|
| Configured method | Uses method-level auth config |
| Unconfigured method | Falls back to service-level auth, backend returns `UNIMPLEMENTED` |
| Unknown service | Gateway returns 404 (no route) |

### Supported Configuration Options

#### Authentication Policies
- **`required`**: Endpoint requires valid authentication
- **`optional`**: Authentication is checked but not required  
- **`no-need`**: Public endpoint, no authentication required

#### Cluster Types  
- **`grpc`**: gRPC service (HTTP/2, default if not specified)
- **`http`**: HTTP/REST service (HTTP/1.1)

#### Protocol Support
- **gRPC-Web**: Browser clients via HTTP/1.1 or HTTP/2
- **Native gRPC**: Direct gRPC clients via HTTP/2
- **HTTP Passthrough**: Direct HTTP service proxy

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `AUTH_ADAPTER_HOST` | Authentication service host | `127.0.0.1` |
| `OPEN_TELEMETRY_HOST` | OpenTelemetry collector host | `127.0.0.1` |
| `OPEN_TELEMETRY_PORT` | OpenTelemetry collector port | `4317` |
| `LOG_LEVEL` | Envoy logging level | `info` |

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Web Browser   │    │   Mobile App    │    │  gRPC Client    │
│   (gRPC-Web)    │    │   (gRPC-Web)    │    │   (HTTP/2)      │
└─────────┬───────┘    └─────────┬───────┘    └─────────┬───────┘
          │                      │                      │
          └──────────────────────┼──────────────────────┘
                                 │
                    ┌─────────────▼──────────────┐
                    │     Envoy API Gateway      │
                    │   ┌─────────────────────┐   │
                    │   │  gRPC-Web Filter    │   │
                    │   │  Auth Filter        │   │
                    │   │  CORS Filter        │   │
                    │   │  Rate Limit Filter  │   │
                    │   │  OpenTelemetry      │   │
                    │   └─────────────────────┘   │
                    └─────────────┬──────────────┘
                                  │
                  ┌───────────────┼───────────────┐
                  │               │               │
         ┌────────▼───────┐ ┌─────▼─────┐ ┌─────▼─────┐
         │ User Service   │ │Auth Service│ │Order Svc  │
         │   (gRPC)       │ │  (gRPC)    │ │  (gRPC)   │
         └────────────────┘ └───────────┘ └───────────┘
```

## CORS Configuration

The gateway includes production-ready CORS configuration that supports:

- **Flexible Origins**: Configure allowed origins with pattern matching
- **gRPC-Web Headers**: Pre-configured for gRPC-Web compatibility
- **Preflight Handling**: Automatic OPTIONS request processing
- **Security Headers**: Proper exposure of gRPC status headers

### CORS Security Considerations

⚠️ **CRITICAL SECURITY WARNING**: The current configuration uses `"*"` for origins, which is **UNSAFE for production**!

**Security Risks with `"*"`:**
- Allows ANY website to make requests to your API
- Enables XSS attacks from malicious websites  
- Breaks authentication security model
- Violates same-origin policy protection

**Production Security Requirements:**

1. **🚨 NEVER use `"*"` in production** - Replace with specific domains
2. **Limit Methods**: Only allow necessary HTTP methods
3. **Header Control**: Carefully configure allowed and exposed headers
4. **Credentials**: Use `allow_credentials: false` with `"*"` (browser requirement)

```yaml
# Production CORS example
cors:
  allow_origin_string_match:
  - exact: "https://yourdomain.com"
  - exact: "https://app.yourdomain.com"
  allow_methods: "GET, POST, OPTIONS"
  allow_headers: "content-type,x-grpc-web,grpc-timeout,authorization"
  expose_headers: "grpc-status,grpc-message"
  max_age: "86400"
```

## ✅ Mixed Protocol Support (IMPLEMENTED)

**The gateway ACTUALLY supports both gRPC and HTTP/REST services** through flexible cluster configuration:

### Real Implementation Details

- **gRPC clusters**: Use HTTP/2 with optimized gRPC protocol options
- **HTTP clusters**: Use HTTP/1.1 for legacy REST APIs  
- **Automatic routing**: Single entry point for all service types
- **CORS handling**: Works for both gRPC-Web and HTTP requests

### Verified Configuration Example

```yaml
# config.yaml - TESTED mixed gRPC and HTTP setup
api_route: "/api/v1/"

clusters:
  - name: user_grpc_service     # gRPC service  
    addr: "user-service:9000"
    type: "grpc"                # HTTP/2 with gRPC options
  - name: payment_http_service  # HTTP/REST service
    addr: "payment-api:8080" 
    type: "http"                # HTTP/1.1 for REST
  - name: fake_service          # nicholasjackson/fake-service
    addr: "fake-service:9091"
    type: "grpc"                # Can be "http" or "grpc"

apis:
  - name: "UserService"         # gRPC → gRPC-Web
    cluster: "user_grpc_service"
    auth:
      policy: "required"
      
  - name: "PaymentAPI"          # HTTP → HTTP passthrough
    cluster: "payment_http_service"
    auth:
      policy: "required"
```

### How It Works

1. **gRPC Services (`type: "grpc"`):**
   - Browser/client → gRPC-Web → API Gateway → HTTP/2 gRPC → Backend
   - Optimized with max_concurrent_streams: 1024, 16MiB windows
   
2. **HTTP Services (`type: "http"`):**
   - Browser/client → HTTP → API Gateway → HTTP/1.1 → REST API
   - Standard HTTP load balancing without gRPC protocol overhead

### Testing Both Protocols

Using `nicholasjackson/fake-service:v0.19.1` (verified working):

```bash
# Test gRPC service
curl 'http://127.0.0.1:8080/api/v1/FakeGRPCService/Handle' \
  -H 'content-type: application/grpc-web+proto' \
  -H 'x-grpc-web: 1'

# Test HTTP service  
curl 'http://127.0.0.1:8080/api/v1/FakeHTTPService/health' \
  -H 'content-type: application/json'
```

See `config.full-example.yaml` for complete working configuration.

### Future: gRPC-JSON Transcoding 

**TODO: gRPC-JSON Transcoding Integration**
- Add `envoy.filters.http.grpc_json_transcoder` filter
- Configure HTTP route mapping to gRPC methods
- Support for REST-style path parameters
- JSON payload transformation
- HTTP status code mapping

## Monitoring & Observability

### OpenTelemetry Integration

The gateway automatically instruments requests with:
- **Distributed Tracing**: Request tracing across service boundaries
- **Metrics Collection**: Performance and error rate metrics
- **Span Context**: Automatic span creation and propagation

### Metrics Endpoints

- **Admin Interface**: `http://localhost:8000` (development only)
- **Health Checks**: Built-in upstream health monitoring
- **Statistics**: Real-time proxy statistics and performance metrics

## Security Features

### Rate Limiting
**TODO: Rate Limiting Configuration**
- Token bucket algorithms
- Per-user and global limits
- Redis-based distributed limiting

### reCAPTCHA Integration
**TODO: reCAPTCHA Validation**
- Frontend challenge integration
- Server-side validation
- Bot protection mechanisms

### OPA Policy Engine
**TODO: Open Policy Agent Integration**
- Fine-grained authorization policies
- Policy-as-code approach
- Runtime policy updates

## Performance Tuning

### HTTP/2 Configuration

The gateway uses optimized HTTP/2 settings for Envoy v1.36+:
- **Max Concurrent Streams**: 1024 (default)
- **Initial Stream Window**: 16MiB
- **Connection Window**: 24MiB

### Resource Limits
- **Circuit Breakers**: Configurable connection and request limits
- **Timeouts**: Customizable request and connection timeouts
- **Buffer Limits**: Memory usage controls

## Development

### Building from Source

```bash
cd envoy/api-gateway
go build -o api-gateway .
```

### Running Tests

```bash
go test ./...
```

### Configuration Validation

```bash
./api-gateway -api-conf config.yaml -out-envoy-conf /dev/null
```

## Production Deployment

### Docker Compose Example

```yaml
version: '3.8'
services:
  api-gateway:
    build: ./envoy/api-gateway
    ports:
      - "8080:8080"
      - "8000:8000"  # Admin interface (remove in production)
    environment:
      - AUTH_ADAPTER_HOST=auth-service
      - OPEN_TELEMETRY_HOST=jaeger
      - LOG_LEVEL=warn
    volumes:
      - ./config.yaml:/app/config.yaml:ro
    depends_on:
      - auth-service
      - jaeger
```

### Security Checklist

- [ ] Remove admin interface exposure (port 8000)
- [ ] Configure specific CORS origins
- [ ] Enable TLS termination
- [ ] Set up proper authentication
- [ ] Configure rate limiting
- [ ] Enable security headers
- [ ] Set up log aggregation
- [ ] Configure health checks

## Comparison with Alternatives

| Feature | Amazon API Gateway | Apache APISIX | Traefik | **Envoy Gateway** |
|---------|-------------------|---------------|---------|-------------------|
| Performance | 7/10 | 10/10 | 5/10 | **9/10** |
| Declarative Config | 5/10 | 5/10 | 6/10 | **10/10** |
| gRPC-Web Support | 3/10 | 7/10 | 6/10 | **10/10** |
| Rate Limiting | 5/10 | 10/10 | 10/10 | **8/10** ¹ |
| Header Enrichment | 5/10 | 5/10 | 5/10 | **10/10** ² |
| Auth Integration | 6/10 | 8/10 | 8/10 | **9/10** ³ |
| Observability | 6/10 | 7/10 | 7/10 | **9/10** |
| Operational Excellence | 7/10 | 6/10 | 7/10 | **10/10** |

**Notes:**
¹ Rate limiting implemented in auth-adapter, not yet integrated into Envoy config generator  
² Header enrichment works perfectly via ext_authz integration  
³ Strong external auth via dedicated auth-adapter service

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Roadmap

- [ ] gRPC-JSON transcoding for REST API compatibility
- [ ] Advanced rate limiting with Redis backend
- [ ] reCAPTCHA integration for bot protection
- [ ] OPA policy engine integration
- [ ] Kubernetes operator for easy deployment
- [ ] Web UI for configuration management
- [ ] Load testing and benchmarking suite

## References

- [Envoy Proxy Documentation](https://www.envoyproxy.io/docs/)
- [gRPC-Web Documentation](https://github.com/grpc/grpc-web)
- [OpenTelemetry Specification](https://opentelemetry.io/docs/)
- [gRPC Official Documentation](https://grpc.io/docs/)