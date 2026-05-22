package claudecode

import (
	"strings"
	"testing"

	"audio2note/internal/summarizer"
)

func TestBuildPrompt_Minimal(t *testing.T) {
	got := buildPrompt(summarizer.Input{
		TranscriptFile: "/abs/transcript.txt",
		OutputFile:     "notes.md",
	})
	want := "Read through the meeting transcript written at /abs/transcript.txt and make notes from this transcript. The output file for this transcript should be named notes.md."
	if got != want {
		t.Errorf("buildPrompt minimal mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestBuildPrompt_WithVault(t *testing.T) {
	got := buildPrompt(summarizer.Input{
		TranscriptFile: "/abs/transcript.txt",
		OutputFile:     "notes.md",
		ObsidianVault:  "/path/to/vault",
	})
	if !strings.Contains(got, "Write the meeting summary to my Obsidian vault located at /path/to/vault.") {
		t.Errorf("vault sentence missing\nfull prompt: %q", got)
	}
}

func TestBuildPrompt_WithExtraPrompt(t *testing.T) {
	got := buildPrompt(summarizer.Input{
		TranscriptFile: "/abs/transcript.txt",
		OutputFile:     "notes.md",
		ExtraPrompt:    "Use bullet points.",
	})
	if !strings.HasSuffix(got, "Use bullet points.") {
		t.Errorf("extra prompt should be appended at the end\nfull prompt: %q", got)
	}
}

func TestBuildPrompt_VaultAndExtraOrdering(t *testing.T) {
	got := buildPrompt(summarizer.Input{
		TranscriptFile: "/abs/transcript.txt",
		OutputFile:     "notes.md",
		ObsidianVault:  "/path/to/vault",
		ExtraPrompt:    "Use bullet points.",
	})
	vaultIdx := strings.Index(got, "Obsidian vault")
	extraIdx := strings.Index(got, "Use bullet points.")
	if vaultIdx < 0 || extraIdx < 0 {
		t.Fatalf("missing fragments\nprompt: %q", got)
	}
	if vaultIdx > extraIdx {
		t.Error("vault sentence should appear before the user's extra prompt")
	}
	if !strings.HasSuffix(got, "Use bullet points.") {
		t.Error("extra prompt should be last")
	}
}

func TestBuildPrompt_OmitsEmptyOptionals(t *testing.T) {
	got := buildPrompt(summarizer.Input{
		TranscriptFile: "/abs/transcript.txt",
		OutputFile:     "notes.md",
	})
	if strings.Contains(got, "Obsidian") {
		t.Error("vault sentence should be omitted when ObsidianVault is empty")
	}
	if strings.Contains(got, "\n\n") {
		// The first paragraph alone shouldn't contain a double newline either.
		t.Errorf("unexpected double newline in minimal prompt: %q", got)
	}
}

func TestBuildPrompt_OmitsOutputFileWhenEmpty(t *testing.T) {
	got := buildPrompt(summarizer.Input{
		TranscriptFile: "/abs/transcript.txt",
	})
	if strings.Contains(got, "output file") {
		t.Errorf("output-file sentence should be omitted when OutputFile is empty\nprompt: %q", got)
	}
	want := "Read through the meeting transcript written at /abs/transcript.txt and make notes from this transcript."
	if got != want {
		t.Errorf("unexpected prompt\n got: %q\nwant: %q", got, want)
	}
}

func TestBuildPrompt_OmitsOutputFileButKeepsVault(t *testing.T) {
	got := buildPrompt(summarizer.Input{
		TranscriptFile: "/abs/transcript.txt",
		ObsidianVault:  "/path/to/vault",
	})
	if strings.Contains(got, "output file") {
		t.Errorf("output-file sentence should be omitted; prompt: %q", got)
	}
	if !strings.Contains(got, "Obsidian vault located at /path/to/vault") {
		t.Errorf("vault sentence missing; prompt: %q", got)
	}
}
