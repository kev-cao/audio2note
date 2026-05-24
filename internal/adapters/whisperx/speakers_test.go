package whisperx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const inlineFixture = `{
  "segments": [
    {"speaker": "SPEAKER_00", "text": "Hello.", "start": 0.0, "end": 1.0},
    {"speaker": "SPEAKER_01", "text": "Hi there.", "start": 1.0, "end": 2.0},
    {"speaker": "SPEAKER_00", "text": "How are you?", "start": 2.0, "end": 3.5}
  ],
  "language": "en"
}`

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
	mw := WhisperX{}
	t.Run("happy path", func(t *testing.T) {
		got, err := mw.HasSpeakerLabels(writeFixture(t, "x.json", inlineFixture))
		if err != nil || !got {
			t.Fatalf("want true,nil; got %v,%v", got, err)
		}
	})
	t.Run("wrong extension", func(t *testing.T) {
		got, err := mw.HasSpeakerLabels(writeFixture(t, "x.txt", inlineFixture))
		if err != nil || got {
			t.Fatalf("want false,nil; got %v,%v", got, err)
		}
	})
	t.Run("json without segments", func(t *testing.T) {
		got, err := mw.HasSpeakerLabels(writeFixture(t, "x.json", `{"language":"en"}`))
		if err != nil || got {
			t.Fatalf("want false,nil; got %v,%v", got, err)
		}
	})
	t.Run("segments without speaker", func(t *testing.T) {
		got, err := mw.HasSpeakerLabels(writeFixture(t, "x.json", `{"segments":[{"text":"x"}]}`))
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
	t.Run("top-level array (macwhisper shape) is rejected", func(t *testing.T) {
		// whisperx wraps in an object; an array at the top is a different
		// schema and we should not silently claim to handle it.
		got, err := mw.HasSpeakerLabels(writeFixture(t, "x.json", `[{"speaker":"a","text":"x"}]`))
		if err != nil || got {
			t.Fatalf("want false,nil; got %v,%v", got, err)
		}
	})
}

func TestLoadSpeakers(t *testing.T) {
	speakers, err := WhisperX{}.LoadSpeakers(writeFixture(t, "x.json", inlineFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(speakers) != 2 {
		t.Fatalf("want 2 speakers, got %d", len(speakers))
	}
	wantOrder := []string{"SPEAKER_00", "SPEAKER_01"}
	for i, sp := range speakers {
		if sp.Name != wantOrder[i] {
			t.Errorf("speaker[%d] name = %q, want %q", i, sp.Name, wantOrder[i])
		}
	}
	if got := len(speakers[0].Quotes); got != 2 {
		t.Errorf("SPEAKER_00 quote count = %d, want 2", got)
	}
	if speakers[0].Quotes[0].Text != "Hello." {
		t.Errorf("SPEAKER_00 first quote = %q", speakers[0].Quotes[0].Text)
	}
}

func TestApplyRenames_PreservesUnknownFields(t *testing.T) {
	path := writeFixture(t, "x.json", inlineFixture)
	err := WhisperX{}.ApplyRenames(path, map[string]string{
		"SPEAKER_00": "Alice",
		"SPEAKER_01": "Bob",
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("rewritten file is not valid JSON: %v\n%s", err, data)
	}

	// language survived
	if got["language"] != "en" {
		t.Errorf("top-level 'language' lost: %v", got["language"])
	}
	segs, _ := got["segments"].([]any)
	if len(segs) != 3 {
		t.Fatalf("segment count = %d, want 3", len(segs))
	}
	wantSpeakers := []string{"Alice", "Bob", "Alice"}
	for i, raw := range segs {
		s := raw.(map[string]any)
		if s["speaker"] != wantSpeakers[i] {
			t.Errorf("segment[%d] speaker = %v, want %q", i, s["speaker"], wantSpeakers[i])
		}
	}
	// First segment's other fields survived.
	want0 := map[string]any{
		"speaker": "Alice",
		"text":    "Hello.",
		"start":   0.0,
		"end":     1.0,
	}
	got0 := segs[0].(map[string]any)
	if !reflect.DeepEqual(got0, want0) {
		t.Errorf("segment[0] roundtrip mismatch\n got: %#v\nwant: %#v", got0, want0)
	}
}

func TestApplyRenames_EmptyMapNoop(t *testing.T) {
	path := writeFixture(t, "x.json", inlineFixture)
	before, _ := os.ReadFile(path)
	if err := (WhisperX{}).ApplyRenames(path, nil); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Errorf("file changed despite empty rename map")
	}
}

// TestRoundTrip_SampleFixture exercises the editor against a fixture file
// (instead of the inline string) — closer to what real whisperx output looks
// like.
func TestRoundTrip_SampleFixture(t *testing.T) {
	src, err := os.ReadFile("testdata/sample.json")
	if err != nil {
		t.Fatal(err)
	}
	path := writeFixture(t, "sample.json", string(src))
	mw := WhisperX{}

	if ok, err := mw.HasSpeakerLabels(path); err != nil || !ok {
		t.Fatalf("HasSpeakerLabels = %v, %v; want true, nil", ok, err)
	}

	speakers, err := mw.LoadSpeakers(path)
	if err != nil {
		t.Fatal(err)
	}
	wantOrder := []string{"SPEAKER_00", "SPEAKER_01", "SPEAKER_02"}
	if len(speakers) != len(wantOrder) {
		t.Fatalf("got %d speakers, want %d", len(speakers), len(wantOrder))
	}
	for i, sp := range speakers {
		if sp.Name != wantOrder[i] {
			t.Errorf("speaker[%d] = %q, want %q", i, sp.Name, wantOrder[i])
		}
	}

	renames := map[string]string{
		"SPEAKER_00": "Alice",
		"SPEAKER_02": "Charlie",
	}
	if err := mw.ApplyRenames(path, renames); err != nil {
		t.Fatal(err)
	}

	// Re-parse before and after; compare every non-speaker segment field.
	var before, after map[string]any
	if err := json.Unmarshal(src, &before); err != nil {
		t.Fatal(err)
	}
	afterBytes, _ := os.ReadFile(path)
	if err := json.Unmarshal(afterBytes, &after); err != nil {
		t.Fatalf("rewritten file is not valid JSON: %v", err)
	}
	beforeSegs, _ := before["segments"].([]any)
	afterSegs, _ := after["segments"].([]any)
	if len(beforeSegs) != len(afterSegs) {
		t.Fatalf("segment count changed: %d → %d", len(beforeSegs), len(afterSegs))
	}
	for i := range beforeSegs {
		b := beforeSegs[i].(map[string]any)
		a := afterSegs[i].(map[string]any)
		origSpeaker := b["speaker"].(string)
		wantSpeaker := origSpeaker
		if v, ok := renames[origSpeaker]; ok {
			wantSpeaker = v
		}
		if a["speaker"] != wantSpeaker {
			t.Errorf("segment[%d] speaker = %v, want %q", i, a["speaker"], wantSpeaker)
		}
		for k, v := range b {
			if k == "speaker" {
				continue
			}
			if !reflect.DeepEqual(a[k], v) {
				t.Errorf("segment[%d] field %q changed: %v → %v", i, k, v, a[k])
			}
		}
	}
	if after["language"] != before["language"] {
		t.Errorf("top-level 'language' lost: %v → %v", before["language"], after["language"])
	}
}
