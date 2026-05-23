// Package plaud handles authentication and (eventually) API access for the
// Plaud transcription service.
package plaud

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	tokenDir  = ".plaud"
	tokenFile = "token"
)

// ErrMissing means the token file does not exist, or exists but is empty.
var ErrMissing = errors.New("missing bearer token")

// TokenPath returns the absolute path to the stored token file.
func TokenPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}
	return filepath.Join(home, tokenDir, tokenFile), nil
}

// SaveToken writes the (whitespace-trimmed) token to ~/.plaud/token, creating
// the directory if needed. The directory is 0700 and the file is 0600.
func SaveToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("token is empty")
	}
	path, err := TokenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create token directory: %w", err)
	}
	return os.WriteFile(path, []byte(token), 0o600)
}

// LoadToken reads the stored token. Returns ErrMissing if the file doesn't
// exist or its trimmed contents are empty.
func LoadToken() (string, error) {
	path, err := TokenPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrMissing
		}
		return "", err
	}
	t := strings.TrimSpace(string(data))
	if t == "" {
		return "", ErrMissing
	}
	return t, nil
}
