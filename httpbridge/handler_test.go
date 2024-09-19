package httpbridge_test

import (
	"context"
	_ "embed"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/geode-io/golambdas/httpbridge"
)

func Test_ServeHTTP(t *testing.T) {
	tests := []struct {
		name    string
		reqJSON string
		handler http.Handler
	}{
		{
			name:    "API Gateway - REST - 200 OK",
			reqJSON: apiGatewayHelloWorldRequest,
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
		},
		{
			name:    "API Gateway - REST - 200 OK with Body",
			reqJSON: apiGatewayHelloWorldRequest,
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"goodbye": "world"}`))
			}),
		},
		{
			name:    "ALB Target Group - 200 OK",
			reqJSON: albTargetGroupHelloWorldRequest,
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
		},
		{
			name:    "ALB Target Group - 200 OK with Body",
			reqJSON: albTargetGroupHelloWorldRequest,
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"goodbye": "world"}`))
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes := json.RawMessage{}
			err := json.Unmarshal([]byte(tt.reqJSON), &jsonBytes)
			require.NoError(t, err)
			_, err = httpbridge.ServeHTTP(tt.handler).Invoke(context.Background(), jsonBytes)
			assert.NoError(t, err)
		})
	}
}

var (
	//go:embed testpayloads/alb_target_group.json
	albTargetGroupHelloWorldRequest string
	//go:embed testpayloads/apigateway_rest.json
	apiGatewayHelloWorldRequest string
)
