package ghclient

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/go-github/v91/github"
)

func New(token string, timeout time.Duration) (*github.Client, error) {
	c, err := github.NewClient(
		github.WithAuthToken(token),
		github.WithTimeout(timeout),
		github.WithTransport(sharedTransport()),
	)
	if err != nil {
		return nil, fmt.Errorf("new github client: %w", err)
	}
	return c, nil
}

func sharedTransport() http.RoundTripper {
	return sharedCaching{base: &meteringTransport{meter: sharedRates}}
}

type sharedCaching struct {
	base http.RoundTripper
}

func (t sharedCaching) RoundTrip(req *http.Request) (*http.Response, error) {
	return (&cachingTransport{cache: currentCache(), base: t.base}).RoundTrip(req)
}
