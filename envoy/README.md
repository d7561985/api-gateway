# Build chain

> DOCKER_BUILDKIT=0 may need

```bash
docker-compose up
```

Test grpc service:
```bash
grpcurl -plaintext 127.0.0.1:9091  FakeService.Handle
```

Web-grpc via api-gateway check
```bash
curl 'http://127.0.0.1:8080/api/FakeService/Handle' \
-H 'content-type: application/grpc-web+proto' \
-H 'x-grpc-web: 1' \
-H 'x-real-ip: 127.0.0.1' \
-H 'authority: 127.0.0.1:8080' \
--insecure
```