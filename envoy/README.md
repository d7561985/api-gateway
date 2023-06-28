# Build chain

```bash
docker-compose up
```

## Test grpc service

U need evans cli

### Web-GRPC:  Health Check Service

```bash
evans --proto ./tools/protos/health_check.proto --port 8080 --web repl --package grpc.health.v1 --service Health
```

then check unary (RPC)

```bash
grpc.health.v1.Health@127.0.0.1:8080> call Check 
service (TYPE_STRING) => ww
{
  "status": "SERVING"
}
```

check streaming (ws flow)
```bash
grpc.health.v1.Health@127.0.0.1:8080> call Watch
service (TYPE_STRING) => w
{
  "status": "SERVING"
}
{
  "status": "SERVING"
}
{
  "status": "SERVING"
}
```

### Web-GRPC: FakeService

```bash
evans --proto ./tools/protos/api.proto --port 8080 --web repl  --service FakeService
```

and then

```bash
FakeService@127.0.0.1:8080> call Handle
data (TYPE_BYTES) => qwe
{
  "Message": "{\n  \"name\": \"web\",\n  \"type\": \"gRPC\",\n  \"ip_addresses\": [\n    \"192.168.192.2\"\n  ],\n  \"start_time\": \"2023-06-28T14:31:18.515878\",\n  \"end_time\": \"2023-06-28T14:31:18.527275\",\n  \"duration\": \"11.395958ms\",\n  \"body\": \"Hello World\",\n  \"code\": 0\n}\n"
}

```

### RPC FakeServer

Just in case, we may need check RCP availability so using `grpcurl` we can check via reflection FakeService

in cli call

```bash
grpcurl -plaintext 127.0.0.1:9091  FakeService.Handle
```

### 
Web-grpc via api-gateway check

```bash
curl 'http://127.0.0.1:8080/api/FakeService/Handle' \
-H 'content-type: application/grpc-web+proto' \
-H 'x-grpc-web: 1' \
-H 'x-real-ip: 127.0.0.1' \
-H 'authority: 127.0.0.1:8080' \
--insecure
```


## DEV

```bash
docker run -d -v "$(pwd)"/envoy.yaml:/etc/envoy/envoy.yaml:ro \
    --network=host envoyproxy/envoy:v1.22.0
```