// Package config handles the user-level configuration file at
// ~/.audio2note/config.json. It mirrors internal/plaud/token.go's dotdir
// skeleton (home-dir lookup, 0700 dir / 0600 file) but is JSON and treats a
// missing file as benign: callers fall back to Defaults() rather than erroring.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	configDir  = ".audio2note"
	configFile = "config.json"
)

// Centralized default values — the single source of truth for every baseline.
// A missing config file resolves to exactly these (see Defaults / Effective).
// The cmd layer passes DefaultTranscriber / DefaultSummarizer as the cobra flag
// defaults, and the summarizer uses DefaultBasePrompt as its fallback, so no
// literal is duplicated across the codebase.
const (
	DefaultTranscriber = "macwhisper"
	DefaultSummarizer  = "claudecode"
	// TranscriptPlaceholder is the token in a base prompt that the summarizer
	// replaces with the transcript's path when building the final prompt.
	TranscriptPlaceholder = "{{transcript}}"
	// DefaultBasePrompt is the generic base instruction handed to the
	// summarizer. The TranscriptPlaceholder is replaced with the transcript's
	// path when the prompt is built (see summarizer.BuildBasePrompt).
	DefaultBasePrompt = "Read through the transcript located at " + TranscriptPlaceholder + " and generate a summary."
)

// Config is the user-level configuration. Every field is optional; an omitted
// field falls back to its default (see Defaults / Effective). Zero-value fields
// are omitted on write so config.json only ever contains keys the user set.
type Config struct {
	DefaultTranscriber string `json:"defaultTranscriber,omitempty"`
	DefaultSummarizer  string `json:"defaultSummarizer,omitempty"`
	BasePrompt         string `json:"basePrompt,omitempty"`
	ObsidianVault      string `json:"obsidianVault,omitempty"`
	// PlaudToken is the Plaud bearer token. It's managed through
	// `audio2note plaud auth setup`, not `config set`, and is shown masked by
	// `config get`.
	PlaudToken string `json:"plaudToken,omitempty"`
}

// Defaults returns the baseline config used when no file exists (and the base
// each loaded file is overlaid onto). ObsidianVault has no default — an unset
// vault means "no vault output".
func Defaults() Config {
	return Config{
		DefaultTranscriber: DefaultTranscriber,
		DefaultSummarizer:  DefaultSummarizer,
		BasePrompt:         DefaultBasePrompt,
	}
}

// Path returns the absolute path to the config file.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}
	return filepath.Join(home, configDir, configFile), nil
}

// Load reads the raw config file contents. A missing file returns a zero Config
// with a nil error (callers merge with Defaults via Effective). Malformed JSON
// is an error. Load intentionally does NOT fold in defaults so that Save round-
// trips only the keys the user actually set.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return c, nil
}

// Effective returns Defaults() overlaid with the on-disk file: any non-empty
// field in the file wins, and every unset field keeps its default. This is the
// fully-populated view callers actually run with and that `config get` displays.
func Effective() (Config, error) {
	file, err := Load()
	if err != nil {
		return Config{}, err
	}
	return merge(Defaults(), file), nil
}

// merge overlays override's non-empty fields onto base.
func merge(base, override Config) Config {
	if override.DefaultTranscriber != "" {
		base.DefaultTranscriber = override.DefaultTranscriber
	}
	if override.DefaultSummarizer != "" {
		base.DefaultSummarizer = override.DefaultSummarizer
	}
	if override.BasePrompt != "" {
		base.BasePrompt = override.BasePrompt
	}
	if override.ObsidianVault != "" {
		base.ObsidianVault = override.ObsidianVault
	}
	if override.PlaudToken != "" {
		base.PlaudToken = override.PlaudToken
	}
	return base
}

// Save writes c to the config file as indented JSON, creating the directory if
// needed. The directory is 0700 and the file is 0600, matching the Plaud token.
func Save(c Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}
