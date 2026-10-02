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

func RotateToken(dataDir string) (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("make remote token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return "", fmt.Errorf("create data dir: %w", err)
	}
	if err := os.WriteFile(TokenPath(dataDir), []byte(token+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write remote token: %w", err)
	}
	return token, nil
}
