package macwhisper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const fixture = `[
  {"speaker": "Speaker 1", "text": "Hello everyone.", "start": 0.0, "end": 2.5, "confidence": 0.95},
  {"speaker": "Speaker 2", "text": "Thanks for having us.", "start": 2.5, "end": 3.8, "confidence": 0.92},
  {"speaker": "Speaker 1", "text": "Let's get started.", "start": 3.8, "end": 5.5, "confidence": 0.96},
  {"speaker": "Speaker 3", "text": "Quick question first.", "start": 5.5, "end": 7.0, "confidence": 0.88}
]`

func writeFixture(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestHasSpeakerLabels(t *testing.T) {
	mw := MacWhisper{}
	t.Run("happy path", func(t *testing.T) {
		got, err := mw.HasSpeakerLabels(writeFixture(t, "x.json", fixture))
		if err != nil || !got {
			t.Fatalf("want true,nil; got %v,%v", got, err)
		}
	})
	t.Run("wrong extension", func(t *testing.T) {
		got, err := mw.HasSpeakerLabels(writeFixture(t, "x.txt", fixture))
		if err != nil || got {
			t.Fatalf("want false,nil; got %v,%v", got, err)
		}
	})
	t.Run("json without speaker", func(t *testing.T) {
		got, err := mw.HasSpeakerLabels(writeFixture(t, "x.json", `[{"foo":"bar"}]`))
		if err != nil || got {
			t.Fatalf("want false,nil; got %v,%v", got, err)
		}
	})
	t.Run("malformed json", func(t *testing.T) {
		got, err := mw.HasSpeakerLabels(writeFixture(t, "x.json", `not json`))
		if err != nil || got {
			t.Fatalf("want false,nil; got %v,%v", got, err)
		}
	})
}

func TestLoadSpeakers(t *testing.T) {
	speakers, err := MacWhisper{}.LoadSpeakers(writeFixture(t, "x.json", fixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(speakers) != 3 {
		t.Fatalf("want 3 speakers, got %d", len(speakers))
	}
	wantOrder := []string{"Speaker 1", "Speaker 2", "Speaker 3"}
	for i, sp := range speakers {
		if sp.Name != wantOrder[i] {
			t.Errorf("speaker[%d] name = %q, want %q", i, sp.Name, wantOrder[i])
		}
	}
	if got := len(speakers[0].Quotes); got != 2 {
		t.Errorf("Speaker 1 quote count = %d, want 2", got)
	}
	if speakers[0].Quotes[0].Text != "Hello everyone." {
		t.Errorf("Speaker 1 first quote = %q", speakers[0].Quotes[0].Text)
	}
}

func TestApplyRenames_PreservesUnknownFields(t *testing.T) {
	path := writeFixture(t, "x.json", fixture)
	err := MacWhisper{}.ApplyRenames(path, map[string]string{
		"Speaker 1": "Alice",
		"Speaker 3": "Charlie",
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("rewritten file is not valid JSON: %v\n%s", err, data)
	}

	wantSpeakers := []string{"Alice", "Speaker 2", "Alice", "Charlie"}
	for i, rec := range got {
		if rec["speaker"] != wantSpeakers[i] {
			t.Errorf("record[%d] speaker = %v, want %q", i, rec["speaker"], wantSpeakers[i])
		}
	}
	// Confidence and timestamps from record 0 should survive.
	rec0 := got[0]
	want0 := map[string]any{
		"speaker":    "Alice",
		"text":       "Hello everyone.",
		"start":      0.0,
		"end":        2.5,
		"confidence": 0.95,
	}
	if !reflect.DeepEqual(rec0, want0) {
		t.Errorf("record[0] roundtrip mismatch\n got: %#v\nwant: %#v", rec0, want0)
	}
}

func TestApplyRenames_EmptyMapNoop(t *testing.T) {
	path := writeFixture(t, "x.json", fixture)
	before, _ := os.ReadFile(path)
	if err := (MacWhisper{}).ApplyRenames(path, nil); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Errorf("file changed despite empty rename map")
	}
}

// TestRoundTrip_SampleFixture exercises the editor against a realistic
// transcript shape (multiple speakers, varied metadata fields) and verifies
// that ApplyRenames preserves every non-`speaker` field on every segment.
func TestRoundTrip_SampleFixture(t *testing.T) {
	src, err := os.ReadFile("testdata/sample.json")
	if err != nil {
		t.Fatal(err)
	}
	path := writeFixture(t, "sample.json", string(src))
	mw := MacWhisper{}

	// Validate.
	if ok, err := mw.HasSpeakerLabels(path); err != nil || !ok {
		t.Fatalf("HasSpeakerLabels = %v, %v; want true, nil", ok, err)
	}

	// LoadSpeakers should report 5 unique speakers, in order of first appearance.
	speakers, err := mw.LoadSpeakers(path)
	if err != nil {
		t.Fatal(err)
	}
	wantOrder := []string{"Speaker 1", "Speaker 2", "Speaker 3", "Speaker 4", "Speaker 5"}
	if len(speakers) != len(wantOrder) {
		t.Fatalf("got %d speakers, want %d", len(speakers), len(wantOrder))
	}
	for i, sp := range speakers {
		if sp.Name != wantOrder[i] {
			t.Errorf("speaker[%d] = %q, want %q", i, sp.Name, wantOrder[i])
		}
		if len(sp.Quotes) == 0 {
			t.Errorf("speaker[%d] %q has no quotes", i, sp.Name)
		}
	}

	// Rename three of the five.
	renames := map[string]string{
		"Speaker 1": "Alice",
		"Speaker 3": "Charlie",
		"Speaker 5": "Eve",
	}
	if err := mw.ApplyRenames(path, renames); err != nil {
		t.Fatal(err)
	}

	// Re-parse both before and after; compare every non-speaker field per
	// segment. This catches any silent field loss in the rewrite path.
	var before, after []map[string]any
	if err := json.Unmarshal(src, &before); err != nil {
		t.Fatal(err)
	}
	afterBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(afterBytes, &after); err != nil {
		t.Fatalf("rewritten file is not valid JSON: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("segment count changed: %d → %d", len(before), len(after))
	}
	for i := range before {
		// Check speaker was applied per the rename map.
		origSpeaker := before[i]["speaker"].(string)
		wantSpeaker := origSpeaker
		if v, ok := renames[origSpeaker]; ok {
			wantSpeaker = v
		}
		if after[i]["speaker"] != wantSpeaker {
			t.Errorf("segment[%d] speaker = %v, want %q", i, after[i]["speaker"], wantSpeaker)
		}
		// Every other field must round-trip identically.
		for k, v := range before[i] {
			if k == "speaker" {
				continue
			}
			if !reflect.DeepEqual(after[i][k], v) {
				t.Errorf("segment[%d] field %q changed: %v → %v", i, k, v, after[i][k])
			}
		}
		// And no new fields were introduced.
		for k := range after[i] {
			if _, ok := before[i][k]; !ok {
				t.Errorf("segment[%d] gained unexpected field %q", i, k)
			}
		}
	}
}
