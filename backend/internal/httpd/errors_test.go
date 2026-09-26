package httpd

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const secretToken = "ghp_abcdefghijklmnopqrstuvwxyz1234"

func TestAnErrorResponseCarriesNoToken(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("no such watch")
	m := newErrorMap("watch_failed", notFound("watch_not_found", sentinel))
	for _, err := range []error{
		errors.New("GET https://x-access-token:" + secretToken + "@api.github.com/repos: 401"),
		errors.Join(sentinel, errors.New("bearer "+secretToken)),
	} {
		rec := httptest.NewRecorder()
		if !m.write(rec, err) {
			t.Fatal("write() reported nothing written")
		}
		if strings.Contains(rec.Body.String(), secretToken) {
			t.Fatalf("the response carries the token: %s", rec.Body)
		}
	}
}

func TestAnErrorResponseKeepsItsStatus(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("no such watch")
	m := newErrorMap("watch_failed", notFound("watch_not_found", sentinel))
	rec := httptest.NewRecorder()
	m.write(rec, sentinel)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "watch_not_found") {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}
