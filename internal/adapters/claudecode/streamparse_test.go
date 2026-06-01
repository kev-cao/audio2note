package claudecode

import (
	"bytes"
	"strings"
	"testing"
)

// Realistic shape excerpted from `claude -p --output-format stream-json
// --json-schema '...'` runs. Whitespace within a line doesn't matter — each
// line is a single JSON event. The final `result` event carries the schema's
// structured_output payload, which is where the title comes from.
const sampleStream = `{"type":"system","subtype":"init","cwd":"/x"}
{"type":"assistant","message":{"content":[{"type":"text","text":"Quarterly Planning Sync"}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"path":"/foo"}}]}}
{"type":"result","subtype":"success","is_error":false,"structured_output":{"title":"Quarterly Planning Sync"}}
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
	if !strings.Contains(out.String(), "Quarterly Planning Sync") {
		t.Errorf("user output should include assistant text; got %q", out.String())
	}
}

func TestParseStreamJSON_SummarizesToolUses(t *testing.T) {
	var out bytes.Buffer
	parseStreamJSON(strings.NewReader(sampleStream), &out)
	if !strings.Contains(out.String(), "[claude] Read") {
		t.Errorf("user output should label tool uses; got %q", out.String())
	}
}

func TestParseStreamJSON_MissingTitleReturnsEmpty(t *testing.T) {
	// Missing structured_output entirely (e.g. agent never called the
	// schema'd output tool) — should not panic, just return "".
	stream := `{"type":"system","subtype":"init"}
{"type":"result","subtype":"success","is_error":false}
`
	var out bytes.Buffer
	got := parseStreamJSON(strings.NewReader(stream), &out)
	if got != "" {
		t.Errorf("missing title should be empty; got %q", got)
	}
}

func TestParseStreamJSON_UnparseableLineEchoesThrough(t *testing.T) {
	stream := "this is not JSON\n" +
		`{"type":"result","structured_output":{"title":"ok"}}` + "\n"
	var out bytes.Buffer
	got := parseStreamJSON(strings.NewReader(stream), &out)
	if got != "ok" {
		t.Errorf("title = %q, want %q", got, "ok")
	}
	if !strings.Contains(out.String(), "this is not JSON") {
		t.Errorf("unparseable line should be echoed; got %q", out.String())
	}
}
