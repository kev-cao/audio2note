package cmd

import (
	"audio2note/internal/config"
	"testing"

	"github.com/spf13/cobra"
)

// TestSettingsRoundTripCoverEveryConfigField guards against adding a Config
// field without a matching setting entry: every non-empty value set through the
// settings table must survive a get.
func TestSettingsRoundTrip(t *testing.T) {
	for _, s := range settings {
		var c config.Config
		s.set(&c, "x-value")
		if got := s.get(c); got != "x-value" {
			t.Errorf("setting %q: get after set = %q, want %q", s.key, got, "x-value")
		}
	}
}

func TestRestrictedSettingsHaveOptions(t *testing.T) {
	for _, key := range []string{"defaultTranscriber", "defaultSummarizer"} {
		s, ok := findSetting(key)
		if !ok {
			t.Fatalf("missing setting %q", key)
		}
		if s.options == nil {
			t.Errorf("setting %q should be restricted (options != nil)", key)
		} else if len(s.options()) == 0 {
			t.Errorf("setting %q options() returned no values", key)
		}
	}
}

func TestFindSettingUnknown(t *testing.T) {
	if _, ok := findSetting("nope"); ok {
		t.Error("findSetting should report false for an unknown key")
	}
}

// newFlagsCmd builds a throwaway command carrying the shared pipeline flags so
// applyConfig's cmd.Flags().Changed() checks work, mirroring how run/plaud-run
// are constructed.
func newFlagsCmd(f *flags) *cobra.Command {
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
	addPipelineFlags(cmd, f)
	return cmd
}

func TestApplyConfig_ConfigFillsUnsetFlags(t *testing.T) {
	var f flags
	cmd := newFlagsCmd(&f)
	// No flags passed, so nothing is "Changed"; config should win over defaults.
	cfg := config.Config{
		DefaultTranscriber: "whisperx",
		DefaultSummarizer:  "opencode",
		BasePrompt:         "custom {{transcript}}",
	}
	if err := applyConfig(cmd, &f, cfg); err != nil {
		t.Fatal(err)
	}
	if f.transcriber != "whisperx" {
		t.Errorf("transcriber = %q, want whisperx", f.transcriber)
	}
	if f.summarizer != "opencode" {
		t.Errorf("summarizer = %q, want opencode", f.summarizer)
	}
	if f.basePrompt != "custom {{transcript}}" {
		t.Errorf("basePrompt = %q, want custom {{transcript}}", f.basePrompt)
	}
}

func TestApplyConfig_ExplicitFlagBeatsConfig(t *testing.T) {
	var f flags
	cmd := newFlagsCmd(&f)
	if err := cmd.Flags().Set("transcriber", "macwhisper"); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{DefaultTranscriber: "whisperx"}
	if err := applyConfig(cmd, &f, cfg); err != nil {
		t.Fatal(err)
	}
	if f.transcriber != "macwhisper" {
		t.Errorf("explicit flag should win: transcriber = %q, want macwhisper", f.transcriber)
	}
}

func TestApplyConfig_InvalidConfigTranscriberErrors(t *testing.T) {
	var f flags
	cmd := newFlagsCmd(&f)
	cfg := config.Config{DefaultTranscriber: "bogus"}
	if err := applyConfig(cmd, &f, cfg); err == nil {
		t.Fatal("expected error for invalid config defaultTranscriber, got nil")
	}
}
