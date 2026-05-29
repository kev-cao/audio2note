package cmd

import (
	"audio2note/internal/plaud"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// richFakePlaud lets us drive runPlaudAfterPick directly. It records each
// call and lets the test control the responses; the download methods write a
// known byte sequence so the file-existence branch can observe the side
// effect.
type richFakePlaud struct {
	downloadRecordingCalls  int
	downloadTranscriptCalls int
	getHighlightsCalls      int
	highlights              []plaud.Highlight
	highlightsErr           error
}

func (r *richFakePlaud) Validate(context.Context) error                  { return nil }
func (r *richFakePlaud) ListFiles(context.Context) ([]plaud.File, error) { return nil, nil }

func (r *richFakePlaud) DownloadRecording(
	_ context.Context, _ plaud.ID, dest string,
) (string, error) {
	r.downloadRecordingCalls++
	if err := os.WriteFile(dest, []byte("FAKE_AUDIO"), 0o600); err != nil {
		return "", err
	}
	return dest, nil
}

func (r *richFakePlaud) DownloadTranscript(
	_ context.Context, _ plaud.ID, dest string,
) (string, error) {
	r.downloadTranscriptCalls++
	if err := os.WriteFile(dest, []byte("FAKE_TRANSCRIPT"), 0o600); err != nil {
		return "", err
	}
	return dest, nil
}

func (r *richFakePlaud) GetHighlights(context.Context, plaud.ID) ([]plaud.Highlight, error) {
	r.getHighlightsCalls++
	return r.highlights, r.highlightsErr
}

func sampleFile() plaud.File {
	return plaud.File{ID: "abc123", Filename: "Sample", Extension: ".mp3"}
}

// baseFlags returns a flags struct that buildOptions can succeed with: the
// transcriber/summarizer names match the real registered adapters but we
// never actually invoke pipeline.Run from these tests, so the adapters are
// never asked to do anything.
func baseFlags(transcriptFile string) flags {
	return flags{
		transcriber:    "macwhisper",
		summarizer:     "claudecode",
		transcriptFile: transcriptFile,
	}
}

func TestRunPlaudAfterPick_SummarizeOnly_FileExists_SkipsDownload(t *testing.T) {
	transcriptPath := filepath.Join(t.TempDir(), "exported.json")
	if err := os.WriteFile(transcriptPath, []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	fp := &richFakePlaud{highlights: []plaud.Highlight{plaud.Highlight(5 * time.Second)}}

	f := baseFlags(transcriptPath)
	f.summarizeOnly = true

	opts, err := runPlaudAfterPick(context.Background(), fp, []plaud.File{sampleFile()}, f, false)
	if err != nil {
		t.Fatal(err)
	}
	if fp.downloadTranscriptCalls != 0 {
		t.Errorf("DownloadTranscript should not have been called; got %d", fp.downloadTranscriptCalls)
	}
	if fp.downloadRecordingCalls != 0 {
		t.Errorf("DownloadRecording should not have been called; got %d", fp.downloadRecordingCalls)
	}
	if fp.getHighlightsCalls != 1 {
		t.Errorf("GetHighlights calls = %d, want 1", fp.getHighlightsCalls)
	}
	data, _ := os.ReadFile(transcriptPath)
	if string(data) != "[]" {
		t.Errorf("transcript file was overwritten; got %q", data)
	}
	if opts.TranscriptFile != transcriptPath {
		t.Errorf("opts.TranscriptFile = %q, want %q", opts.TranscriptFile, transcriptPath)
	}
	if len(opts.Highlights) != 1 || opts.Highlights[0] != 5*time.Second {
		t.Errorf("opts.Highlights = %v, want [5s]", opts.Highlights)
	}
}

func TestRunPlaudAfterPick_SummarizeOnly_FileMissing_Downloads(t *testing.T) {
	transcriptPath := filepath.Join(t.TempDir(), "new.json")
	fp := &richFakePlaud{}

	f := baseFlags(transcriptPath)
	f.summarizeOnly = true

	opts, err := runPlaudAfterPick(context.Background(), fp, []plaud.File{sampleFile()}, f, false)
	if err != nil {
		t.Fatal(err)
	}
	if fp.downloadTranscriptCalls != 1 {
		t.Errorf("DownloadTranscript calls = %d, want 1", fp.downloadTranscriptCalls)
	}
	data, _ := os.ReadFile(transcriptPath)
	if string(data) != "FAKE_TRANSCRIPT" {
		t.Errorf("transcript file contents = %q, want FAKE_TRANSCRIPT", data)
	}
	if opts.TranscriptFile != transcriptPath {
		t.Errorf("opts.TranscriptFile = %q, want %q", opts.TranscriptFile, transcriptPath)
	}
}

func TestRunPlaudAfterPick_DefaultFlow_DownloadsAudio(t *testing.T) {
	transcriptPath := filepath.Join(t.TempDir(), "Transcript.txt")
	fp := &richFakePlaud{}

	f := baseFlags(transcriptPath)
	// default flow — neither summarizeOnly nor transcribeOnly

	opts, err := runPlaudAfterPick(context.Background(), fp, []plaud.File{sampleFile()}, f, false)
	if err != nil {
		t.Fatal(err)
	}
	if fp.downloadRecordingCalls != 1 {
		t.Errorf("DownloadRecording calls = %d, want 1", fp.downloadRecordingCalls)
	}
	if fp.downloadTranscriptCalls != 0 {
		t.Errorf("DownloadTranscript should not have been called")
	}
	if opts.AudioFile == "" {
		t.Errorf("opts.AudioFile should be populated")
	}
	if filepath.Ext(opts.AudioFile) != ".mp3" {
		t.Errorf("opts.AudioFile extension = %q, want .mp3", filepath.Ext(opts.AudioFile))
	}
	if _, err := os.Stat(opts.AudioFile); err != nil {
		t.Errorf("expected audio file on disk at %s: %v", opts.AudioFile, err)
	}
	t.Cleanup(func() { _ = os.Remove(opts.AudioFile) })
}

func TestRunPlaudAfterPick_HighlightsErrorIsNonFatal(t *testing.T) {
	transcriptPath := filepath.Join(t.TempDir(), "exported.json")
	if err := os.WriteFile(transcriptPath, []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	fp := &richFakePlaud{highlightsErr: errors.New("api down")}

	f := baseFlags(transcriptPath)
	f.summarizeOnly = true

	opts, err := runPlaudAfterPick(context.Background(), fp, []plaud.File{sampleFile()}, f, false)
	if err != nil {
		t.Errorf("highlight error should not abort the pipeline; got %v", err)
	}
	if len(opts.Highlights) != 0 {
		t.Errorf("opts.Highlights should remain empty on fetch error; got %v", opts.Highlights)
	}
}

func TestRunPlaudAfterPick_NoHighlightsLeavesFieldNil(t *testing.T) {
	transcriptPath := filepath.Join(t.TempDir(), "exported.json")
	if err := os.WriteFile(transcriptPath, []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	fp := &richFakePlaud{} // returns nil highlights, no error

	f := baseFlags(transcriptPath)
	f.summarizeOnly = true

	opts, err := runPlaudAfterPick(context.Background(), fp, []plaud.File{sampleFile()}, f, false)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Highlights != nil {
		t.Errorf("opts.Highlights should be nil when there are no highlights; got %v", opts.Highlights)
	}
}

func TestRunPlaudAfterPick_SummarizeOnly_MultiRequiresExistingTranscript(t *testing.T) {
	transcriptPath := filepath.Join(t.TempDir(), "doesnt-exist.json")
	fp := &richFakePlaud{}

	f := baseFlags(transcriptPath)
	f.summarizeOnly = true

	a := plaud.File{ID: "a", Filename: "A", Extension: ".mp3", Duration: 60 * time.Second}
	b := plaud.File{ID: "b", Filename: "B", Extension: ".mp3", Duration: 30 * time.Second}

	_, err := runPlaudAfterPick(context.Background(), fp, []plaud.File{a, b}, f, false)
	if err == nil {
		t.Fatal("want error when --summarize-only multi has no local transcript; got nil")
	}
	if fp.downloadTranscriptCalls != 0 {
		t.Errorf("DownloadTranscript should not be called; got %d", fp.downloadTranscriptCalls)
	}
}

func TestRunPlaudAfterPick_SummarizeOnly_MultiFetchesHighlightsForAll(t *testing.T) {
	transcriptPath := filepath.Join(t.TempDir(), "stitched.json")
	if err := os.WriteFile(transcriptPath, []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	fp := &richFakePlaud{
		highlights: []plaud.Highlight{plaud.Highlight(5 * time.Second)},
	}

	f := baseFlags(transcriptPath)
	f.summarizeOnly = true

	a := plaud.File{ID: "a", Filename: "A", Extension: ".mp3", Duration: 60 * time.Second}
	b := plaud.File{ID: "b", Filename: "B", Extension: ".mp3", Duration: 30 * time.Second}

	opts, err := runPlaudAfterPick(context.Background(), fp, []plaud.File{a, b}, f, false)
	if err != nil {
		t.Fatal(err)
	}
	if fp.getHighlightsCalls != 2 {
		t.Errorf("GetHighlights should fire once per selected file; got %d", fp.getHighlightsCalls)
	}
	// Both files report a single highlight at 5s; the second should be
	// offset by file A's 60s duration → [5s, 65s].
	if len(opts.Highlights) != 2 || opts.Highlights[0] != 5*time.Second || opts.Highlights[1] != 65*time.Second {
		t.Errorf("opts.Highlights = %v, want [5s 65s]", opts.Highlights)
	}
}

func TestOffsetHighlights(t *testing.T) {
	f := func(d time.Duration) plaud.File { return plaud.File{Duration: d} }
	tests := []struct {
		name     string
		selected []plaud.File
		perFile  [][]time.Duration
		want     []time.Duration
	}{
		{
			name:     "empty",
			selected: nil,
			perFile:  nil,
			want:     nil,
		},
		{
			name:     "single file passes highlights through",
			selected: []plaud.File{f(60 * time.Second)},
			perFile:  [][]time.Duration{{10 * time.Second, 30 * time.Second}},
			want:     []time.Duration{10 * time.Second, 30 * time.Second},
		},
		{
			name:     "two files, second offset by first's duration",
			selected: []plaud.File{f(60 * time.Second), f(40 * time.Second)},
			perFile:  [][]time.Duration{{10 * time.Second}, {5 * time.Second, 20 * time.Second}},
			want:     []time.Duration{10 * time.Second, 65 * time.Second, 80 * time.Second},
		},
		{
			name:     "empty middle file still offsets later files",
			selected: []plaud.File{f(30 * time.Second), f(20 * time.Second), f(10 * time.Second)},
			perFile:  [][]time.Duration{{5 * time.Second}, nil, {1 * time.Second}},
			want:     []time.Duration{5 * time.Second, 51 * time.Second},
		},
		{
			name:     "all empty returns nil",
			selected: []plaud.File{f(60 * time.Second), f(40 * time.Second)},
			perFile:  [][]time.Duration{nil, nil},
			want:     nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := offsetHighlights(tc.selected, tc.perFile)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("got[%d] = %v, want %v", i, got[i], tc.want[i])
				}
			}
		})
	}
}
