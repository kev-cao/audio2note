package summarizer

import (
	"strings"
	"testing"
	"time"
)

func TestBuildBasePrompt_Minimal(t *testing.T) {
	got := BuildBasePrompt(Input{
		TranscriptFile: "/abs/transcript.txt",
		OutputFile:     "notes.md",
	})
	want := "Read through the meeting transcript written at /abs/transcript.txt and make notes from this transcript. The output file for this transcript should be named notes.md."
	if got != want {
		t.Errorf("BuildBasePrompt minimal mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestBuildBasePrompt_WithVault(t *testing.T) {
	got := BuildBasePrompt(Input{
		TranscriptFile: "/abs/transcript.txt",
		OutputFile:     "notes.md",
		ObsidianVault:  "/path/to/vault",
	})
	if !strings.Contains(got, "Write the meeting summary to my Obsidian vault located at /path/to/vault.") {
		t.Errorf("vault sentence missing\nfull prompt: %q", got)
	}
}

func TestBuildBasePrompt_WithExtraPrompt(t *testing.T) {
	got := BuildBasePrompt(Input{
		TranscriptFile: "/abs/transcript.txt",
		OutputFile:     "notes.md",
		ExtraPrompt:    "Use bullet points.",
	})
	if !strings.HasSuffix(got, "Use bullet points.") {
		t.Errorf("extra prompt should be appended at the end\nfull prompt: %q", got)
	}
}

func TestBuildBasePrompt_VaultAndExtraOrdering(t *testing.T) {
	got := BuildBasePrompt(Input{
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

func TestBuildBasePrompt_OmitsEmptyOptionals(t *testing.T) {
	got := BuildBasePrompt(Input{
		TranscriptFile: "/abs/transcript.txt",
		OutputFile:     "notes.md",
	})
	if strings.Contains(got, "Obsidian") {
		t.Error("vault sentence should be omitted when ObsidianVault is empty")
	}
	if strings.Contains(got, "\n\n") {
		t.Errorf("unexpected double newline in minimal prompt: %q", got)
	}
}

func TestBuildBasePrompt_OmitsOutputFileWhenEmpty(t *testing.T) {
	got := BuildBasePrompt(Input{
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

func TestBuildBasePrompt_OmitsOutputFileButKeepsVault(t *testing.T) {
	got := BuildBasePrompt(Input{
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

func TestBuildBasePrompt_WithHighlights(t *testing.T) {
	got := BuildBasePrompt(Input{
		TranscriptFile: "/abs/transcript.txt",
		Highlights: []time.Duration{
			5*time.Minute + 23*time.Second,
			27 * time.Minute,
		},
	})
	for _, frag := range []string{
		"user marked the following moments",
		"- 00:05:23",
		"- 00:27:00",
		"Pay special attention",
	} {
		if !strings.Contains(got, frag) {
			t.Errorf("missing fragment %q in prompt:\n%s", frag, got)
		}
	}
}

func TestBuildBasePrompt_OmitsHighlightsWhenEmpty(t *testing.T) {
	got := BuildBasePrompt(Input{
		TranscriptFile: "/abs/transcript.txt",
	})
	if strings.Contains(got, "highlights") || strings.Contains(got, "Highlights") {
		t.Errorf("highlight paragraph should be omitted when Highlights is empty\nprompt: %q", got)
	}
}

func TestBuildBasePrompt_HighlightsBetweenVaultAndExtra(t *testing.T) {
	got := BuildBasePrompt(Input{
		TranscriptFile: "/abs/transcript.txt",
		ObsidianVault:  "/path/to/vault",
		Highlights:     []time.Duration{1 * time.Minute},
		ExtraPrompt:    "Use bullet points.",
	})
	vaultIdx := strings.Index(got, "Obsidian vault")
	highlightIdx := strings.Index(got, "user marked the following moments")
	extraIdx := strings.Index(got, "Use bullet points.")
	if vaultIdx < 0 || highlightIdx < 0 || extraIdx < 0 {
		t.Fatalf("missing fragments\nprompt: %q", got)
	}
	if !(vaultIdx < highlightIdx && highlightIdx < extraIdx) {
		t.Errorf("ordering should be vault → highlights → extra; got vault=%d highlight=%d extra=%d", vaultIdx, highlightIdx, extraIdx)
	}
}

func TestBuildBasePrompt_RequestTitleAddsGuidance(t *testing.T) {
	got := BuildBasePrompt(Input{
		TranscriptFile: "/abs/transcript.txt",
		RequestTitle:   true,
	})
	if !strings.Contains(got, "short, descriptive title") {
		t.Errorf("title guidance missing when RequestTitle=true; prompt: %q", got)
	}
}

func TestBuildBasePrompt_NoTitleGuidanceByDefault(t *testing.T) {
	got := BuildBasePrompt(Input{
		TranscriptFile: "/abs/transcript.txt",
	})
	if strings.Contains(got, "title") {
		t.Errorf("title guidance should be omitted when RequestTitle=false; prompt: %q", got)
	}
}

func TestBuildBasePrompt_TitleBeforeExtraPrompt(t *testing.T) {
	got := BuildBasePrompt(Input{
		TranscriptFile: "/abs/transcript.txt",
		RequestTitle:   true,
		ExtraPrompt:    "Use bullet points.",
	})
	titleIdx := strings.Index(got, "short, descriptive title")
	extraIdx := strings.Index(got, "Use bullet points.")
	if titleIdx < 0 || extraIdx < 0 {
		t.Fatalf("missing fragments\nprompt: %q", got)
	}
	if titleIdx > extraIdx {
		t.Error("title guidance should appear before the user's extra prompt")
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
		{2*time.Hour + 30*time.Minute, "02:30:00"},
		{(961640) * time.Millisecond, "00:16:02"},
	}
	for _, tc := range cases {
		if got := FormatHighlight(tc.in); got != tc.want {
			t.Errorf("FormatHighlight(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
