package whisperx

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"audio2note/internal/transcriber"
)

// --- buildArgs ---

func TestBuildArgs_Defaults(t *testing.T) {
	in := transcriber.Input{
		AudioFile:  "/path/to/meeting.mp3",
		OutputFile: "/tmp/out/Transcript.json",
	}
	got := buildArgs(in, "/tmp/out")

	if got[0] != "/path/to/meeting.mp3" {
		t.Errorf("audio file should be first positional, got %q", got[0])
	}
	for _, want := range []string{
		"--model", "small",
		"--device", "cuda",
		"--output_format", "json",
		"--diarize",
		"--output_dir", "/tmp/out",
	} {
		if !containsArg(got, want) {
			t.Errorf("missing %q in args: %v", want, got)
		}
	}
}

func TestBuildArgs_NoDefaults_DropsAllExceptOutputDir(t *testing.T) {
	in := transcriber.Input{
		AudioFile:  "/audio.mp3",
		OutputFile: "/tmp/Transcript.json",
		NoDefaults: true,
	}
	got := buildArgs(in, "/tmp")
	for _, banned := range []string{"--model", "--device", "--output_format", "--diarize"} {
		if containsArg(got, banned) {
			t.Errorf("NoDefaults should drop %q; got %v", banned, got)
		}
	}
	if !containsArg(got, "--output_dir") {
		t.Errorf("--output_dir should always be present; got %v", got)
	}
}

func TestBuildArgs_ExtraArgsAppendedLast(t *testing.T) {
	in := transcriber.Input{
		AudioFile:  "/audio.mp3",
		OutputFile: "/tmp/Transcript.json",
		ExtraArgs:  []string{"--model", "large-v3", "--language", "en"},
	}
	got := buildArgs(in, "/tmp")
	wantSuffix := []string{"--model", "large-v3", "--language", "en"}
	if !reflect.DeepEqual(got[len(got)-len(wantSuffix):], wantSuffix) {
		t.Errorf("ExtraArgs should be the final args; got tail %v, want %v",
			got[len(got)-len(wantSuffix):], wantSuffix)
	}
	// And the default --model small is still earlier in the args (whisperx
	// will take the last value).
	idxDefault := indexOfPair(got, "--model", "small")
	idxOverride := indexOfPair(got, "--model", "large-v3")
	if idxDefault < 0 || idxOverride < 0 {
		t.Fatalf("expected both --model values present; got %v", got)
	}
	if idxOverride < idxDefault {
		t.Errorf("override should come AFTER default; defaultIdx=%d overrideIdx=%d", idxDefault, idxOverride)
	}
}

// --- renameStemFile ---

func TestRenameStemFile_RenamesWhenStemDiffers(t *testing.T) {
	dir := t.TempDir()
	expected := filepath.Join(dir, "meeting.json")
	if err := os.WriteFile(expected, []byte("transcript"), 0o600); err != nil {
		t.Fatal(err)
	}
	requested := filepath.Join(dir, "Transcript.json")

	if err := renameStemFile("/anywhere/meeting.mp3", requested, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(expected); !os.IsNotExist(err) {
		t.Errorf("expected file should be gone; stat err = %v", err)
	}
	data, err := os.ReadFile(requested)
	if err != nil {
		t.Fatalf("requested file missing: %v", err)
	}
	if string(data) != "transcript" {
		t.Errorf("contents = %q, want %q", data, "transcript")
	}
}

func TestRenameStemFile_NoopWhenStemMatches(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "meeting.json")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := renameStemFile("/anywhere/meeting.mp3", p, dir); err != nil {
		t.Errorf("noop case should not error; got %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("file should still exist; got err %v", err)
	}
}

func TestRenameStemFile_NoopWhenExpectedMissing(t *testing.T) {
	dir := t.TempDir()
	requested := filepath.Join(dir, "Transcript.json")
	// no expected file on disk — user likely used --output_format srt etc.
	if err := renameStemFile("/anywhere/meeting.mp3", requested, dir); err != nil {
		t.Errorf("missing-expected case should not error; got %v", err)
	}
}

// --- checkHuggingFaceAuth ---

func TestCheckHuggingFaceAuth_EnvVar(t *testing.T) {
	t.Setenv("HF_TOKEN", "abc123")
	t.Setenv("HOME", t.TempDir()) // ensure no file-based token interferes
	if err := checkHuggingFaceAuth(); err != nil {
		t.Errorf("HF_TOKEN should satisfy auth; got %v", err)
	}
}

func TestCheckHuggingFaceAuth_TokenFile(t *testing.T) {
	t.Setenv("HF_TOKEN", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	tokenPath := filepath.Join(home, ".cache", "huggingface", "token")
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenPath, []byte("hf_realtoken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkHuggingFaceAuth(); err != nil {
		t.Errorf("token file should satisfy auth; got %v", err)
	}
}

func TestCheckHuggingFaceAuth_Missing(t *testing.T) {
	t.Setenv("HF_TOKEN", "")
	t.Setenv("HOME", t.TempDir())
	err := checkHuggingFaceAuth()
	if err == nil {
		t.Fatal("expected error when no HF auth is present")
	}
	if !strings.Contains(err.Error(), "hf auth login") {
		t.Errorf("error should suggest `hf auth login`; got %q", err.Error())
	}
}

func TestCheckHuggingFaceAuth_EmptyTokenFileFails(t *testing.T) {
	t.Setenv("HF_TOKEN", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	tokenPath := filepath.Join(home, ".cache", "huggingface", "token")
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenPath, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkHuggingFaceAuth(); err == nil {
		t.Errorf("empty token file should not satisfy auth")
	}
}

// --- helpers ---

func containsArg(args []string, target string) bool {
	for _, a := range args {
		if a == target {
			return true
		}
	}
	return false
}

// indexOfPair finds the index of `flag` in args where args[idx+1] == value;
// returns -1 if no such occurrence.
func indexOfPair(args []string, flag, value string) int {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return i
		}
	}
	return -1
}
