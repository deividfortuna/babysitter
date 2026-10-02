package ghauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/runfile"
)

const (
	signInFileName = "github-app.json"
	lockRetry      = 50 * time.Millisecond
)

var ErrSignedOut = errors.New("not signed in with the GitHub App")

type Credentials struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatarUrl,omitempty"`
	Token
}

type credentialsFile struct {
	path string
}

func newCredentialsFile(dataDir string) credentialsFile {
	return credentialsFile{path: filepath.Join(dataDir, signInFileName)}
}

func (f credentialsFile) Load() (Credentials, error) {
	b, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return Credentials{}, ErrSignedOut
	}
	if err != nil {
		return Credentials{}, fmt.Errorf("read the GitHub App sign in: %w", err)
	}
	var c Credentials
	if err := json.Unmarshal(b, &c); err != nil {
		return Credentials{}, fmt.Errorf("read the GitHub App sign in %s: %w", f.path, err)
	}
	if c.AccessToken == "" {
		return Credentials{}, ErrSignedOut
	}
	return c, nil
}

func (f credentialsFile) Save(c Credentials) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(f.path, b); err != nil {
		return fmt.Errorf("save the GitHub App sign in: %w", err)
	}
	return nil
}

func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return runfile.ReplaceFile(path, b, 0o600)
}

type signOut struct {
	SignedOutAt time.Time `json:"signedOutAt"`
}

func (f credentialsFile) MarkSignedOut(at time.Time) error {
	b, err := json.Marshal(signOut{SignedOutAt: at})
	if err != nil {
		return err
	}
	if err := writeAtomic(f.path, b); err != nil {
		return fmt.Errorf("remove the GitHub App sign in: %w", err)
	}
	return nil
}

func (f credentialsFile) SignedOutAt() time.Time {
	b, err := os.ReadFile(f.path)
	if err != nil {
		return time.Time{}
	}
	var s signOut
	if json.Unmarshal(b, &s) != nil {
		return time.Time{}
	}
	return s.SignedOutAt
}

func (f credentialsFile) Lock(ctx context.Context) (unlock func(), err error) {
	path := f.path + ".lock"
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("lock the GitHub App sign in: %w", err)
	}
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock the GitHub App sign in: %w", err)
	}
	for {
		held, err := tryLock(lock)
		if err != nil {
			_ = lock.Close()
			return nil, fmt.Errorf("lock the GitHub App sign in: %w", err)
		}
		if held {
			return func() {
				unlockFile(lock)
				_ = lock.Close()
			}, nil
		}
		if err := ghclient.SleepCtx(ctx, lockRetry); err != nil {
			_ = lock.Close()
			return nil, fmt.Errorf("lock the GitHub App sign in: %w", err)
		}
	}
}
