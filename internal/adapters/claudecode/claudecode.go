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

	"audio2note/internal/summarizer"
)

type ClaudeCode struct{}

func (ClaudeCode) Name() string { return "claudecode" }

func (ClaudeCode) Summarize(ctx context.Context, in summarizer.Input) error {
	prompt := buildPrompt(in)
	// --permission-mode acceptEdits suppresses interactive prompts.
	// --add-dir extends the sandbox's allowed write set; without it claude
	// can't write to paths outside cwd (e.g. an Obsidian vault elsewhere on
	// disk, or a transcript file the user pointed at).
	args := []string{"-p", prompt, "--permission-mode", "acceptEdits"}
	if dir := filepath.Dir(in.TranscriptFile); dir != "" {
		args = append(args, "--add-dir", dir)
	}
	if in.ObsidianVault != "" {
		args = append(args, "--add-dir", in.ObsidianVault)
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
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
