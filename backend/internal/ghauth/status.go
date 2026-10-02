package ghauth

import (
	"context"
	"errors"
	"time"
)

type State string

const (
	StateSignedOut State = "signed_out"
	StateConnected State = "connected"
	StateNotInUse  State = "not_in_use"
	StateExpired   State = "expired"
)

type Status struct {
	State     State
	Origin    Origin
	Login     string
	AvatarURL string
	ExpiresAt time.Time
	Err       error
}

func (a *Auth) Status(ctx context.Context) Status {
	cred, err := a.Credential(ctx)
	st := Status{Origin: cred.Origin}
	c, loadErr := a.file.Load()
	signedIn := loadErr == nil
	if signedIn {
		st.Login, st.AvatarURL, st.ExpiresAt = c.Login, c.AvatarURL, c.ExpiresAt
	}
	st.State = stateOf(signedIn, cred.Origin, err)
	if reportable(err) {
		st.Err = err
	}
	return st
}

func (o Origin) comesFirst() bool {
	return o == OriginFlag || o == OriginEnv
}

func stateOf(signedIn bool, origin Origin, err error) State {
	switch {
	case !signedIn:
		return StateSignedOut
	case errors.Is(err, ErrSessionExpired):
		return StateExpired
	case origin.comesFirst():
		return StateNotInUse
	}
	return StateConnected
}

func reportable(err error) bool {
	expected := errors.Is(err, ErrNoToken) || errors.Is(err, ErrSessionExpired)
	return err != nil && !expected
}
