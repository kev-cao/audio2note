package plaud

import (
	"errors"
	"os"
	"path/filepath"
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

func TestSaveToken_FilePerms(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := SaveToken("abc"); err != nil {
		t.Fatal(err)
	}
	path, _ := TokenPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file perms = %o, want 0600", perm)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir perms = %o, want 0700", perm)
	}
}

func TestLoadToken_Missing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := LoadToken()
	if !errors.Is(err, ErrMissing) {
		t.Errorf("want ErrMissing, got %v", err)
	}
}

func TestLoadToken_EmptyFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".plaud", "token")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("   \n"), 0o600); err != nil {
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
