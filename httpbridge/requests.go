package httpbridge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-lambda-go/events"
)

type apiGatewayV2Request events.APIGatewayV2HTTPRequest
type apiGatewayV1Request events.APIGatewayProxyRequest
type albRequest events.ALBTargetGroupRequest

type ambiguousLambdaRequest struct {
	RequestContext struct {
		// if this is present it's an ALB request
		ELB struct {
			TargetGroupArn string `json:"targetGroupArn"`
		} `json:"elb"`
		// if this is present it's an API Gateway v1 request
		AccountID string `json:"accountId"`
	} `json:"requestContext"`
	// if this is present and the version is 2.0 it's an API Gateway v2 request
	Version string `json:"version"`
}

type lambdaHTTPRequest interface {
	Canonize(context.Context) (*http.Request, error)
}

var _ lambdaHTTPRequest = (*apiGatewayV2Request)(nil)
var _ lambdaHTTPRequest = (*apiGatewayV1Request)(nil)
var _ lambdaHTTPRequest = (*albRequest)(nil)

var (
	hostHeader = http.CanonicalHeaderKey("host")
)

func (r *apiGatewayV2Request) Canonize(ctx context.Context) (*http.Request, error) {
	rawQuery := r.RawQueryString
	if len(rawQuery) == 0 {
		params := url.Values{}
		for k, v := range r.QueryStringParameters {
			params.Add(k, v)
		}
		rawQuery = params.Encode()
	}

	headers := make(http.Header)
	for k, v := range r.Headers {
		headers.Add(k, v)
	}

	path, err := url.PathUnescape(r.RawPath)
	if err != nil {
		return nil, fmt.Errorf("failed to unescape path %s from request: %w", r.RawPath, err)
	}

	u := url.URL{
		Host:     headers.Get(hostHeader),
		Path:     path,
		RawQuery: rawQuery,
	}

	var body io.Reader
	if r.IsBase64Encoded {
		body = base64.NewDecoder(base64.StdEncoding, body)
	} else {
		body = strings.NewReader(r.Body)
	}

	out, err := http.NewRequestWithContext(ctx, r.RequestContext.HTTP.Method, u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("failed to canonize incoming http request: %w", err)
	}
	out.RemoteAddr = r.RequestContext.HTTP.SourceIP
	out.RequestURI = u.RequestURI()
	out.Header = headers
	return out, nil
}

func (r *apiGatewayV1Request) Canonize(ctx context.Context) (*http.Request, error) {
	params := url.Values{}
	for k, v := range r.QueryStringParameters {
		params.Add(k, v)
	}
	for k, v := range r.MultiValueQueryStringParameters {
		for _, vv := range v {
			params.Add(k, vv)
		}
	}
	rawQuery := params.Encode()

	headers := make(http.Header)
	for k, v := range r.Headers {
		headers.Add(k, v)
	}
	for k, v := range r.MultiValueHeaders {
		headers[k] = v
	}

	path, err := url.PathUnescape(r.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to unescape path %s from request: %w", r.Path, err)
	}

	u := url.URL{
		Host:     headers.Get(hostHeader),
		Path:     path,
		RawQuery: rawQuery,
	}

	var body io.Reader
	if r.IsBase64Encoded {
		body = base64.NewDecoder(base64.StdEncoding, body)
	} else {
		body = strings.NewReader(r.Body)
	}

	out, err := http.NewRequestWithContext(ctx, r.HTTPMethod, u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("failed to canonize incoming http request: %w", err)
	}
	out.RemoteAddr = r.RequestContext.Identity.SourceIP
	out.RequestURI = u.RequestURI()
	out.Header = headers
	return out, nil
}

func (r *albRequest) sourceIP() string {
	if xff, ok := r.MultiValueHeaders["x-forwarded-for"]; ok && len(xff) > 0 {
		ips := strings.SplitN(xff[0], ",", 2)
		if len(ips) > 0 {
			return ips[0]
		}
	}
	return ""
}

func (r *albRequest) Canonize(ctx context.Context) (*http.Request, error) {
	params := url.Values{}
	for k, v := range r.QueryStringParameters {
		params.Add(k, v)
	}
	rawQuery := params.Encode()

	headers := make(http.Header)
	for k, v := range r.Headers {
		headers.Add(k, v)
	}

	path, err := url.PathUnescape(r.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to unescape path %s from request: %w", r.Path, err)
	}

	u := url.URL{
		Host:     headers.Get(hostHeader),
		Path:     path,
		RawQuery: rawQuery,
	}

	var body io.Reader
	if r.IsBase64Encoded {
		body = base64.NewDecoder(base64.StdEncoding, body)
	} else {
		body = strings.NewReader(r.Body)
	}

	out, err := http.NewRequestWithContext(ctx, r.HTTPMethod, u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("failed to canonize incoming http request: %w", err)
	}
	out.RemoteAddr = r.sourceIP()
	out.RequestURI = u.RequestURI()
	out.Header = headers
	return out, nil
}

var (
	ErrUnsupportedRequestType = errors.New("unsupported request type")
)

func demuxAmbiguousRequest(payload json.RawMessage, rw *lambdaHTTPResponseWriter) (lambdaHTTPRequest, error) {
	var ambiguous ambiguousLambdaRequest
	if err := json.Unmarshal(payload, &ambiguous); err != nil {
		return nil, err
	}

	// Determine the type based on the parsed fields
	switch {
	case ambiguous.RequestContext.ELB.TargetGroupArn != "":
		var albReq albRequest
		err := json.Unmarshal(payload, &albReq)
		rw.preparedResponse = &albResponse{}
		return &albReq, err
	// V2 may also have an account ID
	case ambiguous.Version == "2.0":
		var apiV2Req apiGatewayV2Request
		err := json.Unmarshal(payload, &apiV2Req)
		rw.preparedResponse = &apiGatewayV2Response{}
		return &apiV2Req, err
	case ambiguous.RequestContext.AccountID != "":
		var apiV1Req apiGatewayV1Request
		err := json.Unmarshal(payload, &apiV1Req)
		rw.preparedResponse = &apiGatewayV1Response{}
		return &apiV1Req, err
	}

	return nil, ErrUnsupportedRequestType
}
