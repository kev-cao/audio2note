// Package opencode adapts the OpenCode CLI (`opencode`) to the Summarizer
// interface.
//
// `opencode run <prompt>` is the non-interactive entrypoint: the prompt is a
// trailing positional argument (no stdin support per the docs). We pass
// --dangerously-skip-permissions so opencode can write the notes file
// without prompting; this is opencode's analog of claude's
// --permission-mode acceptEdits and is the only documented permission flag.
package opencode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"audio2note/internal/summarizer"
)

type OpenCode struct{}

func (OpenCode) Name() string { return "opencode" }

func (OpenCode) Summarize(ctx context.Context, in summarizer.Input) error {
	prompt := buildPrompt(in)
	args := []string{"run"}
	if !in.NoDefaults {
		args = append(args, "--dangerously-skip-permissions")
	}
	args = append(args, in.ExtraArgs...)
	// Prompt is the trailing positional.
	args = append(args, prompt)
	cmd := exec.CommandContext(ctx, "opencode", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// buildPrompt mirrors the claudecode adapter's prompt builder. If a third
// AI-CLI adapter lands, factor this (and formatHighlight) into a shared
// helper in the summarizer package.
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
	summarizer.Register(OpenCode{})
}
