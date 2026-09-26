// Package ghfake is the GitHub the backend tests talk to.
//
// GitHub keeps repositories and pull requests in memory, answers the REST
// and GraphQL calls the daemon makes from that state, applies the writes
// (a merge closes the pull request, a posted comment shows in the next
// list), and records every request as an Action. A test sets the state,
// runs the code, and asserts on the state and on the recorded actions.
// Reactors answer a route in place of the state, which is how a test makes
// GitHub fail.
//
// Serve starts any handler as a GitHub Enterprise server, for the tests of
// ghclient itself that need a response GitHub State cannot express.
//
// The package is for tests only. It sits next to the client it fakes, like
// the fake clientset of client-go, and internal/ keeps it out of the API.
package ghfake

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-github/v91/github"
)

// Server is a running fake GitHub API. go-github puts /api/v3 in front of
// every path of an Enterprise URL, so the handler sees /api/v3/repos/...
type Server struct {
	URL string
	srv *httptest.Server
}

// Serve starts h for the length of the test.
func Serve(t testing.TB, h http.Handler) *Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Server{URL: srv.URL, srv: srv}
}

// Close stops the server before the test ends, for a test of a GitHub that
// cannot be reached.
func (s *Server) Close() {
	s.srv.Close()
}

// NewClient returns a go-github client for the server. opts come after the
// URLs, so a test can add a transport or an HTTP client of its own.
func (s *Server) NewClient(opts ...github.ClientOptionsFunc) (*github.Client, error) {
	return github.NewClient(append([]github.ClientOptionsFunc{github.WithEnterpriseURLs(s.URL, s.URL)}, opts...)...)
}

// Client is NewClient for a test, which fails when the client cannot be made.
func (s *Server) Client(t testing.TB, opts ...github.ClientOptionsFunc) *github.Client {
	t.Helper()
	c, err := s.NewClient(opts...)
	if err != nil {
		t.Fatalf("github client for %s: %v", s.URL, err)
	}
	return c
}
