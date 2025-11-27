package main

import (
	"fmt"
	"time"

	"envoy.auth/extAuth"
	"github.com/golang/protobuf/ptypes/wrappers"
	grpcx "github.com/tel-io/instrumentation/middleware/grpc"
	"github.com/tel-io/tel/v2"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	code1 "google.golang.org/genproto/googleapis/rpc/code"
	status1 "google.golang.org/genproto/googleapis/rpc/status"

	envoy_api_v3_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_auth_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
)

type server struct {
	conn    *grpc.ClientConn
	client  extAuth.AuthSessionServiceClient
	authCfg *APIConf
	logger  *tel.Telemetry

	recaptchaProcessor *RecaptchaProcessor
	disabledRecaptcha  bool

	rateLimitManager *RateLimitManager
}

var _ envoy_service_auth_v3.AuthorizationServer = &server{}

func NewServer(logger *tel.Telemetry, extAuthAddr string, authCfg *APIConf, rcConf *RCConf) (*server, error) {
	conn, err := grpc.Dial(
		extAuthAddr,
		grpc.WithInsecure(),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                5 * time.Minute, // defaultKeepalivePolicyMinTime
			PermitWithoutStream: true,
		}),
		// for unary use tel module
		grpc.WithChainUnaryInterceptor(grpcx.UnaryClientInterceptorAll()),
	)
	if err != nil {
		return nil, err
	}

	var (
		recaptchaProcessor *RecaptchaProcessor
		disabledRecaptcha  bool
	)
	if rcConf.URL == "" {
		// disable recaptcha in this case
		logger.Warn("recaptcha is switched off")
		disabledRecaptcha = true
	} else {
		recaptchaProcessor = NewRecaptchaProcessor(rcConf, logger)
	}

	return &server{
		conn:    conn,
		client:  extAuth.NewAuthSessionServiceClient(conn),
		authCfg: authCfg,
		logger:  logger,

		recaptchaProcessor: recaptchaProcessor,
		disabledRecaptcha:  disabledRecaptcha,

		rateLimitManager: NewRateLimitManager(authCfg, logger),
	}, nil
}

func (s *server) Close() error {
	return s.conn.Close()
}

func formCheckResponse(code v3.StatusCode, message string, headers []*envoy_api_v3_core.HeaderValueOption) *envoy_service_auth_v3.CheckResponse {
	resp := &envoy_service_auth_v3.CheckResponse{}

	if code == 0 {
		// Allow request
		resp.Status = &status1.Status{Code: int32(code1.Code_OK), Message: message}
		resp.HttpResponse = &envoy_service_auth_v3.CheckResponse_OkResponse{
			OkResponse: &envoy_service_auth_v3.OkHttpResponse{Headers: headers},
		}
	} else {
		// Deny request - Status must be non-OK for Envoy to deny
		resp.Status = &status1.Status{Code: int32(code1.Code_PERMISSION_DENIED), Message: message}
		resp.HttpResponse = &envoy_service_auth_v3.CheckResponse_DeniedResponse{
			DeniedResponse: &envoy_service_auth_v3.DeniedHttpResponse{
				Status:  &v3.HttpStatus{Code: code},
				Headers: headers,
			},
		}
	}

	return resp
}

func (s *server) Check(ctx context.Context, in *envoy_service_auth_v3.CheckRequest) (*envoy_service_auth_v3.CheckResponse, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		s.logger.Warn("cant receive gRPC metadata")
	} else {
		mstr := "gRPC metadata:\n"
		for k, v := range md {
			mstr += fmt.Sprintf("\t%s: %s\n", k, v)
		}
		s.logger.Debug(mstr)
	}

	span, ccx := tel.StartSpanFromContext(ctx, "check-auth")
	defer span.End()

	respHeaders := []*envoy_api_v3_core.HeaderValueOption{}
	headers := in.Attributes.Request.Http.Headers

	// Get path from request (Envoy passes it in Http.Path, not in headers)
	path := in.Attributes.Request.Http.Path
	if path == "" {
		// Fallback to :path header for gRPC requests
		path = headers[":path"]
	}
	if path == "" {
		st := status.New(codes.InvalidArgument, ":path header is not found")
		return nil, st.Err()
	}

	service, method := parsePath(path)
	s.logger.Debug("parsed path",
		tel.String("path", path),
		tel.String("service", service),
		tel.String("method", method))
	if service == "" || method == "" {
		return formCheckResponse(v3.StatusCode_BadRequest, "bad path", respHeaders), nil
	}

	span.SetAttributes(
		attribute.String("xrealip", headers["x-real-ip"]),
		attribute.String("method", method),
	)

	// Get client IP from x-real-ip or x-forwarded-for
	clientIP := headers["x-real-ip"]
	if clientIP == "" {
		clientIP = headers["x-forwarded-for"]
	}
	if clientIP == "" {
		s.logger.Warn("client IP not found in headers (x-real-ip or x-forwarded-for)")
	}

	v2RepatchaPassed := false
	if !s.rateLimitManager.Check(clientIP, method) {
		v2RepatchaPassed = s.checkReCaptcha(headers, true /*v2*/)
		if v2RepatchaPassed {
			s.rateLimitManager.Reset(clientIP, method)
		} else {
			return formCheckResponse(v3.StatusCode_TooManyRequests, "rate limit is reached", respHeaders), nil
		}
	}

	reqPermission := s.authCfg.GetRequestedPermissions(service, method)
	s.logger.Debug("requested permissions",
		tel.String("method", path), tel.Any("permissions", reqPermission))

	if reqPermission == nil {
		return formCheckResponse(v3.StatusCode_BadRequest, "unknown auth for method", respHeaders), nil
	}

	if !v2RepatchaPassed && reqPermission.NeedReCaptcha() {
		if !s.checkReCaptcha(headers, false /*v2*/) {
			return formCheckResponse(v3.StatusCode_PreconditionFailed, "", respHeaders), nil
		}
	}
	if reqPermission.NoNeed() {
		return formCheckResponse(0, "", respHeaders), nil
	}

	token, err := parseTokenCookie(headers["cookie"])
	s.logger.Debug("token", tel.String("token", token), tel.Error(err))
	if err != nil {
		return formCheckResponse(v3.StatusCode_BadRequest, err.Error(), respHeaders), nil
	}

	if token == "" && reqPermission.Optional() {
		return formCheckResponse(0, "", respHeaders), nil
	}
	if token == "" && reqPermission.Required() {
		return formCheckResponse(v3.StatusCode_Unauthorized, "token required", respHeaders), nil
	}

	// validate session
	req := &extAuth.ValidateSessionRequest{
		SessionToken: token,
	}
	resp, err := s.client.ValidateSession(
		ccx,
		req,
		grpc.WaitForReady(true),
	)

	s.logger.Debug("AuthService", tel.Any("response", resp), tel.Error(err))

	if err != nil {
		respHeaders = append(respHeaders, &envoy_api_v3_core.HeaderValueOption{
			Header: &envoy_api_v3_core.HeaderValue{Key: "set-cookie", Value: "token=; Path=/; Max-Age=0; HttpOnly"},
			Append: &wrappers.BoolValue{Value: false},
		})

		if reqPermission.Optional() {
			return formCheckResponse(0, "", respHeaders), nil
		}

		return formCheckResponse(v3.StatusCode_Unauthorized, err.Error(), respHeaders), nil
	}

	span.SetAttributes(
		attribute.String("userid", resp.UserId),
		attribute.String("sessionid", resp.SessionId),
	)

	if reqPermission.Required() && !authorize(reqPermission.Permission, resp.Roles) {
		return formCheckResponse(v3.StatusCode_Forbidden, "access denied", respHeaders), nil
	}

	respHeaders = append(respHeaders, &envoy_api_v3_core.HeaderValueOption{
		Header: &envoy_api_v3_core.HeaderValue{Key: "user-id", Value: resp.UserId},
		Append: &wrappers.BoolValue{Value: false},
	})

	respHeaders = append(respHeaders, &envoy_api_v3_core.HeaderValueOption{
		Header: &envoy_api_v3_core.HeaderValue{Key: "session-id", Value: resp.SessionId},
		Append: &wrappers.BoolValue{Value: false},
	})

	return formCheckResponse(0, "", respHeaders), nil
}

func (s *server) checkReCaptcha(headers map[string]string, v2 bool) bool {
	if s.disabledRecaptcha {
		return true
	}

	hName := "x-rc-token"
	if v2 {
		hName = "x-rc-token-2"
	}

	token, ok := headers[hName]
	if !ok {
		s.logger.Debug("header is not passed", tel.String("name", hName))
		return false
	}

	return s.recaptchaProcessor.CheckRecaptcha(token, v2)
}
