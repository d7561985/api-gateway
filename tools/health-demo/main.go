package main

import (
	"demo/pkg/health"
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/ktr0731/grpc-web-go-client/grpcweb"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	isServer := flag.Bool("is_server", true, "launch health server")
	port := flag.Int("port", 8081, "grpc port")
	clientAddr := flag.String("client", "127.0.0.1:8080", "client web-grpc address")
	clientPath := flag.String("web-grpc-path", "grpc.health.v1.Health",
		"web-grpc method <package>.<service>")
	clientFn := flag.String("web-grpc-fn", "Check", "actual method in health: Check, ToDo: stream Watch")

	flag.Parse()

	if *isServer {
		startServer(port)
	} else {
		clientWebGrpc(clientAddr, clientPath, clientFn)
	}
}

func clientWebGrpc(clientAddr *string, clientPath *string, clientFn *string) {
	res := &grpc_health_v1.HealthCheckResponse{}

	cc, ee := grpcweb.DialContext(*clientAddr)
	if ee != nil {
		panic(ee)
	}

	methdod := fmt.Sprintf("%s/%s", *clientPath, *clientFn)

	ee = cc.Invoke(context.Background(), methdod, &grpc_health_v1.HealthCheckRequest{}, res)
	if ee != nil {
		panic(ee)
	}

	fmt.Println(res.Status.String())
}

func startServer(port *int) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	gs := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(gs, new(health.Check))
	log.Printf("starting grpc on :%d\n", *port)

	gs.Serve(lis)
}
