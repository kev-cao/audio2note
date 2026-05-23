package plaudpicker

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"

	"audio2note/internal/plaud"
)

// The TUI itself isn't unit-tested (same rationale as internal/speakeredit's
// tui.go). What we *do* test is the row-rendering delegate — it's pure logic
// that's easy to assert on.

func TestDelegateRender_IncludesAllFields(t *testing.T) {
	f := plaud.File{
		ID:            "abc123def4567890",
		Filename:      "Test meeting",
		Extension:     ".mp3",
		Duration:      226 * time.Second,
		HasTranscript: true,
	}
	items := []list.Item{item{file: f}}
	l := list.New(items, delegate{}, 80, 10)

	var buf bytes.Buffer
	delegate{}.Render(&buf, l, 0, items[0])
	out := buf.String()

	if !strings.Contains(out, "Test meeting") {
		t.Errorf("missing filename: %q", out)
	}
	if !strings.Contains(out, "abc123de") {
		t.Errorf("missing truncated id: %q", out)
	}
	if strings.Contains(out, "abc123def4567890") {
		t.Errorf("full id should not appear — should be truncated: %q", out)
	}
	if !strings.Contains(out, "3m46s") {
		t.Errorf("missing duration: %q", out)
	}
	if !strings.Contains(out, "✓") {
		t.Errorf("missing has-transcript indicator: %q", out)
	}
	if !strings.Contains(out, "▶") {
		t.Errorf("selected row should show ▶ prefix: %q", out)
	}
}

func TestDelegateRender_OmitsTranscriptIndicatorWhenAbsent(t *testing.T) {
	f := plaud.File{
		ID:            "x",
		Filename:      "no transcript",
		Duration:      10 * time.Second,
		HasTranscript: false,
	}
	items := []list.Item{item{file: f}}
	l := list.New(items, delegate{}, 80, 10)

	var buf bytes.Buffer
	delegate{}.Render(&buf, l, 0, items[0])
	out := buf.String()

	if strings.Contains(out, "✓") {
		t.Errorf("has-transcript indicator should be absent: %q", out)
	}
}

func TestItem_FilterValue(t *testing.T) {
	it := item{file: plaud.File{Filename: "Standup 2026-05"}}
	if got := it.FilterValue(); got != "Standup 2026-05" {
		t.Errorf("FilterValue = %q, want %q", got, "Standup 2026-05")
	}
}
