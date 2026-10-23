// Package httpapi wires the HTTP surface of the API binary.
//
// Release 1 Foundation 1 exposes only the operational endpoints needed to
// prove the stack runs. Product routes arrive with the feature tasks, and
// every one of them must go through the central authorisation policy in
// internal/authz — see that package's contract.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// Checker reports whether one dependency is ready.
type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

// CheckerFunc adapts a function to Checker.
type CheckerFunc struct {
	DependencyName string
	Fn             func(ctx context.Context) error
}

// Name returns the dependency name used in the readiness response.
func (c CheckerFunc) Name() string { return c.DependencyName }

// Check runs the underlying function.
func (c CheckerFunc) Check(ctx context.Context) error { return c.Fn(ctx) }

// Options configures the router.
type Options struct {
	Logger   *slog.Logger
	Version  string
	Checkers []Checker
	// CheckTimeout bounds the whole readiness probe.
	CheckTimeout time.Duration

	// API, when set, mounts the /api/v1 product routes.
	API *Services
	// SecureCookies sets the Secure flag on the session cookie (always on
	// outside development).
	SecureCookies bool
	// SessionMaxAge is the cookie lifetime; the server enforces expiry itself.
	SessionMaxAge time.Duration
	// StaticDir, when set, serves the built single-page app (ADR 0008).
	StaticDir string
	// AuthRatePerMinute and AuthBurst bound sign-in attempts per client IP.
	// Zero means the defaults (20 per minute, burst 10).
	AuthRatePerMinute int
	AuthBurst         int
}

// NewRouter builds the HTTP handler for the API binary.
func NewRouter(opts Options) http.Handler {
	if opts.CheckTimeout <= 0 {
		opts.CheckTimeout = 5 * time.Second
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleLive)
	mux.HandleFunc("GET /readyz", handleReady(opts))
	mux.HandleFunc("GET /version", handleVersion(opts.Version))
	if opts.API != nil {
		mountAPI(mux, *opts.API, opts)
	}
	if opts.StaticDir != "" {
		mux.Handle("/", spaHandler(opts.StaticDir))
	}

	return RequestID(RequestLogger(opts.Logger, Recover(opts.Logger, SecurityHeaders(mux))))
}

// handleLive answers whether the process is running. It touches no dependency,
// so an orchestrator does not restart a healthy process because the database
// is briefly unavailable.
func handleLive(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReady answers whether the process can serve traffic: it checks every
// dependency and names the ones that are not ready.
func handleReady(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), opts.CheckTimeout)
		defer cancel()

		dependencies := make(map[string]string, len(opts.Checkers))
		ready := true
		for _, checker := range opts.Checkers {
			if err := checker.Check(ctx); err != nil {
				ready = false
				dependencies[checker.Name()] = "unavailable"
				// The reason goes to the log, not to the response body: an
				// error string can carry connection details.
				opts.Logger.Warn("dependency not ready",
					slog.String("component", checker.Name()),
					slog.String("error", err.Error()),
				)
				continue
			}
			dependencies[checker.Name()] = "ok"
		}

		status := http.StatusOK
		state := "ready"
		if !ready {
			status = http.StatusServiceUnavailable
			state = "not_ready"
		}
		writeJSON(w, status, map[string]any{"status": state, "dependencies": dependencies})
	}
}

func handleVersion(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"version": version})
	}
}

// writeJSON writes a JSON response. Callers pass only values that are safe to
// expose; no error string from a dependency is ever put in a response body.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
		// The header is already written, so there is nothing to recover to.
		return
	}
}
