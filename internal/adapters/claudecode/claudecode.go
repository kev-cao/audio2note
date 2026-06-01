// Package claudecode adapts the Claude Code CLI (`claude`) to the Summarizer
// interface.
package claudecode

import (
	"audio2note/internal/summarizer"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// titleSchema is the JSON Schema passed via --json-schema when the caller
// wants a title back. The schema is what makes Claude populate the result
// blob's `structured_output` field with `{"title": "..."}`.
const titleSchema = `{"type":"object","properties":{"title":{"type":"string"}},"required":["title"]}`

type ClaudeCode struct{}

func (ClaudeCode) Name() string { return "claudecode" }

func (ClaudeCode) Summarize(ctx context.Context, in summarizer.Input) (summarizer.Result, error) {
	prompt := summarizer.BuildBasePrompt(in)
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
	if in.RequestTitle {
		// --output-format json gives a single result blob at the end with
		// structured_output populated by --json-schema. We avoid stream-json
		// because its live event stream is mostly tool-call noise (Read /
		// Write / Bash) that isn't useful to surface.
		args = append(args, "--output-format", "json", "--json-schema", titleSchema)
	}
	args = append(args, in.ExtraArgs...)
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if !in.RequestTitle {
		cmd.Stdout = os.Stdout
		return summarizer.Result{}, cmd.Run()
	}

	fmt.Println("→ summarizing with claude…")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return summarizer.Result{}, fmt.Errorf("claudecode: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return summarizer.Result{}, fmt.Errorf("claudecode: start: %w", err)
	}
	body, readErr := io.ReadAll(stdout)
	waitErr := cmd.Wait()
	if waitErr != nil {
		return summarizer.Result{}, waitErr
	}
	if readErr != nil {
		return summarizer.Result{}, fmt.Errorf("claudecode: read stdout: %w", readErr)
	}
	text, title := parseResult(body)
	if text != "" {
		fmt.Println(text)
	}
	return summarizer.Result{Title: title}, nil
}

// parseResult unmarshals claude's --output-format json result blob and pulls
// out the final assistant text (to show the user) and the schema-validated
// title. Both are best-effort: an unparseable blob returns ("", ""), matching
// the contract on summarizer.Input.RequestTitle.
func parseResult(body []byte) (text, title string) {
	var r resultBlob
	if err := json.Unmarshal(body, &r); err != nil {
		return "", ""
	}
	return r.Result, r.StructuredOutput.Title
}

// resultBlob is the trimmed-down shape of claude's --output-format json
// result. We only model the fields we actually use.
type resultBlob struct {
	Result           string `json:"result"`
	StructuredOutput struct {
		Title string `json:"title"`
	} `json:"structured_output"`
}

func init() {
	summarizer.Register(ClaudeCode{})
}
