package httpbridge

import (
	"net/http"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/oapi-codegen/runtime/strictmiddleware/nethttp"
)

type apiOptions struct {
	lambdaMiddlewares   []func(lambda.Handler) lambda.Handler
	strictMiddlewares   []nethttp.StrictHTTPMiddlewareFunc
	lowLevelMiddlewares []func(http.Handler) http.Handler
}

type APIOption func(*apiOptions)

func APIMiddleware(middlewares ...nethttp.StrictHTTPMiddlewareFunc) APIOption {
	return func(o *apiOptions) {
		o.strictMiddlewares = append(o.strictMiddlewares, middlewares...)
	}
}

func HTTPMiddleware(middlewares ...func(http.Handler) http.Handler) APIOption {
	return func(o *apiOptions) {
		o.lowLevelMiddlewares = append(o.lowLevelMiddlewares, middlewares...)
	}
}

func LambdaMiddleware(middlewares ...func(lambda.Handler) lambda.Handler) APIOption {
	return func(o *apiOptions) {
		o.lambdaMiddlewares = append(o.lambdaMiddlewares, middlewares...)
	}
}
