package cmd

import (
	"audio2note/internal/config"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveVault(t *testing.T) {
	tests := []struct {
		name         string
		f            flags
		cfgVault     string // config.obsidianVault
		vaultFlagSet bool
		want         string
		wantErr      string // substring expected in error message (empty = no error)
	}{
		{
			name: "neither flag, no config",
			want: "",
		},
		{
			name:     "obsidian flag with config vault",
			f:        flags{obsidian: true},
			cfgVault: "/from/config",
			want:     "/from/config",
		},
		{
			name:    "obsidian flag without config vault errors",
			f:       flags{obsidian: true},
			wantErr: "obsidianVault",
		},
		{
			name:         "vault flag wins over config",
			f:            flags{obsidianVault: "/from/flag"},
			vaultFlagSet: true,
			cfgVault:     "/from/config",
			want:         "/from/flag",
		},
		{
			name:         "vault flag without config",
			f:            flags{obsidianVault: "/only/from/flag"},
			vaultFlagSet: true,
			want:         "/only/from/flag",
		},
		{
			name:         "vault flag empty errors",
			f:            flags{},
			vaultFlagSet: true,
			wantErr:      "non-empty path",
		},
		{
			name:     "config vault used when no flags",
			cfgVault: "/from/config",
			want:     "/from/config",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveVault(tc.f, config.Config{ObsidianVault: tc.cfgVault}, tc.vaultFlagSet)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil (returned %q)", tc.wantErr, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveAudioFiles(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mp3")
	b := filepath.Join(dir, "b.mp3")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("fake"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("summarize-only with no args returns empty", func(t *testing.T) {
		got, err := resolveAudioFiles(context.Background(), nil, flags{summarizeOnly: true})
		if err != nil || got != "" {
			t.Errorf("got %q, %v; want \"\", nil", got, err)
		}
	})

	t.Run("summarize-only with args errors", func(t *testing.T) {
		_, err := resolveAudioFiles(context.Background(), []string{a}, flags{summarizeOnly: true})
		if err == nil || !strings.Contains(err.Error(), "--summarize-only") {
			t.Errorf("want --summarize-only error; got %v", err)
		}
	})

	t.Run("no args requires file", func(t *testing.T) {
		_, err := resolveAudioFiles(context.Background(), nil, flags{})
		if err == nil || !strings.Contains(err.Error(), "required") {
			t.Errorf("want 'required' error; got %v", err)
		}
	})

	t.Run("single arg returns absolute path unchanged", func(t *testing.T) {
		got, err := resolveAudioFiles(context.Background(), []string{a}, flags{})
		if err != nil {
			t.Fatal(err)
		}
		if got != a {
			t.Errorf("got %q, want %q", got, a)
		}
	})

	t.Run("missing file errors", func(t *testing.T) {
		_, err := resolveAudioFiles(context.Background(), []string{filepath.Join(dir, "nope.mp3")}, flags{})
		if err == nil {
			t.Errorf("want error for missing file")
		}
	})
}

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{name: "empty", input: "", want: nil},
		{name: "single", input: "--stream", want: []string{"--stream"}},
		{name: "multiple", input: "--model whisper-large --stream", want: []string{"--model", "whisper-large", "--stream"}},
		{name: "quoted value with spaces", input: "--model 'whisper large v3'", want: []string{"--model", "whisper large v3"}},
		{name: "double-quoted", input: `--name "John Doe"`, want: []string{"--name", "John Doe"}},
		{name: "mismatched quote errors", input: `--foo 'unterminated`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := splitArgs(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
