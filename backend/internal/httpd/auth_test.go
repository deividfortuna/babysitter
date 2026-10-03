package httpd

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/events"
	"github.com/deividfortuna/babysitter/internal/ghauth"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

type fakeAuth struct {
	status     Auth
	token      string
	tokenErr   error
	prompt     SignInPrompt
	startErr   error
	cancels    int
	signOuts   int
	signOutErr error
	bus        *events.Bus
}

func (f *fakeAuth) Status(context.Context) Auth { return f.status }

func (f *fakeAuth) StartSignIn(context.Context) (SignInPrompt, error) { return f.prompt, f.startErr }

func (f *fakeAuth) CancelSignIn() { f.cancels++ }

func (f *fakeAuth) AppToken(context.Context) (string, error) { return f.token, f.tokenErr }

func (f *fakeAuth) SignOut(context.Context) error {
	f.signOuts++
	if f.bus != nil {
		f.bus.Publish(events.AuthChanged, "", 0)
	}
	return f.signOutErr
}

func authRouter(t *testing.T, auth AuthController) (http.Handler, *events.Bus) {
	bus := events.NewBus()
	return NewRouter(Deps{Log: testutil.Logger(t), Bus: bus, Auth: auth}), bus
}

func TestGetAuthAnswersTheStatus(t *testing.T) {
	want := Auth{
		State: "connected", Origin: "app", AppAvailable: true, Login: "octocat",
		InstallURL:    "https://github.com/apps/babysitter/installations/new",
		Installations: []AuthInstallation{{Login: "acme", AvatarURL: "https://avatars.githubusercontent.com/acme", Organization: true}},
	}
	h, _ := authRouter(t, &fakeAuth{status: want})

	var got Auth
	if rec := call(t, h, http.MethodGet, "/auth", "", &got); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("auth = %+v, want %+v", got, want)
	}
}

func TestAuthIsUnavailableWithoutController(t *testing.T) {
	h, _ := authRouter(t, nil)

	for _, req := range []struct{ method, path string }{{http.MethodGet, "/auth"}, {http.MethodPost, "/auth/signin"}, {http.MethodPost, "/auth/signout"}} {
		if rec := call(t, h, req.method, req.path, "", nil); rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s = %d, want 503", req.method, req.path, rec.Code)
		}
	}
}

func TestStartSignInAnswersThePrompt(t *testing.T) {
	prompt := SignInPrompt{UserCode: "ABCD-1234", VerificationURI: "https://github.com/login/device", ExpiresAt: time.Date(2026, 10, 2, 9, 15, 0, 0, time.UTC)}
	h, _ := authRouter(t, &fakeAuth{prompt: prompt})

	var got SignInPrompt
	if rec := call(t, h, http.MethodPost, "/auth/signin", "", &got); rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if got != prompt {
		t.Fatalf("prompt = %+v, want %+v", got, prompt)
	}
}

func TestStartSignInWithoutAppIsUnavailable(t *testing.T) {
	h, _ := authRouter(t, &fakeAuth{startErr: ghauth.ErrNoApp})

	rec := call(t, h, http.MethodPost, "/auth/signin", "", nil)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if code := errorCode(t, rec); code != "app_unavailable" {
		t.Fatalf("code = %q, want app_unavailable", code)
	}
}

func TestCancelSignIn(t *testing.T) {
	auth := &fakeAuth{}
	h, _ := authRouter(t, auth)

	if rec := call(t, h, http.MethodDelete, "/auth/signin", "", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if auth.cancels != 1 {
		t.Fatalf("cancels = %d, want 1", auth.cancels)
	}
}

func TestSignOutForgetsTheViewer(t *testing.T) {
	bus := events.NewBus()
	auth := &fakeAuth{bus: bus}
	logins := []string{"from-the-app", "from-gh"}
	h := NewRouter(Deps{Log: testutil.Logger(t), Bus: bus, Auth: auth, Viewer: func(context.Context) (Viewer, error) {
		login := logins[0]
		logins = logins[1:]
		return Viewer{Login: login}, nil
	}})
	var before, after Viewer
	call(t, h, http.MethodGet, "/viewer", "", &before)

	if rec := call(t, h, http.MethodPost, "/auth/signout", "", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	call(t, h, http.MethodGet, "/viewer", "", &after)

	if auth.signOuts != 1 {
		t.Fatalf("sign outs = %d, want 1", auth.signOuts)
	}
	if before.Login != "from-the-app" || after.Login != "from-gh" {
		t.Fatalf("viewer before = %q, after = %q; want the account of the next source after the sign out", before.Login, after.Login)
	}
}

func TestSignOutFailure(t *testing.T) {
	h, _ := authRouter(t, &fakeAuth{signOutErr: errors.New("permission denied")})

	if rec := call(t, h, http.MethodPost, "/auth/signout", "", nil); rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func tokenCall(t *testing.T, h http.Handler, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, Prefix+"/auth/token", nil)
	maps.Copy(req.Header, header)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAppTokenNeedsTheSecret(t *testing.T) {
	h := NewRouter(Deps{Log: testutil.Logger(t), Auth: &fakeAuth{token: "ghu_app"}, TokenSecret: "s3cret"})

	for _, tc := range []struct {
		name   string
		header http.Header
		status int
	}{
		{"no secret", http.Header{}, http.StatusForbidden},
		{"wrong secret", http.Header{TokenSecretHeader: {"guess"}}, http.StatusForbidden},
		{"secret from a browser", http.Header{TokenSecretHeader: {"s3cret"}, "Origin": {"http://127.0.0.1:5173"}}, http.StatusForbidden},
		{"secret", http.Header{TokenSecretHeader: {"s3cret"}}, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := tokenCall(t, h, tc.header)

			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body)
			}
			if given := strings.Contains(rec.Body.String(), "ghu_app"); given != (tc.status == http.StatusOK) {
				t.Fatalf("body = %s", rec.Body)
			}
		})
	}
}

func TestAppTokenWithoutASecretIsNeverGiven(t *testing.T) {
	h := NewRouter(Deps{Log: testutil.Logger(t), Auth: &fakeAuth{token: "ghu_app"}})

	if rec := tokenCall(t, h, http.Header{TokenSecretHeader: {""}}); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestAppTokenWhenTheAppIsNotInUse(t *testing.T) {
	h := NewRouter(Deps{Log: testutil.Logger(t), Auth: &fakeAuth{}, TokenSecret: "s3cret"})

	rec := tokenCall(t, h, http.Header{TokenSecretHeader: {"s3cret"}})

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "app_not_in_use") {
		t.Fatalf("body = %s, want the code app_not_in_use", rec.Body)
	}
}

func TestTheDaemonAnswersLoopbackHostsOnly(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := loopbackHostOnly(ok)

	for host, want := range map[string]int{
		"127.0.0.1:7777":      http.StatusNoContent,
		"localhost:7777":      http.StatusNoContent,
		"[::1]:7777":          http.StatusNoContent,
		"localhost":           http.StatusNoContent,
		"evil.example:7777":   http.StatusForbidden,
		"192.168.1.10:7777":   http.StatusForbidden,
		"127.0.0.1.nip.io:80": http.StatusForbidden,
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("Host %s = %d, want %d", host, rec.Code, want)
		}
	}
}
