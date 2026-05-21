package cmd

import (
	"reflect"
	"strings"
	"testing"
)

func TestResolveVault(t *testing.T) {
	tests := []struct {
		name         string
		f            flags
		vaultFlagSet bool
		env          string // value to set $OBSIDIAN_VAULT_PATH to ("" means unset/empty)
		want         string
		wantErr      string // substring expected in error message (empty = no error)
	}{
		{
			name: "neither flag",
			want: "",
		},
		{
			name: "obsidian flag with env",
			f:    flags{obsidian: true},
			env:  "/from/env",
			want: "/from/env",
		},
		{
			name:    "obsidian flag without env",
			f:       flags{obsidian: true},
			wantErr: "OBSIDIAN_VAULT_PATH",
		},
		{
			name:         "vault flag wins over env",
			f:            flags{obsidianVault: "/from/flag"},
			vaultFlagSet: true,
			env:          "/from/env",
			want:         "/from/flag",
		},
		{
			name:         "vault flag without env",
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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OBSIDIAN_VAULT_PATH", tc.env)
			got, err := resolveVault(tc.f, tc.vaultFlagSet)
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
