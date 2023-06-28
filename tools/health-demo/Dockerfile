FROM golang:1.20.5-bullseye@sha256:4c94dfe9567ee66a0ebc35a28f9f939e9ca3a71cb2ab942359887c584da1b28e as golang-base
FROM debian:bullseye-slim@sha256:924df86f8aad741a0134b2de7d8e70c5c6863f839caadef62609c1be1340daf5 as os-base

FROM golang-base as golang-grpc-server-builder

WORKDIR /build
ADD . /build

RUN go mod download

# Build for linux
ENV GOOS=linux
ENV GOARCH=amd64
ENV CGO_ENABLED=0
RUN go build -o server


FROM os-base as golang-grpc-server
WORKDIR /root/
# Copy the linux amd64 binary
COPY --from=golang-grpc-server-builder /build/server /bin/
ENTRYPOINT /bin/server