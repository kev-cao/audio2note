// Package claudecode adapts the Claude Code CLI (`claude`) to the Summarizer
// interface.
package claudecode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"audio2note/internal/summarizer"
)

type ClaudeCode struct{}

func (ClaudeCode) Name() string { return "claudecode" }

func (ClaudeCode) Summarize(ctx context.Context, in summarizer.Input) error {
	prompt := buildPrompt(in)
	args := []string{"-p", prompt}
	if !in.NoDefaults {
		// --permission-mode acceptEdits suppresses interactive prompts.
		// --add-dir extends the sandbox's allowed write set; without it
		// claude can't write to paths outside cwd (e.g. an Obsidian vault
		// elsewhere on disk, or a transcript file the user pointed at).
		args = append(args, "--permission-mode", "acceptEdits")
		if dir := filepath.Dir(in.TranscriptFile); dir != "" {
			args = append(args, "--add-dir", dir)
		}
		if in.ObsidianVault != "" {
			args = append(args, "--add-dir", in.ObsidianVault)
		}
	}
	args = append(args, in.ExtraArgs...)
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func buildPrompt(in summarizer.Input) string {
	var b strings.Builder
	fmt.Fprintf(&b,
		"Read through the meeting transcript written at %s and make notes from this transcript.",
		in.TranscriptFile,
	)
	if in.OutputFile != "" {
		fmt.Fprintf(&b, " The output file for this transcript should be named %s.", in.OutputFile)
	}
	if in.ObsidianVault != "" {
		fmt.Fprintf(&b, "\n\nWrite the meeting summary to my Obsidian vault located at %s.", in.ObsidianVault)
	}
	if len(in.Highlights) > 0 {
		b.WriteString("\n\nThe user marked the following moments in this recording as highlights:")
		for _, h := range in.Highlights {
			fmt.Fprintf(&b, "\n  - %s", formatHighlight(h))
		}
		b.WriteString("\n\nPay special attention to the transcript content around these timestamps; treat them as the most important moments in the discussion.")
	}
	if in.ExtraPrompt != "" {
		fmt.Fprintf(&b, "\n\n%s", in.ExtraPrompt)
	}
	return b.String()
}

// formatHighlight renders a duration as HH:MM:SS (zero-padded).
func formatHighlight(d time.Duration) string {
	total := int(d.Round(time.Second).Seconds())
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func init() {
	summarizer.Register(ClaudeCode{})
}
