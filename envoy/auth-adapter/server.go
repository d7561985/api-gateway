package main

import (
	"fmt"
	"time"

	"envoy.auth/extAuth"
	wrappers "github.com/gogo/protobuf/types"
	grpcx "github.com/tel-io/instrumentation/middleware/grpc"
	"github.com/tel-io/tel/v2"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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

func formCheckResponse(code StatusCode, message string, headers []*HeaderValueOption) *CheckResponse {
	resp := &CheckResponse{
		Status: &RPCStatus{Code: int32(code), Message: message},
	}
	if code == 0 {
		resp.HttpResponse = &CheckResponse_OkResponse{
			OkResponse: &OkHttpResponse{Headers: headers},
		}
	} else {
		resp.HttpResponse = &CheckResponse_DeniedResponse{
			DeniedResponse: &DeniedHttpResponse{
				Status:  &HttpStatus{Code: code},
				Headers: headers,
			},
		}
	}

	return resp
}

func (s *server) Check(ctx context.Context, in *CheckRequest) (*CheckResponse, error) {
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

	respHeaders := []*HeaderValueOption{}
	headers := in.Attributes.Request.Http.Headers
	path, ok := headers[":path"]
	if !ok {
		st := status.New(codes.InvalidArgument, ":path header is not found")
		return nil, st.Err()
	}

	service, method := parsePath(path)
	if service == "" || method == "" {
		return formCheckResponse(StatusCode_BadRequest, "bad path", respHeaders), nil
	}

	span.SetAttributes(
		attribute.String("xrealip", headers["x-real-ip"]),
		attribute.String("method", method),
	)

	if headers["x-real-ip"] == "" {
		s.logger.Error("x-real-ip must be in request headers")
	}

	v2RepatchaPassed := false
	if !s.rateLimitManager.Check(headers["x-real-ip"], method) {
		v2RepatchaPassed = s.checkReCaptcha(headers, true /*v2*/)
		if v2RepatchaPassed {
			s.rateLimitManager.Reset(headers["x-real-ip"], method)
		} else {
			return formCheckResponse(StatusCode_TooManyRequests, "rate limit is reached", respHeaders), nil
		}
	}

	reqPermission := s.authCfg.GetRequestedPermissions(service, method)
	s.logger.Debug("requested permissions",
		tel.String("method", path), tel.Any("permissions", reqPermission))

	if reqPermission == nil {
		return formCheckResponse(StatusCode_BadRequest, "unknown auth for method", respHeaders), nil
	}

	if !v2RepatchaPassed && reqPermission.NeedReCaptcha() {
		if !s.checkReCaptcha(headers, false /*v2*/) {
			return formCheckResponse(StatusCode_PreconditionFailed, "", respHeaders), nil
		}
	}
	if reqPermission.NoNeed() {
		return formCheckResponse(0, "", respHeaders), nil
	}

	token, err := parseTokenCookie(headers["cookie"])
	s.logger.Debug("token", tel.String("token", token), tel.Error(err))
	if err != nil {
		return formCheckResponse(StatusCode_BadRequest, err.Error(), respHeaders), nil
	}

	if token == "" && reqPermission.Optional() {
		return formCheckResponse(0, "", respHeaders), nil
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
		respHeaders = append(respHeaders, &HeaderValueOption{
			Header: &HeaderValue{Key: "set-cookie", Value: "token=; Path=/; Max-Age=0; HttpOnly"},
			Append: &wrappers.BoolValue{Value: false},
		})

		if reqPermission.Optional() {
			return formCheckResponse(0, "", respHeaders), nil
		}

		return formCheckResponse(StatusCode_Unauthorized, err.Error(), respHeaders), nil
	}

	span.SetAttributes(
		attribute.String("userid", resp.UserId),
		attribute.String("sessionid", resp.SessionId),
	)

	if reqPermission.Required() && !authorize(reqPermission.Permission, resp.Roles) {
		return formCheckResponse(StatusCode_Forbidden, "access denied", respHeaders), nil
	}

	respHeaders = append(respHeaders, &HeaderValueOption{
		Header: &HeaderValue{Key: "user-id", Value: resp.UserId},
		Append: &wrappers.BoolValue{Value: false},
	})

	respHeaders = append(respHeaders, &HeaderValueOption{
		Header: &HeaderValue{Key: "session-id", Value: resp.SessionId},
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
