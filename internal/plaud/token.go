// Package plaud handles authentication and API access for the Plaud
// transcription service.
package plaud

import (
	"audio2note/internal/config"
	"errors"
	"strings"
)

// ErrMissing means no bearer token is configured (the plaudToken key in
// config.json is unset or empty).
var ErrMissing = errors.New("missing bearer token")

// SaveToken writes the (whitespace-trimmed) token to the "plaudToken" key in
// config.json, preserving every other configured key.
func SaveToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("token is empty")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.PlaudToken = token
	return config.Save(cfg)
}

// LoadToken reads the bearer token from the "plaudToken" key in config.json.
// Returns ErrMissing when no token is configured.
func LoadToken() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	t := strings.TrimSpace(cfg.PlaudToken)
	if t == "" {
		return "", ErrMissing
	}
	return t, nil
}
