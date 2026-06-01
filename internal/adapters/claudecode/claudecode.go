// Package claudecode adapts the Claude Code CLI (`claude`) to the Summarizer
// interface.
package claudecode

import (
	"audio2note/internal/summarizer"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// titleSchema is the JSON Schema passed via --json-schema when the caller
// wants a title back. The schema is what makes Claude emit a final result
// event whose `structured_output` field contains `{"title": "..."}`.
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
		// stream-json keeps the user's live progress feedback intact (we
		// re-emit the assistant's text content as it arrives); --json-schema
		// constrains the agent's final structured output so we can pull a
		// title out of the result event. --verbose is required by claude
		// when combining -p with stream-json.
		args = append(args, "--output-format", "stream-json", "--verbose", "--json-schema", titleSchema)
	}
	args = append(args, in.ExtraArgs...)
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if !in.RequestTitle {
		cmd.Stdout = os.Stdout
		return summarizer.Result{}, cmd.Run()
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return summarizer.Result{}, fmt.Errorf("claudecode: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return summarizer.Result{}, fmt.Errorf("claudecode: start: %w", err)
	}
	title := parseStreamJSON(stdout, os.Stdout)
	if err := cmd.Wait(); err != nil {
		return summarizer.Result{}, err
	}
	return summarizer.Result{Title: title}, nil
}

// parseStreamJSON reads claude's stream-json output line-by-line. Assistant
// text is mirrored to userOut so the user keeps seeing live progress; tool
// uses are summarized one-line each. The final `result` event carries the
// schema-validated structured output — that's where the title comes from. A
// missing or unparseable title is non-fatal (returns ""), matching the
// best-effort contract on summarizer.Input.RequestTitle.
func parseStreamJSON(r io.Reader, userOut io.Writer) string {
	var title string
	sc := bufio.NewScanner(r)
	// Some events (tool inputs, system prompts echoed back) can be large; a
	// 64KB default token isn't enough.
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var ev streamEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			// Unknown line shape — just echo it through so debug info isn't
			// silently swallowed.
			fmt.Fprintln(userOut, sc.Text())
			continue
		}
		switch ev.Type {
		case "assistant":
			for _, c := range ev.Message.Content {
				switch c.Type {
				case "text":
					if c.Text != "" {
						fmt.Fprintln(userOut, c.Text)
					}
				case "tool_use":
					fmt.Fprintf(userOut, "[claude] %s\n", c.Name)
				}
			}
		case "result":
			if ev.StructuredOutput.Title != "" {
				title = ev.StructuredOutput.Title
			}
		}
	}
	return title
}

// streamEvent is the trimmed-down shape of claude's stream-json events. We
// only model the fields we actually use.
type streamEvent struct {
	Type    string `json:"type"`
	Message struct {
		Content []streamContent `json:"content"`
	} `json:"message"`
	StructuredOutput struct {
		Title string `json:"title"`
	} `json:"structured_output"`
}

type streamContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
	Name string `json:"name"`
}

func init() {
	summarizer.Register(ClaudeCode{})
}
