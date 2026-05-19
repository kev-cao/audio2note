// Package claudecode adapts the Claude Code CLI (`claude`) to the Summarizer
// interface.
package claudecode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"audio2note/internal/summarizer"
)

type ClaudeCode struct{}

func (ClaudeCode) Name() string { return "claudecode" }

func (ClaudeCode) Summarize(ctx context.Context, in summarizer.Input) error {
	prompt := buildPrompt(in)
	// --permission-mode acceptEdits so claude can write the notes file
	// without prompting in this headless invocation.
	cmd := exec.CommandContext(ctx, "claude", "-p", prompt, "--permission-mode", "acceptEdits")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func buildPrompt(in summarizer.Input) string {
	var b strings.Builder
	fmt.Fprintf(&b,
		"Read through the meeting transcript written at %s and make notes from this transcript. The output file for this transcript should be named %s.",
		in.TranscriptFile, in.OutputFile,
	)
	if in.ObsidianVault != "" {
		fmt.Fprintf(&b, "\n\nWrite the meeting summary to my Obsidian vault located at %s.", in.ObsidianVault)
	}
	return b.String()
}

func init() {
	summarizer.Register(ClaudeCode{})
}
