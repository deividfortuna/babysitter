package remote

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func guarded(t *testing.T) http.Handler {
	t.Helper()
	return Guard(func() string { return "secret" }, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
}

func serve(h http.Handler, r *http.Request) int {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec.Code
}

func TestGuardLetsTheTokenThrough(t *testing.T) {
	h := guarded(t)

	bearer := httptest.NewRequest(http.MethodGet, "/api/v1/watches", nil)
	bearer.Header.Set("Authorization", "Bearer secret")
	if got := serve(h, bearer); got != http.StatusTeapot {
		t.Fatalf("bearer token: status %d, want %d", got, http.StatusTeapot)
	}

	for _, path := range []string{"/api/v1/events", "/api/v1/logs/stream"} {
		query := httptest.NewRequest(http.MethodGet, path+"?token=secret", nil)
		if got := serve(h, query); got != http.StatusTeapot {
			t.Fatalf("%s with the token in the query: status %d, want %d", path, got, http.StatusTeapot)
		}
	}
}

func TestGuardTakesTheTokenInTheQueryOnlyForTheEventStreams(t *testing.T) {
	h := guarded(t)

	query := httptest.NewRequest(http.MethodGet, "/api/v1/watches?token=secret", nil)
	if got := serve(h, query); got != http.StatusUnauthorized {
		t.Fatalf("token in the query of an API route: status %d, want %d", got, http.StatusUnauthorized)
	}
}

func TestGuardRefusesAMissingOrWrongToken(t *testing.T) {
	h := guarded(t)

	if got := serve(h, httptest.NewRequest(http.MethodGet, "/api/v1/watches", nil)); got != http.StatusUnauthorized {
		t.Fatalf("no token: status %d, want %d", got, http.StatusUnauthorized)
	}
	wrong := httptest.NewRequest(http.MethodGet, "/api/v1/watches", nil)
	wrong.Header.Set("Authorization", "Bearer nope")
	if got := serve(h, wrong); got != http.StatusUnauthorized {
		t.Fatalf("wrong token: status %d, want %d", got, http.StatusUnauthorized)
	}
}

func TestGuardLetsThePreflightAndTheHealthCheckThrough(t *testing.T) {
	h := guarded(t)

	preflight := httptest.NewRequest(http.MethodOptions, "/api/v1/watches", nil)
	preflight.Header.Set("Access-Control-Request-Method", "GET")
	if got := serve(h, preflight); got != http.StatusTeapot {
		t.Fatalf("preflight: status %d, want %d", got, http.StatusTeapot)
	}
	if got := serve(h, httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)); got != http.StatusTeapot {
		t.Fatalf("health check: status %d, want %d", got, http.StatusTeapot)
	}
}

func TestGuardKeepsTheLocalOnlyRoutesLocal(t *testing.T) {
	h := guarded(t)

	for _, path := range []string{"/api/v1/control/shutdown", "/api/v1/watches/3/hook", "/api/v1/auth/token"} {
		r := httptest.NewRequest(http.MethodPost, path, nil)
		r.Header.Set("Authorization", "Bearer secret")
		if got := serve(h, r); got != http.StatusForbidden {
			t.Fatalf("%s: status %d, want %d", path, got, http.StatusForbidden)
		}
	}
}
