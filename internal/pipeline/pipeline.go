// Package pipeline orchestrates transcription followed by summarization.
package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"audio2note/internal/plaud"
	"audio2note/internal/plaudpicker"
	"audio2note/internal/speakeredit"
	"audio2note/internal/summarizer"
	"audio2note/internal/transcriber"
)

type Options struct {
	AudioFile             string
	Transcriber           transcriber.Transcriber
	Summarizer            summarizer.Summarizer
	TranscriptFile        string // absolute path
	OutputFile            string
	ObsidianVault         string // empty when no vault
	SkipTranscribe        bool
	SkipSummarize         bool
	TranscriberArgs       []string
	TranscriberNoDefaults bool
	SummarizerArgs        []string
	SummarizerNoDefaults  bool
	ExtraPrompt           string
	EditSpeakerLabels     bool
	PlaudClient           plaud.Client // nil = not in --plaud mode
}

func Run(ctx context.Context, opts Options) error {
	if opts.PlaudClient != nil {
		var err error
		opts, err = fetchFromPlaud(ctx, opts)
		if err != nil {
			return err
		}
	}

	if !opts.SkipTranscribe {
		fmt.Printf("→ transcribing %s with %s → %s\n", opts.AudioFile, opts.Transcriber.Name(), opts.TranscriptFile)
		in := transcriber.Input{
			AudioFile:  opts.AudioFile,
			OutputFile: opts.TranscriptFile,
			ExtraArgs:  opts.TranscriberArgs,
			NoDefaults: opts.TranscriberNoDefaults,
		}
		if err := opts.Transcriber.Transcribe(ctx, in); err != nil {
			return fmt.Errorf("transcribe: %w", err)
		}
	}

	if opts.EditSpeakerLabels {
		if err := runSpeakerEdit(opts); err != nil {
			return err
		}
	}

	if !opts.SkipSummarize {
		if _, err := os.Stat(opts.TranscriptFile); err != nil {
			return fmt.Errorf("transcript file: %w", err)
		}
		fmt.Printf("→ summarizing %s with %s\n", opts.TranscriptFile, opts.Summarizer.Name())
		in := summarizer.Input{
			TranscriptFile: opts.TranscriptFile,
			OutputFile:     opts.OutputFile,
			ObsidianVault:  opts.ObsidianVault,
			ExtraArgs:      opts.SummarizerArgs,
			NoDefaults:     opts.SummarizerNoDefaults,
			ExtraPrompt:    opts.ExtraPrompt,
		}
		if err := opts.Summarizer.Summarize(ctx, in); err != nil {
			return fmt.Errorf("summarize: %w", err)
		}
	}
	return nil
}

func fetchFromPlaud(ctx context.Context, opts Options) (Options, error) {
	fmt.Println("→ fetching Plaud library…")
	files, err := opts.PlaudClient.ListFiles(ctx)
	if err != nil {
		return opts, fmt.Errorf("plaud list files: %w", err)
	}
	if len(files) == 0 {
		return opts, fmt.Errorf("plaud library is empty")
	}
	selected, ok, err := plaudpicker.Pick(files)
	if err != nil {
		return opts, fmt.Errorf("plaud picker: %w", err)
	}
	if !ok {
		return opts, fmt.Errorf("plaud selection cancelled")
	}

	if opts.SkipTranscribe {
		fmt.Printf("→ downloading transcript for %s → %s\n", selected.Filename, opts.TranscriptFile)
		path, err := opts.PlaudClient.DownloadTranscript(ctx, selected.ID, opts.TranscriptFile)
		if err != nil {
			return opts, fmt.Errorf("download transcript: %w", err)
		}
		opts.TranscriptFile = path
		return opts, nil
	}

	tmpPath := filepath.Join(os.TempDir(), "audio2note-plaud-"+string(selected.ID)+selected.Extension)
	fmt.Printf("→ downloading recording for %s → %s\n", selected.Filename, tmpPath)
	path, err := opts.PlaudClient.DownloadRecording(ctx, selected.ID, tmpPath)
	if err != nil {
		return opts, fmt.Errorf("download recording: %w", err)
	}
	opts.AudioFile = path
	return opts, nil
}

func runSpeakerEdit(opts Options) error {
	editor, ok := opts.Transcriber.(transcriber.SpeakerLabelEditor)
	if !ok {
		return fmt.Errorf("transcriber %q does not support --edit-speaker-labels", opts.Transcriber.Name())
	}
	if _, err := os.Stat(opts.TranscriptFile); err != nil {
		return fmt.Errorf("transcript file: %w", err)
	}
	hasLabels, err := editor.HasSpeakerLabels(opts.TranscriptFile)
	if err != nil {
		return fmt.Errorf("validate speaker labels: %w", err)
	}
	if !hasLabels {
		return fmt.Errorf("transcript file %q is not in a format with speaker labels", opts.TranscriptFile)
	}
	speakers, err := editor.LoadSpeakers(opts.TranscriptFile)
	if err != nil {
		return fmt.Errorf("load speakers: %w", err)
	}
	if len(speakers) == 0 {
		fmt.Println("→ no speakers found, skipping edit step")
		return nil
	}
	fmt.Printf("→ editing %d speaker label(s) in %s\n", len(speakers), opts.TranscriptFile)
	renames, saved, err := speakeredit.Run(speakers)
	if err != nil {
		return fmt.Errorf("speaker edit: %w", err)
	}
	if !saved {
		return fmt.Errorf("speaker editing cancelled")
	}
	if len(renames) == 0 {
		return nil
	}
	if err := editor.ApplyRenames(opts.TranscriptFile, renames); err != nil {
		return fmt.Errorf("apply renames: %w", err)
	}
	fmt.Printf("→ applied %d rename(s)\n", len(renames))
	return nil
}
