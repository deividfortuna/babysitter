package ghclient

import (
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/google/go-github/v91/github"
)

type downloadStatus struct {
	status int
	msg    string
}

func (d *downloadStatus) Error() string { return d.msg }

func downloadError(status int, what string) error {
	return &downloadStatus{status: status, msg: fmt.Sprintf("download the %s: %d %s", what, status, http.StatusText(status))}
}

func HasStatus(err error, codes ...int) bool {
	status, ok := statusOf(err)
	return ok && slices.Contains(codes, status)
}

func statusOf(err error) (int, bool) {
	var ghErr *github.ErrorResponse
	if errors.As(err, &ghErr) && ghErr.Response != nil {
		return ghErr.Response.StatusCode, true
	}
	if dl, ok := errors.AsType[*downloadStatus](err); ok {
		return dl.status, true
	}
	return 0, false
}

func IsGone(err error) bool {
	return HasStatus(err, http.StatusNotFound, http.StatusGone)
}

func IsDenied(err error) bool {
	return HasStatus(err, http.StatusUnauthorized, http.StatusForbidden, http.StatusGone)
}
