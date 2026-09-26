package ghclient

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/go-github/v91/github"
)

func ghError(status int) error {
	return fmt.Errorf("call failed: %w", &github.ErrorResponse{
		Response: &http.Response{StatusCode: status},
		Message:  http.StatusText(status),
	})
}

func TestHasStatusReadsTheAnswerOfTheAPIAndOfADownload(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		err   error
		codes []int
		want  bool
	}{
		{"api answer of one of the codes", ghError(http.StatusGone), []int{http.StatusForbidden, http.StatusGone}, true},
		{"api answer of another code", ghError(http.StatusBadGateway), []int{http.StatusGone}, false},
		{"download answer of one of the codes", downloadError(http.StatusNotFound, "logs of job o/r/9"), []int{http.StatusNotFound}, true},
		{"download answer of another code", downloadError(http.StatusBadGateway, "logs of job o/r/9"), []int{http.StatusNotFound}, false},
		{"no answer at all", errors.New("network down"), []int{http.StatusNotFound}, false},
		{"no error", nil, []int{http.StatusNotFound}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := HasStatus(c.err, c.codes...); got != c.want {
				t.Fatalf("HasStatus(%v, %v) = %v, want %v", c.err, c.codes, got, c.want)
			}
		})
	}
}

func TestIsDeniedNamesTheStatusesOfAResourceTheTokenCannotHave(t *testing.T) {
	t.Parallel()
	denied := []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusGone}
	for _, status := range denied {
		if !IsDenied(ghError(status)) {
			t.Errorf("IsDenied(%d) = false, want true", status)
		}
	}
	for _, status := range []int{http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError} {
		if IsDenied(ghError(status)) {
			t.Errorf("IsDenied(%d) = true, want false", status)
		}
	}
}

func TestIsNotFoundAndIsRefusedReadTheSameStatuses(t *testing.T) {
	t.Parallel()
	if !IsNotFound(ghError(http.StatusNotFound)) || IsNotFound(ghError(http.StatusGone)) {
		t.Error("IsNotFound answers something other than 404")
	}
	for _, status := range []int{http.StatusMethodNotAllowed, http.StatusConflict, http.StatusUnprocessableEntity} {
		if !IsRefused(ghError(status)) {
			t.Errorf("IsRefused(%d) = false, want true", status)
		}
	}
	if IsRefused(ghError(http.StatusNotFound)) {
		t.Error("IsRefused(404) = true, want false")
	}
	if !IsRefused(fmt.Errorf("merge: %w: no reason given", ErrNotMerged)) {
		t.Error("IsRefused of an answer that merged nothing = false, want true")
	}
}
