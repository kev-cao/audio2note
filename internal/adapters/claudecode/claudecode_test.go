package claudecode

import "testing"

// Realistic shape excerpted from `claude -p --output-format json
// --json-schema '...'` runs. The single blob carries the final assistant
// text in `result` and the schema-validated payload in `structured_output`.
const sampleResult = `{"type":"result","subtype":"success","is_error":false,"result":"Wrote summary to notes.md","structured_output":{"title":"Quarterly Planning Sync"}}`

func TestParseResult_ExtractsTitleAndText(t *testing.T) {
	text, title := parseResult([]byte(sampleResult))
	if title != "Quarterly Planning Sync" {
		t.Errorf("title = %q, want %q", title, "Quarterly Planning Sync")
	}
	if text != "Wrote summary to notes.md" {
		t.Errorf("text = %q, want %q", text, "Wrote summary to notes.md")
	}
}

func TestParseResult_MissingTitleReturnsEmpty(t *testing.T) {
	// Missing structured_output entirely (e.g. agent never called the
	// schema'd output tool) — should not panic, just return "".
	body := `{"type":"result","subtype":"success","is_error":false,"result":"done"}`
	text, title := parseResult([]byte(body))
	if title != "" {
		t.Errorf("missing title should be empty; got %q", title)
	}
	if text != "done" {
		t.Errorf("text = %q, want %q", text, "done")
	}
}

func TestParseResult_UnparseableReturnsZero(t *testing.T) {
	text, title := parseResult([]byte("this is not JSON"))
	if title != "" || text != "" {
		t.Errorf("unparseable body should return (\"\",\"\"); got (%q, %q)", text, title)
	}
}
