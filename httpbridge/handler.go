package httpbridge

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/oapi-codegen/runtime/strictmiddleware/nethttp"
)

func ServeAPI[STRICTAPI any, API any](
	api STRICTAPI,
	configureHandler func(
		STRICTAPI,
		[]nethttp.StrictHTTPMiddlewareFunc,
	) API,
	serve func(API) http.Handler,
	opts ...APIOption,
) lambda.Handler {
	useOpts := apiOptions{}
	for _, opt := range opts {
		opt(&useOpts)
	}

	handler := serve(configureHandler(api, useOpts.strictMiddlewares))
	for _, middleware := range useOpts.lowLevelMiddlewares {
		handler = middleware(handler)
	}

	return ServeHTTP(handler, useOpts.lowLevelMiddlewares...)
}

func ServeHTTP(
	handler http.Handler,
	middleware ...func(http.Handler) http.Handler,
) lambda.Handler {
	for _, middleware := range middleware {
		handler = middleware(handler)
	}

	lambdaHandler := func(ctx context.Context, req json.RawMessage) (json.RawMessage, error) {
		slog.Info("received request payload", "request.payload.raw", req)
		lambdaHTTPResponseWriter := &lambdaHTTPResponseWriter{}
		disambiguatedRequest, err := demuxAmbiguousRequest(req, lambdaHTTPResponseWriter)
		if err != nil {
			slog.ErrorContext(ctx, "failed to demux ambiguous request", "error", err)
			return json.Marshal(leastCommonDenominatorResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       err.Error(),
			})
		}

		httpRequest, err := disambiguatedRequest.Canonize(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "failed to canonize request", "error", err)
			return json.Marshal(leastCommonDenominatorResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       err.Error(),
			})
		}
		handler.ServeHTTP(lambdaHTTPResponseWriter, httpRequest)
		resp := &ambiguousLambdaResponse{}
		err = resp.TranscodeFrom(lambdaHTTPResponseWriter)
		if err != nil {
			slog.ErrorContext(ctx, "failed to transcode response", "error", err)
			return json.Marshal(leastCommonDenominatorResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       err.Error(),
			})
		}
		slog.InfoContext(ctx, "wrote response in memory", "resp", resp.String(), "resp.writer", lambdaHTTPResponseWriter.String())
		return resp.bytes, nil
	}

	return lambda.NewHandlerWithOptions(lambdaHandler, lambda.WithEnableSIGTERM(func() {
		slog.Info("received SIGTERM, shutting down")
	}))
}

func ServeAPIGatewayV2(
	handler http.Handler,
	middleware ...func(http.Handler) http.Handler,
) lambda.Handler {
	return serve(
		handler,
		func(req events.APIGatewayV2HTTPRequest) *apiGatewayV2Request {
			return ptr(apiGatewayV2Request(req))
		},
		func(res *apiGatewayV2Response) *events.APIGatewayV2HTTPResponse {
			if res == nil {
				return nil
			}

			return ptr(events.APIGatewayV2HTTPResponse(*res))
		},
		func() *apiGatewayV2Response { return &apiGatewayV2Response{} },
		func(statusCode int, err error) *events.APIGatewayV2HTTPResponse {
			return &events.APIGatewayV2HTTPResponse{
				StatusCode: statusCode,
				Body:       err.Error(),
			}
		},
		middleware...,
	)
}

func ServeAPIGateway(
	handler http.Handler,
	middleware ...func(http.Handler) http.Handler,
) lambda.Handler {
	return serve(
		handler,
		func(req events.APIGatewayProxyRequest) *apiGatewayV1Request {
			return ptr(apiGatewayV1Request(req))
		},
		func(res *apiGatewayV1Response) *events.APIGatewayProxyResponse {
			if res == nil {
				return nil
			}

			return ptr(events.APIGatewayProxyResponse(*res))
		},
		func() *apiGatewayV1Response { return &apiGatewayV1Response{} },
		func(statusCode int, err error) *events.APIGatewayProxyResponse {
			return &events.APIGatewayProxyResponse{
				StatusCode: statusCode,
				Body:       err.Error(),
			}
		},
		middleware...,
	)
}

func ServeALB(
	handler http.Handler,
	middleware ...func(http.Handler) http.Handler,
) lambda.Handler {
	return serve(
		handler,
		func(req events.ALBTargetGroupRequest) *albRequest {
			return ptr(albRequest(req))
		},
		func(res *albResponse) *events.ALBTargetGroupResponse {
			if res == nil {
				return nil
			}

			return ptr(events.ALBTargetGroupResponse(*res))
		},
		func() *albResponse { return &albResponse{} },
		func(statusCode int, err error) *events.ALBTargetGroupResponse {
			return &events.ALBTargetGroupResponse{
				StatusCode: statusCode,
				Body:       err.Error(),
			}
		},
		middleware...,
	)
}

func serve[RAWREQ any, REQ lambdaHTTPRequest, RAWRESP any, RESP lambdaHTTPResponse](
	handler http.Handler,
	castReq func(RAWREQ) REQ,
	castResp func(RESP) RAWRESP,
	newResp func() RESP,
	newErrResp func(int, error) RAWRESP,
	middleware ...func(http.Handler) http.Handler,
) lambda.Handler {
	for _, middleware := range middleware {
		handler = middleware(handler)
	}

	lambdaHandler := func(ctx context.Context, rawReq RAWREQ) (RAWRESP, error) {
		slog.InfoContext(ctx, "received request payload", slog.Group("request", "payload", rawReq))
		lambdaHTTPResponseWriter := &lambdaHTTPResponseWriter{}
		req := castReq(rawReq)
		httpRequest, err := req.Canonize(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "failed to canonize request", "error", err)
			return newErrResp(http.StatusInternalServerError, err), nil
		}
		handler.ServeHTTP(lambdaHTTPResponseWriter, httpRequest)
		resp := newResp()
		err = resp.TranscodeFrom(lambdaHTTPResponseWriter)
		if err != nil {
			slog.ErrorContext(ctx, "failed to transcode response", "error", err)
			return newErrResp(http.StatusInternalServerError, err), nil
		}
		slog.InfoContext(ctx, "wrote response in memory", "resp", resp, "resp.writer", lambdaHTTPResponseWriter)
		return castResp(resp), nil
	}

	return lambda.NewHandlerWithOptions(lambdaHandler, lambda.WithEnableSIGTERM(func() {
		slog.Info("received SIGTERM, shutting down")
	}))
}

func ptr[TO any](to TO) *TO {
	return &to
}
