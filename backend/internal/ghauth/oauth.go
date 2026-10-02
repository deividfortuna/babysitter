package ghauth

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/deividfortuna/babysitter/internal/ghclient"
)

const (
	gitHubURL    = "https://github.com"
	deviceGrant  = "urn:ietf:params:oauth:grant-type:device_code"
	refreshGrant = "refresh_token"
	slowDownStep = 5

	allowedPollFailures = 3
)

var (
	ErrCodeExpired = errors.New("the code expired before it was entered on GitHub: sign in again")
	ErrDenied      = errors.New("the sign in was cancelled on GitHub")

	errRefreshRefused = errors.New("GitHub refused to renew the app token")
	errPending        = errors.New("authorization_pending")
	errSlowDown       = errors.New("slow_down")

	oauthHTTP = &http.Client{Timeout: 30 * time.Second}
)

type DeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

const defaultCodeLifetime = 900

func (c DeviceCode) complete() bool {
	return c.DeviceCode != "" && c.UserCode != ""
}

func (c DeviceCode) Lifetime() time.Duration {
	return time.Duration(cmp.Or(c.ExpiresIn, defaultCodeLifetime)) * time.Second
}

type Token struct {
	AccessToken      string    `json:"accessToken"`
	ExpiresAt        time.Time `json:"expiresAt,omitzero"`
	RefreshToken     string    `json:"refreshToken,omitempty"`
	RefreshExpiresAt time.Time `json:"refreshExpiresAt,omitzero"`
}

type OAuth struct {
	BaseURL  string
	ClientID string
	Now      func() time.Time

	PollUnit time.Duration
}

type tokenAnswer struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             int    `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
	Interval              int    `json:"interval"`
	Error                 string `json:"error"`
	ErrorDescription      string `json:"error_description"`
}

func (o OAuth) RequestCode(ctx context.Context) (DeviceCode, error) {
	var code DeviceCode
	if err := o.post(ctx, "/login/device/code", url.Values{"client_id": {o.ClientID}}, &code); err != nil {
		return DeviceCode{}, fmt.Errorf("ask GitHub for a device code: %w", err)
	}
	if !code.complete() {
		return DeviceCode{}, errors.New("ask GitHub for a device code: the answer has no code")
	}
	return code, nil
}

func (o OAuth) Wait(ctx context.Context, code DeviceCode) (Token, error) {
	interval := max(code.Interval, 1)
	deadline := o.Now().Add(code.Lifetime())
	failures := 0
	for {
		if err := o.sleep(ctx, interval); err != nil {
			return Token{}, err
		}
		tok, err := o.exchange(ctx, code.DeviceCode)
		failures = failuresAfter(failures, err)
		switch {
		case err == nil:
			return tok, nil
		case failures > allowedPollFailures:
			return Token{}, err
		case errors.Is(err, errSlowDown):
			interval += slowDownStep
		case temporary(err), errors.Is(err, errPending):
		default:
			return Token{}, err
		}
		if !o.Now().Before(deadline) {
			return Token{}, ErrCodeExpired
		}
	}
}

func failuresAfter(failures int, err error) int {
	if temporary(err) {
		return failures + 1
	}
	return 0
}

type statusError struct {
	code   int
	status string
}

func (e statusError) Error() string {
	return "GitHub answered " + e.status
}

func temporary(err error) bool {
	if se, ok := errors.AsType[statusError](err); ok {
		return se.code >= http.StatusInternalServerError
	}
	_, transport := errors.AsType[*url.Error](err)
	return transport
}

func (o OAuth) Refresh(ctx context.Context, refreshToken string) (Token, error) {
	form := url.Values{"client_id": {o.ClientID}, "grant_type": {refreshGrant}, "refresh_token": {refreshToken}}
	var answer tokenAnswer
	if err := o.post(ctx, "/login/oauth/access_token", form, &answer); err != nil {
		return Token{}, fmt.Errorf("renew the app token: %w", err)
	}
	if answer.Error != "" {
		return Token{}, fmt.Errorf("%w: %s", errRefreshRefused, cmp.Or(answer.ErrorDescription, answer.Error))
	}
	return o.token(answer)
}

func (o OAuth) exchange(ctx context.Context, deviceCode string) (Token, error) {
	form := url.Values{"client_id": {o.ClientID}, "device_code": {deviceCode}, "grant_type": {deviceGrant}}
	var answer tokenAnswer
	if err := o.post(ctx, "/login/oauth/access_token", form, &answer); err != nil {
		return Token{}, fmt.Errorf("ask GitHub for the app token: %w", err)
	}
	switch answer.Error {
	case "":
		return o.token(answer)
	case "authorization_pending":
		return Token{}, errPending
	case "slow_down":
		return Token{}, errSlowDown
	case "expired_token":
		return Token{}, ErrCodeExpired
	case "access_denied":
		return Token{}, ErrDenied
	}
	return Token{}, fmt.Errorf("ask GitHub for the app token: %s", cmp.Or(answer.ErrorDescription, answer.Error))
}

func (o OAuth) token(answer tokenAnswer) (Token, error) {
	if answer.AccessToken == "" {
		return Token{}, errors.New("GitHub answered without a token")
	}
	now := o.Now()
	tok := Token{AccessToken: answer.AccessToken, RefreshToken: answer.RefreshToken}
	if answer.ExpiresIn > 0 {
		tok.ExpiresAt = now.Add(time.Duration(answer.ExpiresIn) * time.Second)
	}
	if answer.RefreshTokenExpiresIn > 0 {
		tok.RefreshExpiresAt = now.Add(time.Duration(answer.RefreshTokenExpiresIn) * time.Second)
	}
	return tok, nil
}

func (o OAuth) post(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cmp.Or(o.BaseURL, gitHubURL)+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusError{code: resp.StatusCode, status: resp.Status}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (o OAuth) sleep(ctx context.Context, intervals int) error {
	return ghclient.SleepCtx(ctx, time.Duration(intervals)*cmp.Or(o.PollUnit, time.Second))
}
