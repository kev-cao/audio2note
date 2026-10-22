package plaud

import (
	"audio2note/internal/config"
	"errors"
	"testing"
)

func TestSaveAndLoadToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := SaveToken("abc123"); err != nil {
		t.Fatal(err)
	}
	got, err := LoadToken()
	if err != nil {
		t.Fatal(err)
	}
	if got != "abc123" {
		t.Errorf("got %q, want %q", got, "abc123")
	}
}

func TestSaveToken_TrimsWhitespace(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := SaveToken("  abc123\n\t"); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadToken()
	if got != "abc123" {
		t.Errorf("got %q, want %q", got, "abc123")
	}
}

func TestSaveToken_RejectsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := SaveToken("   "); err == nil {
		t.Error("expected error for whitespace-only token")
	}
	if err := SaveToken(""); err == nil {
		t.Error("expected error for empty token")
	}
}

func TestLoadToken_Missing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := LoadToken()
	if !errors.Is(err, ErrMissing) {
		t.Errorf("want ErrMissing, got %v", err)
	}
}

func TestLoadToken_MissingWhenConfigHasOtherKeys(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// A config file that exists but has no plaudToken must still be ErrMissing.
	if err := config.Save(config.Config{ObsidianVault: "/v"}); err != nil {
		t.Fatal(err)
	}
	_, err := LoadToken()
	if !errors.Is(err, ErrMissing) {
		t.Errorf("want ErrMissing, got %v", err)
	}
}

func TestSaveToken_Overwrites(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := SaveToken("first"); err != nil {
		t.Fatal(err)
	}
	if err := SaveToken("second"); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadToken()
	if got != "second" {
		t.Errorf("got %q, want %q", got, "second")
	}
}

// SaveToken must preserve the user's other config keys (it's a merge, not a
// clobber).
func TestSaveToken_PreservesOtherKeys(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := config.Save(config.Config{BasePrompt: "keep me", ObsidianVault: "/v"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveToken("tok"); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.BasePrompt != "keep me" || got.ObsidianVault != "/v" {
		t.Errorf("SaveToken clobbered other keys: %+v", got)
	}
	if got.PlaudToken != "tok" {
		t.Errorf("PlaudToken = %q, want tok", got.PlaudToken)
	}
}
