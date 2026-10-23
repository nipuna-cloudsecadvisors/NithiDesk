package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"time"
)

type contextKey string

const requestIDKey contextKey = "request_id"

// RequestIDHeader is the header a request id is read from and echoed back on.
const RequestIDHeader = "X-Request-Id"

// RequestIDFrom returns the request id carried by the context, if any.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// RequestID attaches a request id to every request so a log line, an audit
// record, and a client report can be correlated without logging content.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newRequestID()
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// A client-supplied request id is deliberately not trusted: it would let a
// caller forge correlation ids across firms.
func newRequestID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand does not fail in practice; fall back to a timestamp so
		// the request still gets a usable, non-empty id.
		return "ts-" + time.Now().UTC().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(buf)
}

// statusRecorder captures the response status for the access log.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// RequestLogger writes one structured line per request. It logs the method,
// the route path, the status, the duration, the client IP, and the request id
// — identifiers and operational facts only. Query strings are not logged,
// because a search term can contain a client name.
func RequestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: 0}

		next.ServeHTTP(recorder, r)

		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		logger.Info("http request",
			slog.String("request_id", RequestIDFrom(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", status),
			slog.String("duration", time.Since(started).String()),
			slog.String("client_ip", clientIP(r)),
		)
	})
}

// Recover turns a panic into a 500 without leaking a stack trace to the
// client; the detail goes to the log only.
func Recover(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("panic recovered",
					slog.String("request_id", RequestIDFrom(r.Context())),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("error", "panic"),
				)
				writeJSON(w, http.StatusInternalServerError, errorBody{Error: errorDetail{Code: "internal_error",
					RequestID: RequestIDFrom(r.Context())}})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders sets the response headers that apply to every route.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("Cross-Origin-Opener-Policy", "same-origin")
		header.Set("Cache-Control", "no-store")
		// API responses are data, never documents; the web app handler
		// replaces this with its own policy.
		header.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		header.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
