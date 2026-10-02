package ghauth

import (
	"context"
	"sync"
	"time"
)

type Identity struct {
	Login     string
	AvatarURL string
}

type Whoami func(ctx context.Context, token string) (Identity, error)

type Prompt struct {
	UserCode        string
	VerificationURI string
	ExpiresAt       time.Time
}

func (a *Auth) RequestCode(ctx context.Context) (DeviceCode, error) {
	if !a.app.Available() {
		return DeviceCode{}, ErrNoApp
	}
	return a.oauth.RequestCode(ctx)
}

func (a *Auth) Complete(ctx context.Context, code DeviceCode, whoami Whoami) (Credentials, error) {
	tok, err := a.oauth.Wait(ctx, code)
	if err != nil {
		return Credentials{}, err
	}
	who, err := whoami(ctx, tok.AccessToken)
	if err != nil {
		return Credentials{}, err
	}
	c := Credentials{Login: who.Login, AvatarURL: who.AvatarURL, Token: tok}
	unlock, err := a.file.Lock(ctx)
	if err != nil {
		return Credentials{}, err
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return Credentials{}, err
	}
	if err := a.file.Save(c); err != nil {
		return Credentials{}, err
	}
	a.hold(nil)
	a.announce(identity{login: who.Login})
	return c, nil
}

type attempt struct {
	prompt Prompt
	cancel context.CancelFunc
}

type SignIns struct {
	auth   *Auth
	whoami Whoami
	base   context.Context

	startMu sync.Mutex

	mu      sync.Mutex
	active  *attempt
	lastErr error
}

func (a *Auth) SignIns(base context.Context, whoami Whoami) *SignIns {
	return &SignIns{auth: a, whoami: whoami, base: base}
}

func (s *SignIns) Start(ctx context.Context) (Prompt, error) {
	s.startMu.Lock()
	defer s.startMu.Unlock()
	if prompt, waiting := s.Current(); waiting {
		return prompt, nil
	}
	code, err := s.auth.RequestCode(ctx)
	if err != nil {
		return Prompt{}, err
	}
	run, cancel := context.WithCancel(s.base)
	att := &attempt{
		prompt: Prompt{
			UserCode:        code.UserCode,
			VerificationURI: code.VerificationURI,
			ExpiresAt:       s.auth.now().Add(code.Lifetime()),
		},
		cancel: cancel,
	}
	s.mu.Lock()
	s.active, s.lastErr = att, nil
	s.mu.Unlock()
	go s.complete(run, att, code)
	s.auth.changed()
	return att.prompt, nil
}

func (s *SignIns) Current() (Prompt, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == nil {
		return Prompt{}, false
	}
	return s.active.prompt, true
}

func (s *SignIns) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

func (s *SignIns) Cancel() {
	s.mu.Lock()
	att := s.active
	s.active = nil
	s.mu.Unlock()
	if att == nil {
		return
	}
	att.cancel()
	s.auth.changed()
}

func (s *SignIns) complete(ctx context.Context, att *attempt, code DeviceCode) {
	defer att.cancel()
	_, err := s.auth.Complete(ctx, code, s.whoami)
	s.mu.Lock()
	current := s.active == att
	if current {
		s.active, s.lastErr = nil, err
	}
	s.mu.Unlock()
	if current {
		s.auth.changed()
	}
}
