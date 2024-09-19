package httpmiddleware

import (
	"log/slog"
	"net/http"

	"github.com/felixge/httpsnoop"
)

func Logging(level slog.Level) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			method := r.Method
			path := r.URL.Path
			host := r.Host
			queryParams := r.URL.Query()
			headers := r.Header
			snoop := httpsnoop.CaptureMetrics(next, w, r)
			slog.Log(
				r.Context(),
				level,
				"received http request",
				"request.method", method,
				"request.path", path,
				"request.host", host,
				"request.query_params", queryParams,
				"request.headers", headers,
				"request.content_length", r.ContentLength,
				"response.status", snoop.Code,
				"response.content_length", snoop.Written,
				"response.headers", w.Header(),
				"response.duration", snoop.Duration,
			)
		})
	}
}
