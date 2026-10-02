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
)

const (
	signInFileName = "github-app.json"
	lockStale      = 2 * time.Minute
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
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (f credentialsFile) Remove() error {
	err := os.Remove(f.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove the GitHub App sign in: %w", err)
	}
	return nil
}

func (f credentialsFile) Lock(ctx context.Context) (unlock func(), err error) {
	path := f.path + ".lock"
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("lock the GitHub App sign in: %w", err)
	}
	for {
		lock, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = lock.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("lock the GitHub App sign in: %w", err)
		}
		removeStale(path)
		if err := ghclient.SleepCtx(ctx, lockRetry); err != nil {
			return nil, fmt.Errorf("lock the GitHub App sign in: %w", err)
		}
	}
}

func removeStale(path string) {
	info, err := os.Stat(path)
	if err == nil && time.Since(info.ModTime()) > lockStale {
		_ = os.Remove(path)
	}
}
