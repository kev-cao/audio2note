package opencode

import (
	"bytes"
	"strings"
	"testing"
)

// Realistic shape excerpted from `opencode run --format json` runs. Each line
// is one event; the model's response arrives as one or more "text" events
// whose `part.text` carries the actual content.
const sampleStream = `{"type":"step_start","sessionID":"s1"}
{"type":"text","part":{"text":"Wrote summary to notes.md\n{\"title\":\"Quarterly Planning Sync\"}"}}
{"type":"step_finish","tokens":{"total":42}}
`

func TestParseStreamJSON_ExtractsTitle(t *testing.T) {
	var out bytes.Buffer
	got := parseStreamJSON(strings.NewReader(sampleStream), &out)
	if got != "Quarterly Planning Sync" {
		t.Errorf("title = %q, want %q", got, "Quarterly Planning Sync")
	}
}

func TestParseStreamJSON_PrintsAssistantText(t *testing.T) {
	var out bytes.Buffer
	parseStreamJSON(strings.NewReader(sampleStream), &out)
	if !strings.Contains(out.String(), "Wrote summary to notes.md") {
		t.Errorf("user output should include assistant text; got %q", out.String())
	}
}

func TestParseStreamJSON_SummarizesToolUses(t *testing.T) {
	stream := `{"type":"tool","part":{"tool":"Read"}}
{"type":"text","part":{"text":"{\"title\":\"ok\"}"}}
`
	var out bytes.Buffer
	parseStreamJSON(strings.NewReader(stream), &out)
	if !strings.Contains(out.String(), "[opencode] Read") {
		t.Errorf("user output should label tool uses; got %q", out.String())
	}
}

func TestParseStreamJSON_MissingTitleReturnsEmpty(t *testing.T) {
	// No JSON object emitted (model ignored the instruction) — should not
	// panic, just return "".
	stream := `{"type":"step_start"}
{"type":"text","part":{"text":"summary written, no title block here"}}
{"type":"step_finish"}
`
	var out bytes.Buffer
	got := parseStreamJSON(strings.NewReader(stream), &out)
	if got != "" {
		t.Errorf("missing title should be empty; got %q", got)
	}
}

func TestParseStreamJSON_UnparseableLineEchoesThrough(t *testing.T) {
	stream := "this is not JSON\n" +
		`{"type":"text","part":{"text":"{\"title\":\"ok\"}"}}` + "\n"
	var out bytes.Buffer
	got := parseStreamJSON(strings.NewReader(stream), &out)
	if got != "ok" {
		t.Errorf("title = %q, want %q", got, "ok")
	}
	if !strings.Contains(out.String(), "this is not JSON") {
		t.Errorf("unparseable line should be echoed; got %q", out.String())
	}
}

func TestExtractTitle_PrefersLastMatch(t *testing.T) {
	// If the model narrates the JSON it's about to emit, the real emission
	// should win.
	s := `I'll output {"title":"placeholder"} at the end
{"title":"Real Final Title"}`
	if got := extractTitle(s); got != "Real Final Title" {
		t.Errorf("extractTitle = %q, want %q", got, "Real Final Title")
	}
}

func TestExtractTitle_StillFindsTitleInsideFences(t *testing.T) {
	// The prompt asks the model not to fence, but if it does anyway the
	// inner line is still a valid JSON object on its own — line-by-line
	// scanning picks it up, which is the lenient behavior we want.
	s := "```json\n{\"title\":\"fenced\"}\n```"
	if got := extractTitle(s); got != "fenced" {
		t.Errorf("extractTitle = %q, want %q (inner line should still parse)", got, "fenced")
	}
}

func TestExtractTitle_EmptyTitleSkipped(t *testing.T) {
	s := `{"title":""}
{"title":"non-empty"}`
	if got := extractTitle(s); got != "non-empty" {
		t.Errorf("extractTitle = %q, want %q", got, "non-empty")
	}
}
