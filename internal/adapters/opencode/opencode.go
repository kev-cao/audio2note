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
	"audio2note/internal/summarizer"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type OpenCode struct{}

func (OpenCode) Name() string { return "opencode" }

func (OpenCode) Summarize(ctx context.Context, in summarizer.Input) (summarizer.Result, error) {
	prompt := summarizer.BuildBasePrompt(in)
	if in.RequestTitle {
		// opencode has no --json-schema equivalent, so we ask the model to
		// emit the title as a single JSON object on its own line and parse it
		// out of the streamed text. The anti-fence clause matters — without
		// it, the model wraps the JSON in a ```json block ~50% of the time.
		prompt += "\n\nAt the very end of your response, on its own line, output the title as a single JSON object: {\"title\":\"<your title>\"}. Do not wrap in markdown fences."
	}

	args := []string{"run"}
	if !in.NoDefaults {
		args = append(args, "--dangerously-skip-permissions")
	}
	if in.RequestTitle {
		args = append(args, "--format", "json")
	}
	args = append(args, in.ExtraArgs...)
	args = append(args, prompt)

	cmd := exec.CommandContext(ctx, "opencode", args...)
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if !in.RequestTitle {
		cmd.Stdout = os.Stdout
		return summarizer.Result{}, cmd.Run()
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return summarizer.Result{}, fmt.Errorf("opencode: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return summarizer.Result{}, fmt.Errorf("opencode: start: %w", err)
	}
	title := parseStreamJSON(stdout, os.Stdout)
	if err := cmd.Wait(); err != nil {
		return summarizer.Result{}, err
	}
	return summarizer.Result{Title: title}, nil
}

// parseStreamJSON reads opencode's --format json output line-by-line. Each
// line is one event; we mirror text events to userOut for live progress and
// scan them for the trailing {"title":"..."} JSON object. Title capture is
// best-effort: a missing/malformed object returns "" with no error.
func parseStreamJSON(r io.Reader, userOut io.Writer) string {
	var textBuf strings.Builder
	sc := bufio.NewScanner(r)
	// Text events can carry the entire model response in a single line, so
	// the 64KB default token isn't enough.
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var ev streamEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			fmt.Fprintln(userOut, sc.Text())
			continue
		}
		switch ev.Type {
		case "text":
			if ev.Part.Text != "" {
				fmt.Fprintln(userOut, ev.Part.Text)
				textBuf.WriteString(ev.Part.Text)
				textBuf.WriteByte('\n')
			}
		case "tool":
			if ev.Part.Tool != "" {
				fmt.Fprintf(userOut, "[opencode] %s\n", ev.Part.Tool)
			}
		}
	}
	return extractTitle(textBuf.String())
}

// extractTitle scans s line-by-line for the last JSON object with a non-empty
// "title" string field. We pick the last match so that a model that narrates
// the JSON it's about to emit ("I'll output {...}") doesn't beat the actual
// emission.
func extractTitle(s string) string {
	var found string
	for line := range strings.SplitSeq(s, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") || !strings.HasSuffix(line, "}") {
			continue
		}
		var obj struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal([]byte(line), &obj); err == nil && obj.Title != "" {
			found = obj.Title
		}
	}
	return found
}

// streamEvent is the trimmed-down shape of opencode's --format json events.
// We only model the fields we actually use.
type streamEvent struct {
	Type string `json:"type"`
	Part struct {
		Text string `json:"text"`
		Tool string `json:"tool"`
	} `json:"part"`
}

func init() {
	summarizer.Register(OpenCode{})
}
