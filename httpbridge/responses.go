package httpbridge

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/aws/aws-lambda-go/events"
)

var (
	setCookieHeader        = http.CanonicalHeaderKey("set-cookie")
	contentTypeHeader      = http.CanonicalHeaderKey("content-type")
	transferEncodingHeader = http.CanonicalHeaderKey("transfer-encoding")
)

const (
	mimeTypeApplicationOctetStream = "application/octet-stream"
)

type lambdaHTTPResponseWriter struct {
	header     http.Header
	body       bytes.Buffer
	statusCode int

	preparedResponse lambdaHTTPResponse
}

var _ http.ResponseWriter = (*lambdaHTTPResponseWriter)(nil)
var _ http.Flusher = (*lambdaHTTPResponseWriter)(nil)

func (l *lambdaHTTPResponseWriter) Header() http.Header {
	if l.header == nil {
		l.header = make(http.Header)
	}
	return l.header
}

func (l *lambdaHTTPResponseWriter) writeHeaderLine(data []byte) {
	if l.statusCode != 0 {
		return
	}
	l.statusCode = 200
	header := l.Header()
	_, hasType := header[contentTypeHeader]
	hasTE := header.Get(transferEncodingHeader) != ""
	if !hasType && !hasTE {
		if data != nil {
			header.Set(contentTypeHeader, http.DetectContentType(data))
		}
	}
	l.header = header
}

func (l *lambdaHTTPResponseWriter) Write(data []byte) (int, error) {
	l.writeHeaderLine(data)
	written, err := l.body.Write(data)
	if err != nil {
		return written, err
	}

	return written, nil
}

func (l *lambdaHTTPResponseWriter) WriteHeader(statusCode int) {
	if l.statusCode != 0 {
		return
	}
	l.statusCode = statusCode
}

func (l *lambdaHTTPResponseWriter) Flush() {
	if l.statusCode != 0 {
		return
	}
	l.WriteHeader(http.StatusOK)
}

func (r *apiGatewayV2Response) TranscodeFrom(httpResponse *lambdaHTTPResponseWriter) error {
	r.StatusCode = httpResponse.statusCode
	r.Headers = make(map[string]string)
	for k, v := range httpResponse.header {
		if k == setCookieHeader {
			r.Cookies = append(r.Cookies, v...)
			continue
		}

		r.Headers[k] = strings.Join(v, ",")
	}
	// TODO: base64-encode other binary content-types as needed
	contentType := httpResponse.header.Get(contentTypeHeader)
	body := httpResponse.body.Bytes()
	if contentType == mimeTypeApplicationOctetStream {
		r.Body = base64.StdEncoding.EncodeToString(body)
		r.IsBase64Encoded = true
	} else {
		r.Body = string(body)
	}
	return nil
}

func (r *apiGatewayV1Response) TranscodeFrom(httpResponse *lambdaHTTPResponseWriter) error {
	r.StatusCode = httpResponse.statusCode
	r.Headers = make(map[string]string)
	for k, v := range httpResponse.header {
		if len(v) > 1 {
			r.MultiValueHeaders[k] = v
		} else if len(v) == 1 {
			r.Headers[k] = v[0]
		}
	}
	// TODO: base64-encode other binary content-types as needed
	contentType := httpResponse.header.Get(contentTypeHeader)
	body := httpResponse.body.Bytes()
	if contentType == mimeTypeApplicationOctetStream {
		r.Body = base64.StdEncoding.EncodeToString(body)
		r.IsBase64Encoded = true
	} else {
		r.Body = string(body)
	}
	return nil
}

func (r *albResponse) TranscodeFrom(httpResponse *lambdaHTTPResponseWriter) error {
	r.StatusCode = httpResponse.statusCode
	r.StatusDescription = http.StatusText(httpResponse.statusCode)
	r.Headers = make(map[string]string)
	for k, v := range httpResponse.header {
		if len(v) > 1 {
			r.MultiValueHeaders[k] = v
		} else if len(v) == 1 {
			r.Headers[k] = v[0]
		}
	}
	// TODO: base64-encode other binary content-types as needed
	contentType := httpResponse.header.Get(contentTypeHeader)
	body := httpResponse.body.Bytes()
	if contentType == mimeTypeApplicationOctetStream {
		r.Body = base64.StdEncoding.EncodeToString(body)
		r.IsBase64Encoded = true
	} else {
		r.Body = string(body)
	}
	return nil
}

type apiGatewayV2Response events.APIGatewayV2HTTPResponse

type apiGatewayV1Response events.APIGatewayProxyResponse

type albResponse events.ALBTargetGroupResponse

type lambdaHTTPResponse interface {
	TranscodeFrom(writer *lambdaHTTPResponseWriter) error
}

var _ lambdaHTTPResponse = (*apiGatewayV2Response)(nil)
var _ lambdaHTTPResponse = (*apiGatewayV1Response)(nil)
var _ lambdaHTTPResponse = (*albResponse)(nil)

type ambiguousLambdaResponse struct {
	bytes []byte
}

type leastCommonDenominatorResponse struct {
	StatusCode      int    `json:"statusCode"`
	Body            string `json:"body"`
	IsBase64Encoded bool   `json:"isBase64Encoded"`
}

func (r *ambiguousLambdaResponse) TranscodeFrom(httpResponse *lambdaHTTPResponseWriter) error {
	var out any
	if httpResponse.preparedResponse == nil {
		resp := &leastCommonDenominatorResponse{}
		resp.StatusCode = httpResponse.statusCode
		contentType := httpResponse.header.Get(contentTypeHeader)
		body := httpResponse.body.Bytes()
		if contentType == mimeTypeApplicationOctetStream {
			resp.Body = base64.StdEncoding.EncodeToString(body)
			resp.IsBase64Encoded = true
		} else {
			resp.Body = string(body)
		}
		out = resp
	} else {
		err := httpResponse.preparedResponse.TranscodeFrom(httpResponse)
		if err != nil {
			return fmt.Errorf("failed to transcode response: %w", err)
		}
		out = httpResponse.preparedResponse
	}

	jsonBytes, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("failed to marshal response to JSON: %w", err)
	}
	r.bytes = jsonBytes
	return nil
}
