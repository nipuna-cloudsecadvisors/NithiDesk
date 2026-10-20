package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/httpapi"
)

func testRouter(checkers ...httpapi.Checker) http.Handler {
	return httpapi.NewRouter(httpapi.Options{
		Logger:   slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Version:  "test",
		Checkers: checkers,
	})
}

func okChecker(name string) httpapi.Checker {
	return httpapi.CheckerFunc{DependencyName: name, Fn: func(context.Context) error { return nil }}
}

func failingChecker(name, detail string) httpapi.Checker {
	return httpapi.CheckerFunc{
		DependencyName: name,
		Fn:             func(context.Context) error { return errors.New(detail) },
	}
}

func get(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

func TestHealthzIsIndependentOfDependencies(t *testing.T) {
	recorder := get(t, testRouter(failingChecker("database", "down")), "/healthz")

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /healthz = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestReadyzReportsEveryDependency(t *testing.T) {
	recorder := get(t, testRouter(okChecker("database"), okChecker("objectstore")), "/readyz")

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /readyz = %d, want %d", recorder.Code, http.StatusOK)
	}
	var body struct {
		Status       string            `json:"status"`
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if body.Status != "ready" {
		t.Errorf("status = %q, want ready", body.Status)
	}
	for _, dependency := range []string{"database", "objectstore"} {
		if body.Dependencies[dependency] != "ok" {
			t.Errorf("dependencies[%q] = %q, want ok", dependency, body.Dependencies[dependency])
		}
	}
}

func TestReadyzFailsWhenADependencyIsDown(t *testing.T) {
	recorder := get(t, testRouter(okChecker("database"), failingChecker("objectstore", "bucket missing")), "/readyz")

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /readyz = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

// A dependency error can name a host, a bucket, or a credential. Safe error
// messages are a security-plan requirement (docs/10-security-plan.md §15), so
// the detail must stay in the log and out of the response body.
func TestReadyzDoesNotLeakDependencyDetail(t *testing.T) {
	recorder := get(t, testRouter(failingChecker("database", "dial tcp 10.1.2.3:5432: connection refused")), "/readyz")

	if strings.Contains(recorder.Body.String(), "10.1.2.3") {
		t.Fatalf("response leaked connection detail: %s", recorder.Body.String())
	}
}

func TestVersionEndpoint(t *testing.T) {
	recorder := get(t, testRouter(), "/version")

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /version = %d, want %d", recorder.Code, http.StatusOK)
	}
	var body struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if body.Version != "test" {
		t.Errorf("version = %q, want test", body.Version)
	}
}

func TestUnknownRouteIsNotFound(t *testing.T) {
	recorder := get(t, testRouter(), "/matters")

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("GET /matters = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestEveryResponseCarriesRequestIDAndSecurityHeaders(t *testing.T) {
	recorder := get(t, testRouter(), "/healthz")

	if recorder.Header().Get(httpapi.RequestIDHeader) == "" {
		t.Error("response is missing a request id header")
	}
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
		"Cache-Control":          "no-store",
	} {
		if got := recorder.Header().Get(header); got != want {
			t.Errorf("header %s = %q, want %q", header, got, want)
		}
	}
}

func TestRequestIDsAreUnique(t *testing.T) {
	router := testRouter()

	first := get(t, router, "/healthz").Header().Get(httpapi.RequestIDHeader)
	second := get(t, router, "/healthz").Header().Get(httpapi.RequestIDHeader)

	if first == second {
		t.Errorf("two requests shared the request id %q", first)
	}
}

// A client must not be able to choose its own correlation id.
func TestClientSuppliedRequestIDIsIgnored(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set(httpapi.RequestIDHeader, "forged-id")

	testRouter().ServeHTTP(recorder, request)

	if recorder.Header().Get(httpapi.RequestIDHeader) == "forged-id" {
		t.Error("the router echoed a client-supplied request id")
	}
}
