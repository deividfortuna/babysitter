package httpd

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/deividfortuna/babysitter/internal/redact"
	"github.com/deividfortuna/babysitter/internal/store"
)

type errorCase struct {
	status    int
	code      string
	sentinels []error
}

func badRequest(code string, errs ...error) errorCase {
	return errorCase{http.StatusBadRequest, code, errs}
}

func notFound(code string, errs ...error) errorCase {
	return errorCase{http.StatusNotFound, code, errs}
}

func conflict(code string, errs ...error) errorCase {
	return errorCase{http.StatusConflict, code, errs}
}

func unavailable(code string, errs ...error) errorCase {
	return errorCase{http.StatusServiceUnavailable, code, errs}
}

func unprocessable(code string, errs ...error) errorCase {
	return errorCase{http.StatusUnprocessableEntity, code, errs}
}

type errorMap struct {
	fallback string
	cases    []errorCase
}

func newErrorMap(fallback string, cases ...errorCase) errorMap {
	return errorMap{fallback: fallback, cases: cases}
}

func (m errorMap) write(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	message := redact.Text(err.Error())
	for _, c := range m.cases {
		for _, sentinel := range c.sentinels {
			if errors.Is(err, sentinel) {
				writeError(w, c.status, c.code, message)
				return true
			}
		}
	}
	writeError(w, http.StatusInternalServerError, m.fallback, message)
	return true
}

var storeErrors = newErrorMap("store_failed",
	notFound("watch_not_found", store.ErrWatchNotFound),
	notFound("repository_not_found", store.ErrRepoNotFound),
	conflict("repository_exists", store.ErrRepoExists),
	badRequest("bad_request", store.ErrInvalidSettings, store.ErrInvalidNotification),
)

func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "bad_request", name+" must be a positive integer")
		return 0, false
	}
	return id, true
}

func queryInt(w http.ResponseWriter, r *http.Request, name string, fallback int64) (int64, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, true
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", name+" must be an integer")
		return 0, false
	}
	return n, true
}
