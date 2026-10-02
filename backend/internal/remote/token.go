package remote

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/deividfortuna/babysitter/internal/runfile"
)

const TokenFileName = "remote-token"

const tokenBytes = 32

func TokenPath(dataDir string) string {
	return filepath.Join(dataDir, TokenFileName)
}

func Token(dataDir string) (string, error) {
	path := TokenPath(dataDir)
	existing, err := os.ReadFile(path)
	if err == nil {
		if token := strings.TrimSpace(string(existing)); token != "" {
			return token, nil
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("read remote token: %w", err)
	}
	return RotateToken(dataDir)
}

func CurrentToken(dataDir string) string {
	var data []byte
	err := runfile.WhileBusy(func() (err error) {
		data, err = os.ReadFile(TokenPath(dataDir))
		return err
	})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func RotateToken(dataDir string) (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("make remote token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return "", fmt.Errorf("create data dir: %w", err)
	}
	if err := replaceFile(TokenPath(dataDir), []byte(token+"\n")); err != nil {
		return "", fmt.Errorf("write remote token: %w", err)
	}
	return token, nil
}

func replaceFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return runfile.WhileBusy(func() error { return os.Rename(tmp.Name(), path) })
}
