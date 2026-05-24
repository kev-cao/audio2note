package opencode

import (
	"strings"
	"testing"
	"time"

	"audio2note/internal/summarizer"
)

// The Summarize method shells out to a real `opencode` binary, so we don't
// exercise it here. The prompt builder is pure and worth testing.

func TestBuildPrompt_Minimal(t *testing.T) {
	got := buildPrompt(summarizer.Input{
		TranscriptFile: "/abs/transcript.txt",
	})
	want := "Read through the meeting transcript written at /abs/transcript.txt and make notes from this transcript."
	if got != want {
		t.Errorf("buildPrompt minimal mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestBuildPrompt_WithVaultAndOutputFile(t *testing.T) {
	got := buildPrompt(summarizer.Input{
		TranscriptFile: "/abs/transcript.txt",
		OutputFile:     "notes.md",
		ObsidianVault:  "/path/to/vault",
	})
	for _, frag := range []string{
		"output file for this transcript should be named notes.md",
		"Obsidian vault located at /path/to/vault",
	} {
		if !strings.Contains(got, frag) {
			t.Errorf("missing fragment %q in prompt:\n%s", frag, got)
		}
	}
}

func TestBuildPrompt_WithHighlights(t *testing.T) {
	got := buildPrompt(summarizer.Input{
		TranscriptFile: "/abs/transcript.txt",
		Highlights: []time.Duration{
			5*time.Minute + 23*time.Second,
			1*time.Hour + 2*time.Minute,
		},
	})
	for _, frag := range []string{
		"user marked the following moments",
		"- 00:05:23",
		"- 01:02:00",
		"Pay special attention",
	} {
		if !strings.Contains(got, frag) {
			t.Errorf("missing fragment %q in prompt:\n%s", frag, got)
		}
	}
}

func TestBuildPrompt_OmitsEmptyOptionals(t *testing.T) {
	got := buildPrompt(summarizer.Input{
		TranscriptFile: "/abs/transcript.txt",
	})
	if strings.Contains(got, "output file") {
		t.Errorf("output-file sentence should be omitted")
	}
	if strings.Contains(got, "Obsidian") {
		t.Errorf("vault sentence should be omitted")
	}
	if strings.Contains(got, "highlights") || strings.Contains(got, "Highlights") {
		t.Errorf("highlights paragraph should be omitted")
	}
}

func TestFormatHighlight(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "00:00:00"},
		{5*time.Minute + 23*time.Second, "00:05:23"},
		{1*time.Hour + 2*time.Minute + 3*time.Second, "01:02:03"},
		{(961640) * time.Millisecond, "00:16:02"},
	}
	for _, tc := range cases {
		if got := formatHighlight(tc.in); got != tc.want {
			t.Errorf("formatHighlight(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
