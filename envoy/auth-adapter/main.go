package main

import (
	"net"
	"os"
	"os/signal"
	"syscall"

	grpcx "github.com/tel-io/instrumentation/middleware/grpc"
	"github.com/tel-io/tel/v2"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"

	envoy_service_auth_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
)

func parseRCConf() *RCConf {
	rcConf := &RCConf{}

	rcConf.URL = os.Getenv("RECAPTCHA_URL")
	rcConf.SecretV2 = os.Getenv("RECAPTCHA_SECRET_V2")
	rcConf.SecretV3 = os.Getenv("RECAPTCHA_SECRET_V3")
	rcConf.MinScore = 0.5 //FIXME should be configurable?

	return rcConf
}

func main() {
	logg, closer := tel.New(context.Background(), tel.GetConfigFromEnv())
	defer closer()

	logg.Info("starting auth-adapter")

	sigs := make(chan os.Signal, 1)

	signal.Notify(sigs, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// load auth config
	authCfg, err := LoadConfig("/opt/auth-adapter/config.yaml")
	if err != nil {
		panic(err)
	}

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(grpcx.UnaryServerInterceptor()),
	)

	go func() {
		listener, err := net.Listen("tcp", ":9000")
		if err != nil {
			grpclog.Fatalf("failed to listen: %v", err)
		}

		s, err := NewServer(&logg, os.Getenv("AUTH_SERVICE_ADDR"), authCfg, parseRCConf())
		if err != nil {
			panic(err)
		}

		envoy_service_auth_v3.RegisterAuthorizationServer(grpcServer, s)

		logg.Info("gRPC service started at :9000")
		err = grpcServer.Serve(listener)
		if err != nil {
			panic(err)
		}
	}()

	<-sigs
	logg.Info("stopping...")
	grpcServer.Stop()

	logg.Info("done.")
}
