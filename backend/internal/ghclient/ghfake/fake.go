package ghfake

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-github/v91/github"
)

// The routes of the fake, as Action.Route names them and React takes them.
const (
	RouteUser            = "GET /user"
	RouteUserRepos       = "GET /user/repos"
	RouteRepo            = "GET /repos/{owner}/{repo}"
	RouteCompare         = "GET /repos/{owner}/{repo}/compare/{basehead}"
	RouteRules           = "GET /repos/{owner}/{repo}/rules/branches/{branch}"
	RouteProtection      = "GET /repos/{owner}/{repo}/branches/{branch}/protection"
	RouteLatestRelease   = "GET /repos/{owner}/{repo}/releases/latest"
	RoutePulls           = "GET /repos/{owner}/{repo}/pulls"
	RoutePull            = "GET /repos/{owner}/{repo}/pulls/{number}"
	RouteMerge           = "PUT /repos/{owner}/{repo}/pulls/{number}/merge"
	RouteRequestReviews  = "POST /repos/{owner}/{repo}/pulls/{number}/requested_reviewers"
	RouteReviews         = "GET /repos/{owner}/{repo}/pulls/{number}/reviews"
	RouteSubmitReview    = "POST /repos/{owner}/{repo}/pulls/{number}/reviews"
	RouteReviewComment   = "GET /repos/{owner}/{repo}/pulls/comments/{id}"
	RouteReviewComments  = "GET /repos/{owner}/{repo}/pulls/{number}/comments"
	RouteReply           = "POST /repos/{owner}/{repo}/pulls/{number}/comments"
	RouteIssueComment    = "GET /repos/{owner}/{repo}/issues/comments/{id}"
	RouteIssueComments   = "GET /repos/{owner}/{repo}/issues/{number}/comments"
	RouteComment         = "POST /repos/{owner}/{repo}/issues/{number}/comments"
	RouteCheckRuns       = "GET /repos/{owner}/{repo}/commits/{sha}/check-runs"
	RouteStatus          = "GET /repos/{owner}/{repo}/commits/{sha}/status"
	RouteWorkflowRuns    = "GET /repos/{owner}/{repo}/actions/runs"
	RouteJobs            = "GET /repos/{owner}/{repo}/actions/runs/{run}/jobs"
	RouteJobLogs         = "GET /repos/{owner}/{repo}/actions/jobs/{job}/logs"
	RouteJobLogDownload  = "GET /_logs/{owner}/{repo}/{job}"
	RouteGraphQL         = "POST /graphql"
	apiPrefix            = "/api/v3"
	defaultNotFoundError = "Not Found"
)

// AnyRoute makes React answer every route.
const AnyRoute = ""

// Action is one request the fake received.
type Action struct {
	Method string
	// Route is the route that matched, one of the Route constants, or empty
	// when no route did.
	Route string
	// Path is the path without the /api/v3 of the Enterprise URL.
	Path  string
	Query url.Values
	Vars  map[string]string
	Body  []byte
	// Status is the status the fake answered with, or 0 while the answer is
	// still on its way.
	Status int
}

// Decode reads the JSON body of the action into v.
func (a Action) Decode(t testing.TB, v any) {
	t.Helper()
	if err := json.Unmarshal(a.Body, v); err != nil {
		t.Fatalf("body of %s %s: %v", a.Method, a.Path, err)
	}
}

// Response is what a reactor answers.
type Response struct {
	Status int
	// Body is sent as it is. A Response without a Body sends GitHub's error
	// document with Message.
	Body    string
	Message string
	Header  map[string]string
}

// Reactor answers an action in place of the state when it returns true.
// It runs without the lock of the fake, so it may call Update.
type Reactor func(Action) (Response, bool)

type reactor struct {
	route string
	fn    Reactor
}

type route struct {
	method string
	name   string
	segs   []string
	serve  func(*call)
}

// GitHub is the fake. Its zero value is not usable; call New.
type GitHub struct {
	mu       sync.Mutex
	viewer   *github.User
	repos    map[string]*Repo
	order    []*Repo
	releases map[string]*github.RepositoryRelease
	actions  []Action
	reactors []reactor
	routes   []route
	rate     *Rate
	nextID   int64
	base     string
}

// Rate is the rate limit the fake reports on every response.
type Rate struct {
	Resource  string
	Limit     int
	Remaining int
	Reset     time.Time
}

// New returns a fake with alice as the authenticated user and no repository.
func New() *GitHub {
	g := &GitHub{
		viewer:   &github.User{Login: new("alice")},
		repos:    map[string]*Repo{},
		releases: map[string]*github.RepositoryRelease{},
		nextID:   1_000_000,
	}
	g.routes = []route{
		g.route(RouteUser, (*call).user),
		g.route(RouteUserRepos, (*call).userRepos),
		g.route(RouteRules, (*call).rules),
		g.route(RouteProtection, (*call).protection),
		g.route(RouteLatestRelease, (*call).latestRelease),
		g.route(RouteReviewComment, (*call).reviewComment),
		g.route(RouteIssueComment, (*call).issueComment),
		g.route(RoutePulls, (*call).pulls),
		g.route(RoutePull, (*call).pull),
		g.route(RouteMerge, (*call).merge),
		g.route(RouteRequestReviews, (*call).requestReviews),
		g.route(RouteReviews, (*call).reviews),
		g.route(RouteSubmitReview, (*call).submitReview),
		g.route(RouteReviewComments, (*call).reviewComments),
		g.route(RouteReply, (*call).reply),
		g.route(RouteIssueComments, (*call).issueComments),
		g.route(RouteComment, (*call).comment),
		g.route(RouteCheckRuns, (*call).checkRuns),
		g.route(RouteStatus, (*call).status),
		g.route(RouteWorkflowRuns, (*call).workflowRuns),
		g.route(RouteJobs, (*call).jobs),
		g.route(RouteJobLogs, (*call).jobLogs),
		g.route(RouteJobLogDownload, (*call).jobLogDownload),
		g.route(RouteGraphQL, (*call).graphql),
		g.route(RouteRepo, (*call).repo),
		g.route(RouteCompare, (*call).compare),
	}
	return g
}

func (g *GitHub) route(name string, serve func(*call)) route {
	method, pattern, _ := strings.Cut(name, " ")
	return route{method: method, name: name, segs: strings.Split(strings.Trim(pattern, "/"), "/"), serve: serve}
}

// Serve starts the fake for the length of the test.
func (g *GitHub) Serve(t testing.TB) *Server {
	t.Helper()
	s := Serve(t, g)
	g.mu.Lock()
	g.base = s.URL
	g.mu.Unlock()
	return s
}

// Client starts the fake and returns a client for it.
func (g *GitHub) Client(t testing.TB, opts ...github.ClientOptionsFunc) *github.Client {
	t.Helper()
	return g.Serve(t).Client(t, opts...)
}

// Update runs fn under the lock of the fake, for a test that changes the
// state while the code under test may be calling. fn must not call the
// methods of the fake that take the lock themselves: Repo, PR, React and
// the Action readers.
func (g *GitHub) Update(fn func()) {
	g.mu.Lock()
	defer g.mu.Unlock()
	fn()
}

// Viewer is the user GET /user answers with, and the author of the
// comments the fake receives.
func (g *GitHub) Viewer() *github.User {
	return g.viewer
}

// SetRate makes every response carry the rate limit headers of r, and nil
// takes them away.
func (g *GitHub) SetRate(r *Rate) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rate = r
}

// Release sets the latest release of a repository.
func (g *GitHub) Release(fullName string, r *github.RepositoryRelease) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.releases[strings.ToLower(fullName)] = r
}

// React puts fn in front of the state for route, or for every route with
// AnyRoute. The reactor added last runs first, as in client-go.
func (g *GitHub) React(route string, fn Reactor) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reactors = append([]reactor{{route: route, fn: fn}}, g.reactors...)
}

// Fail makes route answer status with a GitHub error document.
func (g *GitHub) Fail(route string, status int, message string) {
	g.React(route, func(Action) (Response, bool) {
		return Response{Status: status, Message: message}, true
	})
}

// Observe calls fn on every request before it is answered.
func (g *GitHub) Observe(fn func(Action)) {
	g.React(AnyRoute, func(a Action) (Response, bool) {
		fn(a)
		return Response{}, false
	})
}

// GraphQLError makes the GraphQL endpoint answer with errors, the way
// GitHub does: status 200 and no data.
func (g *GitHub) GraphQLError(message string) {
	g.React(RouteGraphQL, func(Action) (Response, bool) {
		body, _ := json.Marshal(map[string]any{"data": nil, "errors": []map[string]string{{"message": message}}})
		return Response{Status: http.StatusOK, Body: string(body)}, true
	})
}

// Actions returns every request so far, oldest first.
func (g *GitHub) Actions() []Action {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]Action(nil), g.actions...)
}

// Calls returns the requests to route.
func (g *GitHub) Calls(route string) []Action {
	var out []Action
	for _, a := range g.Actions() {
		if a.Route == route {
			out = append(out, a)
		}
	}
	return out
}

// Count returns how many requests went to route.
func (g *GitHub) Count(route string) int {
	return len(g.Calls(route))
}

// CountPath returns how many requests went to path, without /api/v3.
func (g *GitHub) CountPath(path string) int {
	n := 0
	for _, a := range g.Actions() {
		if a.Path == path {
			n++
		}
	}
	return n
}

// Total returns how many requests the fake received.
func (g *GitHub) Total() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.actions)
}

// Reset forgets the recorded requests. The state stays.
func (g *GitHub) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.actions = nil
}

func (g *GitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	path := strings.TrimPrefix(r.URL.Path, apiPrefix)
	a := Action{Method: r.Method, Path: path, Query: r.URL.Query(), Body: body}
	rt, vars := g.match(r.Method, path)
	if rt != nil {
		a.Route, a.Vars = rt.name, vars
	}

	g.mu.Lock()
	i := len(g.actions)
	g.actions = append(g.actions, a)
	reactors := append([]reactor(nil), g.reactors...)
	g.mu.Unlock()

	sw := &statusWriter{ResponseWriter: w}
	defer func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.actions[i].Status = sw.status()
	}()
	for _, re := range reactors {
		if re.route != AnyRoute && re.route != a.Route {
			continue
		}
		if resp, ok := re.fn(a); ok {
			g.mu.Lock()
			g.headers(sw)
			g.mu.Unlock()
			writeResponse(sw, resp)
			return
		}
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	g.headers(sw)
	if rt == nil {
		writeError(sw, http.StatusNotFound, defaultNotFoundError)
		return
	}
	rt.serve(&call{g: g, w: sw, r: r, a: a})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.code == 0 {
		w.code = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if w.code == 0 {
		w.code = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}

func (w *statusWriter) status() int {
	if w.code == 0 {
		return http.StatusOK
	}
	return w.code
}

func (g *GitHub) match(method, path string) (*route, map[string]string) {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	for i := range g.routes {
		rt := &g.routes[i]
		if rt.method != method || len(rt.segs) != len(segs) {
			continue
		}
		vars := map[string]string{}
		ok := true
		for j, s := range rt.segs {
			if strings.HasPrefix(s, "{") {
				vars[strings.Trim(s, "{}")] = segs[j]
				continue
			}
			if s != segs[j] {
				ok = false
				break
			}
		}
		if ok {
			return rt, vars
		}
	}
	return nil, nil
}

func (g *GitHub) headers(w http.ResponseWriter) {
	if g.rate == nil {
		return
	}
	h := w.Header()
	h.Set("X-RateLimit-Limit", fmt.Sprint(g.rate.Limit))
	h.Set("X-RateLimit-Remaining", fmt.Sprint(g.rate.Remaining))
	h.Set("X-RateLimit-Reset", fmt.Sprint(g.rate.Reset.Unix()))
	if g.rate.Resource != "" {
		h.Set("X-RateLimit-Resource", g.rate.Resource)
	}
}

func (g *GitHub) id() int64 {
	g.nextID++
	return g.nextID
}

func writeResponse(w http.ResponseWriter, resp Response) {
	for k, v := range resp.Header {
		w.Header().Set(k, v)
	}
	status := resp.Status
	if status == 0 {
		status = http.StatusOK
	}
	if resp.Body == "" && status >= http.StatusBadRequest {
		msg := resp.Message
		if msg == "" {
			msg = http.StatusText(status)
		}
		writeError(w, status, msg)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, resp.Body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"message": message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}
