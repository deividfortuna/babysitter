package remote

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

const TokenQueryParam = "token"

var localOnlySuffixes = []string{"/control/shutdown", "/hook"}

func Guard(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPreflight(r) || isHealthCheck(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if !matches(token, presented(r)) {
			deny(w, http.StatusUnauthorized, "unauthorized", "the daemon wants the token of its remote access")
			return
		}
		if isLocalOnly(r.URL.Path) {
			deny(w, http.StatusForbidden, "local_only", "only a client on the machine of the daemon can call this")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isPreflight(r *http.Request) bool {
	return r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""
}

func isHealthCheck(path string) bool {
	return strings.HasSuffix(path, "/healthz")
}

func presented(r *http.Request) string {
	if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return strings.TrimSpace(bearer)
	}
	return r.URL.Query().Get(TokenQueryParam)
}

func matches(want, got string) bool {
	return got != "" && subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

func isLocalOnly(path string) bool {
	for _, suffix := range localOnlySuffixes {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

func deny(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}
