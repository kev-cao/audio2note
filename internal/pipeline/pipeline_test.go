package pipeline_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"audio2note/internal/pipeline"
	"audio2note/internal/plaud"
	"audio2note/internal/summarizer"
	"audio2note/internal/transcriber"
)

// --- fakes ---

type fakeTranscriber struct {
	name            string
	transcribeCalls int
	lastInput       transcriber.Input
	transcribeErr   error
}

func (f *fakeTranscriber) Name() string                  { return f.name }
func (f *fakeTranscriber) DefaultTranscriptFile() string { return "fake.txt" }
func (f *fakeTranscriber) Transcribe(_ context.Context, in transcriber.Input) error {
	f.transcribeCalls++
	f.lastInput = in
	return f.transcribeErr
}

type fakeSummarizer struct {
	name           string
	summarizeCalls int
	lastInput      summarizer.Input
	summarizeErr   error
}

func (f *fakeSummarizer) Name() string { return f.name }
func (f *fakeSummarizer) Summarize(_ context.Context, in summarizer.Input) error {
	f.summarizeCalls++
	f.lastInput = in
	return f.summarizeErr
}

// fakePlaudClient lets us exercise pipeline.fetchFromPlaud's pre-picker
// branches (list error, empty library) without spinning up a TTY.
type fakePlaudClient struct {
	files        []plaud.File
	listErr      error
	listCalls    int
}

func (f *fakePlaudClient) Validate(context.Context) error { return nil }
func (f *fakePlaudClient) ListFiles(context.Context) ([]plaud.File, error) {
	f.listCalls++
	return f.files, f.listErr
}
func (f *fakePlaudClient) DownloadRecording(context.Context, plaud.ID, string) (string, error) {
	return "", errors.New("not implemented in fake")
}
func (f *fakePlaudClient) DownloadTranscript(context.Context, plaud.ID, string) (string, error) {
	return "", errors.New("not implemented in fake")
}
func (f *fakePlaudClient) GetHighlights(context.Context, plaud.ID) ([]plaud.Highlight, error) {
	return nil, nil
}

// fakeSpeakerEditor satisfies both Transcriber and SpeakerLabelEditor.
type fakeSpeakerEditor struct {
	*fakeTranscriber
	hasLabels    bool
	hasLabelsErr error
	loadErr      error
}

func (f *fakeSpeakerEditor) HasSpeakerLabels(string) (bool, error) {
	return f.hasLabels, f.hasLabelsErr
}
func (f *fakeSpeakerEditor) LoadSpeakers(string) ([]transcriber.Speaker, error) {
	return nil, f.loadErr
}
func (f *fakeSpeakerEditor) ApplyRenames(string, map[string]string) error {
	return nil
}

// --- helpers ---

func writeTranscriptFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "transcript.txt")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// --- tests: orchestration ---

func TestRun_HappyPath(t *testing.T) {
	transcript := writeTranscriptFile(t, "hello")
	ft := &fakeTranscriber{name: "ft"}
	fs := &fakeSummarizer{name: "fs"}

	err := pipeline.Run(context.Background(), pipeline.Options{
		AudioFile:      "/audio.mp3",
		Transcriber:    ft,
		Summarizer:     fs,
		TranscriptFile: transcript,
		OutputFile:     "notes.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ft.transcribeCalls != 1 || fs.summarizeCalls != 1 {
		t.Errorf("calls = (%d transcribe, %d summarize), want (1, 1)", ft.transcribeCalls, fs.summarizeCalls)
	}
	if ft.lastInput.AudioFile != "/audio.mp3" || ft.lastInput.OutputFile != transcript {
		t.Errorf("transcriber input = %+v", ft.lastInput)
	}
	if fs.lastInput.TranscriptFile != transcript || fs.lastInput.OutputFile != "notes.md" {
		t.Errorf("summarizer input = %+v", fs.lastInput)
	}
}

func TestRun_SkipTranscribe(t *testing.T) {
	transcript := writeTranscriptFile(t, "hello")
	ft := &fakeTranscriber{}
	fs := &fakeSummarizer{}

	err := pipeline.Run(context.Background(), pipeline.Options{
		Transcriber:    ft,
		Summarizer:     fs,
		TranscriptFile: transcript,
		OutputFile:     "notes.md",
		SkipTranscribe: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ft.transcribeCalls != 0 {
		t.Errorf("Transcribe should not have run; calls = %d", ft.transcribeCalls)
	}
	if fs.summarizeCalls != 1 {
		t.Errorf("Summarize should have run once; calls = %d", fs.summarizeCalls)
	}
}

func TestRun_SkipSummarize(t *testing.T) {
	ft := &fakeTranscriber{}
	fs := &fakeSummarizer{}

	err := pipeline.Run(context.Background(), pipeline.Options{
		AudioFile:     "/audio.mp3",
		Transcriber:   ft,
		Summarizer:    fs,
		OutputFile:    "notes.md",
		SkipSummarize: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ft.transcribeCalls != 1 {
		t.Errorf("Transcribe should have run once; calls = %d", ft.transcribeCalls)
	}
	if fs.summarizeCalls != 0 {
		t.Errorf("Summarize should not have run; calls = %d", fs.summarizeCalls)
	}
}

func TestRun_TranscribeErrorPropagates(t *testing.T) {
	ft := &fakeTranscriber{transcribeErr: errors.New("boom")}
	fs := &fakeSummarizer{}

	err := pipeline.Run(context.Background(), pipeline.Options{
		AudioFile:   "/audio.mp3",
		Transcriber: ft,
		Summarizer:  fs,
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected transcribe error, got %v", err)
	}
	if fs.summarizeCalls != 0 {
		t.Errorf("Summarize should not have run after transcribe failure; calls = %d", fs.summarizeCalls)
	}
}

func TestRun_SummarizeErrorPropagates(t *testing.T) {
	transcript := writeTranscriptFile(t, "hello")
	ft := &fakeTranscriber{}
	fs := &fakeSummarizer{summarizeErr: errors.New("kaboom")}

	err := pipeline.Run(context.Background(), pipeline.Options{
		AudioFile:      "/audio.mp3",
		Transcriber:    ft,
		Summarizer:     fs,
		TranscriptFile: transcript,
	})
	if err == nil || !strings.Contains(err.Error(), "kaboom") {
		t.Errorf("expected summarize error, got %v", err)
	}
}

func TestRun_SummarizeMissingTranscriptErrors(t *testing.T) {
	ft := &fakeTranscriber{}
	fs := &fakeSummarizer{}

	err := pipeline.Run(context.Background(), pipeline.Options{
		Transcriber:    ft,
		Summarizer:     fs,
		TranscriptFile: "/definitely/does/not/exist.txt",
		SkipTranscribe: true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "transcript file") {
		t.Errorf("error = %q, want substring 'transcript file'", err)
	}
}

// --- tests: edit-speaker-labels error paths (no TUI invocation) ---

func TestRun_EditLabels_UnsupportedTranscriber(t *testing.T) {
	transcript := writeTranscriptFile(t, "hi")
	ft := &fakeTranscriber{name: "plainfake"}

	err := pipeline.Run(context.Background(), pipeline.Options{
		Transcriber:       ft,
		Summarizer:        &fakeSummarizer{},
		TranscriptFile:    transcript,
		SkipTranscribe:    true,
		SkipSummarize:     true,
		EditSpeakerLabels: true,
	})
	if err == nil {
		t.Fatal("expected unsupported error")
	}
	if !strings.Contains(err.Error(), "plainfake") || !strings.Contains(err.Error(), "edit-speaker-labels") {
		t.Errorf("error %q should name the transcriber and the flag", err)
	}
}

func TestRun_EditLabels_MissingFile(t *testing.T) {
	fse := &fakeSpeakerEditor{fakeTranscriber: &fakeTranscriber{name: "se"}}

	err := pipeline.Run(context.Background(), pipeline.Options{
		Transcriber:       fse,
		Summarizer:        &fakeSummarizer{},
		TranscriptFile:    "/nope/missing.json",
		SkipTranscribe:    true,
		SkipSummarize:     true,
		EditSpeakerLabels: true,
	})
	if err == nil || !strings.Contains(err.Error(), "transcript file") {
		t.Errorf("expected transcript-file error, got %v", err)
	}
}

func TestRun_EditLabels_NoLabels(t *testing.T) {
	transcript := writeTranscriptFile(t, "plain text")
	fse := &fakeSpeakerEditor{
		fakeTranscriber: &fakeTranscriber{name: "se"},
		hasLabels:       false,
	}

	err := pipeline.Run(context.Background(), pipeline.Options{
		Transcriber:       fse,
		Summarizer:        &fakeSummarizer{},
		TranscriptFile:    transcript,
		SkipTranscribe:    true,
		SkipSummarize:     true,
		EditSpeakerLabels: true,
	})
	if err == nil || !strings.Contains(err.Error(), "speaker labels") {
		t.Errorf("expected 'speaker labels' error, got %v", err)
	}
}

func TestRun_EditLabels_HasLabelsError(t *testing.T) {
	transcript := writeTranscriptFile(t, "x")
	fse := &fakeSpeakerEditor{
		fakeTranscriber: &fakeTranscriber{name: "se"},
		hasLabelsErr:    errors.New("read failure"),
	}

	err := pipeline.Run(context.Background(), pipeline.Options{
		Transcriber:       fse,
		Summarizer:        &fakeSummarizer{},
		TranscriptFile:    transcript,
		SkipTranscribe:    true,
		SkipSummarize:     true,
		EditSpeakerLabels: true,
	})
	if err == nil || !strings.Contains(err.Error(), "read failure") {
		t.Errorf("expected wrapped HasSpeakerLabels error, got %v", err)
	}
}

func TestRun_EditLabels_LoadError(t *testing.T) {
	transcript := writeTranscriptFile(t, "x")
	fse := &fakeSpeakerEditor{
		fakeTranscriber: &fakeTranscriber{name: "se"},
		hasLabels:       true,
		loadErr:         errors.New("parse failure"),
	}

	err := pipeline.Run(context.Background(), pipeline.Options{
		Transcriber:       fse,
		Summarizer:        &fakeSummarizer{},
		TranscriptFile:    transcript,
		SkipTranscribe:    true,
		SkipSummarize:     true,
		EditSpeakerLabels: true,
	})
	if err == nil || !strings.Contains(err.Error(), "parse failure") {
		t.Errorf("expected wrapped LoadSpeakers error, got %v", err)
	}
}

func TestRun_EditLabels_EmptySpeakerListIsNoop(t *testing.T) {
	transcript := writeTranscriptFile(t, "x")
	fse := &fakeSpeakerEditor{
		fakeTranscriber: &fakeTranscriber{name: "se"},
		hasLabels:       true,
		// loadErr nil + LoadSpeakers returns nil slice → pipeline should skip the TUI cleanly.
	}

	err := pipeline.Run(context.Background(), pipeline.Options{
		Transcriber:       fse,
		Summarizer:        &fakeSummarizer{},
		TranscriptFile:    transcript,
		SkipTranscribe:    true,
		SkipSummarize:     true,
		EditSpeakerLabels: true,
	})
	if err != nil {
		t.Errorf("expected no error when no speakers found, got %v", err)
	}
}

// --- tests: --plaud pre-picker error paths ---
//
// We can't drive plaudpicker.Pick from a test (it requires a TTY), so we only
// cover the branches in fetchFromPlaud that fire before the picker opens.

func TestRun_Plaud_ListFilesErrorPropagates(t *testing.T) {
	fpc := &fakePlaudClient{listErr: errors.New("boom")}

	err := pipeline.Run(context.Background(), pipeline.Options{
		Transcriber:    &fakeTranscriber{name: "ft"},
		Summarizer:     &fakeSummarizer{name: "fs"},
		TranscriptFile: writeTranscriptFile(t, ""),
		SkipTranscribe: true,
		SkipSummarize:  true,
		PlaudClient:    fpc,
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected wrapped 'boom' error, got %v", err)
	}
	if fpc.listCalls != 1 {
		t.Errorf("ListFiles calls = %d, want 1", fpc.listCalls)
	}
}

func TestRun_Plaud_EmptyLibraryErrors(t *testing.T) {
	fpc := &fakePlaudClient{files: nil}

	err := pipeline.Run(context.Background(), pipeline.Options{
		Transcriber:    &fakeTranscriber{},
		Summarizer:     &fakeSummarizer{},
		TranscriptFile: writeTranscriptFile(t, ""),
		SkipTranscribe: true,
		SkipSummarize:  true,
		PlaudClient:    fpc,
	})
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected 'empty' error, got %v", err)
	}
}
