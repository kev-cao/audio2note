package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"audio2note/internal/plaud"
)

// richFakePlaud is a white-box-test-only Plaud client that records each call
// and lets the test control the responses. It writes a known byte sequence
// to the destination path when its download methods are invoked so subsequent
// stat-the-file logic can observe the side effect.
type richFakePlaud struct {
	downloadRecordingCalls  int
	downloadTranscriptCalls int
	getHighlightsCalls      int
	highlights              []plaud.Highlight
	highlightsErr           error
}

func (r *richFakePlaud) Validate(context.Context) error                  { return nil }
func (r *richFakePlaud) ListFiles(context.Context) ([]plaud.File, error) { return nil, nil }

func (r *richFakePlaud) DownloadRecording(_ context.Context, _ plaud.ID, dest string) (string, error) {
	r.downloadRecordingCalls++
	if err := os.WriteFile(dest, []byte("FAKE_AUDIO"), 0o600); err != nil {
		return "", err
	}
	return dest, nil
}

func (r *richFakePlaud) DownloadTranscript(_ context.Context, _ plaud.ID, dest string) (string, error) {
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

func TestFetchAfterPick_SummarizeOnly_FileExists_SkipsDownload(t *testing.T) {
	transcriptPath := filepath.Join(t.TempDir(), "exported.json")
	if err := os.WriteFile(transcriptPath, []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	fp := &richFakePlaud{highlights: []plaud.Highlight{plaud.Highlight(5 * time.Second)}}

	got, err := fetchAfterPick(context.Background(), Options{
		PlaudClient:    fp,
		SkipTranscribe: true,
		TranscriptFile: transcriptPath,
	}, sampleFile())
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
	if got.TranscriptFile != transcriptPath {
		t.Errorf("TranscriptFile = %q, want %q", got.TranscriptFile, transcriptPath)
	}
	// File contents must be untouched.
	data, _ := os.ReadFile(transcriptPath)
	if string(data) != "[]" {
		t.Errorf("transcript file was overwritten; got %q", data)
	}
	if len(got.Highlights) != 1 || got.Highlights[0] != 5*time.Second {
		t.Errorf("Highlights = %v, want [5s]", got.Highlights)
	}
}

func TestFetchAfterPick_SummarizeOnly_FileMissing_Downloads(t *testing.T) {
	transcriptPath := filepath.Join(t.TempDir(), "new.json")
	fp := &richFakePlaud{}

	got, err := fetchAfterPick(context.Background(), Options{
		PlaudClient:    fp,
		SkipTranscribe: true,
		TranscriptFile: transcriptPath,
	}, sampleFile())
	if err != nil {
		t.Fatal(err)
	}
	if fp.downloadTranscriptCalls != 1 {
		t.Errorf("DownloadTranscript calls = %d, want 1", fp.downloadTranscriptCalls)
	}
	if got.TranscriptFile != transcriptPath {
		t.Errorf("TranscriptFile = %q, want %q", got.TranscriptFile, transcriptPath)
	}
	data, _ := os.ReadFile(transcriptPath)
	if string(data) != "FAKE_TRANSCRIPT" {
		t.Errorf("transcript file contents = %q, want FAKE_TRANSCRIPT", data)
	}
}

func TestFetchAfterPick_DefaultFlow_DownloadsAudio(t *testing.T) {
	fp := &richFakePlaud{}

	got, err := fetchAfterPick(context.Background(), Options{
		PlaudClient: fp,
	}, sampleFile())
	if err != nil {
		t.Fatal(err)
	}
	if fp.downloadRecordingCalls != 1 {
		t.Errorf("DownloadRecording calls = %d, want 1", fp.downloadRecordingCalls)
	}
	if fp.downloadTranscriptCalls != 0 {
		t.Errorf("DownloadTranscript should not have been called")
	}
	if got.AudioFile == "" {
		t.Errorf("AudioFile should be populated")
	}
	if filepath.Ext(got.AudioFile) != ".mp3" {
		t.Errorf("AudioFile extension = %q, want .mp3", filepath.Ext(got.AudioFile))
	}
	// Confirm the file actually landed.
	if _, err := os.Stat(got.AudioFile); err != nil {
		t.Errorf("expected audio file at %s: %v", got.AudioFile, err)
	}
	t.Cleanup(func() { _ = os.Remove(got.AudioFile) })
}

func TestFetchAfterPick_HighlightsErrorIsNonFatal(t *testing.T) {
	transcriptPath := filepath.Join(t.TempDir(), "exported.json")
	if err := os.WriteFile(transcriptPath, []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	fp := &richFakePlaud{highlightsErr: errors.New("api down")}

	got, err := fetchAfterPick(context.Background(), Options{
		PlaudClient:    fp,
		SkipTranscribe: true,
		TranscriptFile: transcriptPath,
	}, sampleFile())
	if err != nil {
		t.Errorf("highlight error should not abort the pipeline; got %v", err)
	}
	if len(got.Highlights) != 0 {
		t.Errorf("Highlights should remain empty on fetch error; got %v", got.Highlights)
	}
}

func TestFetchAfterPick_NoHighlightsLeavesFieldNil(t *testing.T) {
	transcriptPath := filepath.Join(t.TempDir(), "exported.json")
	if err := os.WriteFile(transcriptPath, []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	fp := &richFakePlaud{} // returns nil highlights, no error

	got, err := fetchAfterPick(context.Background(), Options{
		PlaudClient:    fp,
		SkipTranscribe: true,
		TranscriptFile: transcriptPath,
	}, sampleFile())
	if err != nil {
		t.Fatal(err)
	}
	if got.Highlights != nil {
		t.Errorf("Highlights should be nil when there are no highlights; got %v", got.Highlights)
	}
}
