package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAndLoadRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	want := Config{
		DefaultTranscriber: "whisperx",
		DefaultSummarizer:  "opencode",
		BasePrompt:         "Summarize {{transcript}} tersely.",
		ObsidianVault:      "/vaults/work",
	}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestLoadMissingFileIsBenign(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := Load()
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if (got != Config{}) {
		t.Fatalf("missing file should return zero Config, got %+v", got)
	}
}

func TestEffectiveMissingFileReturnsDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := Effective()
	if err != nil {
		t.Fatal(err)
	}
	if got != Defaults() {
		t.Fatalf("missing file should yield Defaults()\n got %+v\nwant %+v", got, Defaults())
	}
}

func TestEffectiveOverlaysFileOverDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Only set two keys; the rest must fall back to defaults.
	if err := Save(Config{DefaultTranscriber: "whisperx", ObsidianVault: "/v"}); err != nil {
		t.Fatal(err)
	}
	got, err := Effective()
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultTranscriber != "whisperx" {
		t.Errorf("file key should win: DefaultTranscriber = %q, want %q", got.DefaultTranscriber, "whisperx")
	}
	if got.ObsidianVault != "/v" {
		t.Errorf("file key should win: ObsidianVault = %q, want %q", got.ObsidianVault, "/v")
	}
	if got.DefaultSummarizer != DefaultSummarizer {
		t.Errorf("unset key should fall back: DefaultSummarizer = %q, want %q", got.DefaultSummarizer, DefaultSummarizer)
	}
	if got.BasePrompt != DefaultBasePrompt {
		t.Errorf("unset key should fall back: BasePrompt = %q, want %q", got.BasePrompt, DefaultBasePrompt)
	}
}

func TestLoadMalformedJSONErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, configDir, configFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestSavePerms(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := Save(Config{DefaultTranscriber: "whisperx"}); err != nil {
		t.Fatal(err)
	}
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("file perm = %o, want 600", perm)
	}
	di, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if perm := di.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir perm = %o, want 700", perm)
	}
}

func TestSaveOmitsEmptyFields(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := Save(Config{DefaultTranscriber: "whisperx"}); err != nil {
		t.Fatal(err)
	}
	path, _ := Path()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// omitempty means the untouched keys must not be serialized.
	for _, key := range []string{"defaultSummarizer", "basePrompt", "obsidianVault"} {
		if strings.Contains(string(data), `"`+key+`"`) {
			t.Errorf("unset key %q should be omitted from file, got:\n%s", key, data)
		}
	}
}
