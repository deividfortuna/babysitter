package ghauth

import (
	"context"
	"errors"
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
	if err := a.file.Save(c); err != nil {
		return Credentials{}, err
	}
	a.announce(who.Login)
	return c, nil
}

type SignIns struct {
	auth   *Auth
	whoami Whoami
	base   context.Context

	mu      sync.Mutex
	prompt  *Prompt
	cancel  context.CancelFunc
	lastErr error
}

func (a *Auth) SignIns(base context.Context, whoami Whoami) *SignIns {
	return &SignIns{auth: a, whoami: whoami, base: base}
}

func (s *SignIns) Start(ctx context.Context) (Prompt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.prompt != nil {
		return *s.prompt, nil
	}
	code, err := s.auth.RequestCode(ctx)
	if err != nil {
		return Prompt{}, err
	}
	prompt := Prompt{
		UserCode:        code.UserCode,
		VerificationURI: code.VerificationURI,
		ExpiresAt:       s.auth.now().Add(time.Duration(code.ExpiresIn) * time.Second),
	}
	run, cancel := context.WithCancel(s.base)
	s.prompt, s.cancel, s.lastErr = &prompt, cancel, nil
	go s.complete(run, cancel, code)
	s.auth.changed()
	return prompt, nil
}

func (s *SignIns) Current() (Prompt, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.prompt == nil {
		return Prompt{}, false
	}
	return *s.prompt, true
}

func (s *SignIns) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

func (s *SignIns) Cancel() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *SignIns) complete(ctx context.Context, cancel context.CancelFunc, code DeviceCode) {
	defer cancel()
	_, err := s.auth.Complete(ctx, code, s.whoami)
	if errors.Is(err, context.Canceled) {
		err = nil
	}
	s.mu.Lock()
	s.prompt, s.cancel, s.lastErr = nil, nil, err
	s.mu.Unlock()
	s.auth.changed()
}
