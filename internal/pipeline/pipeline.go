// Package pipeline orchestrates transcription followed by summarization.
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

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
	Highlights            []time.Duration
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
			Highlights:     opts.Highlights,
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
	return fetchAfterPick(ctx, opts, selected)
}

// fetchAfterPick handles the post-picker work: figures out whether to download
// the recording or the transcript, then always tries to fetch highlights.
// Highlight fetch failures are non-fatal — summarization is more important
// than the highlight annotation.
func fetchAfterPick(ctx context.Context, opts Options, selected plaud.File) (Options, error) {
	if opts.SkipTranscribe {
		// TODO(plaud-mw-export): once the MacWhisper CLI supports exporting
		// transcripts with timestamps, this file-existence branch can be
		// removed. The whole workflow becomes: `--plaud --summarize-only`
		// always downloads the transcript from Plaud. The current branch
		// exists so the user can run `--plaud --transcribe-only`, manually
		// export a timestamped JSON from the MacWhisper desktop app, and
		// then `--plaud --summarize-only --transcript-file exported.json`
		// to feed the local file while still pulling highlights from Plaud.
		switch _, statErr := os.Stat(opts.TranscriptFile); {
		case statErr == nil:
			fmt.Printf("→ using existing transcript file %s; fetching highlights only\n", opts.TranscriptFile)
		case errors.Is(statErr, os.ErrNotExist):
			fmt.Printf("→ downloading transcript for %s → %s\n", selected.Filename, opts.TranscriptFile)
			path, err := opts.PlaudClient.DownloadTranscript(ctx, selected.ID, opts.TranscriptFile)
			if err != nil {
				return opts, fmt.Errorf("download transcript: %w", err)
			}
			opts.TranscriptFile = path
		default:
			return opts, fmt.Errorf("transcript file: %w", statErr)
		}
	} else {
		tmpPath := filepath.Join(os.TempDir(), "audio2note-plaud-"+string(selected.ID)+selected.Extension)
		fmt.Printf("→ downloading recording for %s → %s\n", selected.Filename, tmpPath)
		path, err := opts.PlaudClient.DownloadRecording(ctx, selected.ID, tmpPath)
		if err != nil {
			return opts, fmt.Errorf("download recording: %w", err)
		}
		opts.AudioFile = path
	}

	highlights, err := opts.PlaudClient.GetHighlights(ctx, selected.ID)
	if err != nil {
		fmt.Printf("→ couldn't fetch highlights: %v (proceeding without)\n", err)
		return opts, nil
	}
	if len(highlights) == 0 {
		return opts, nil
	}
	fmt.Printf("→ got %d highlight(s)\n", len(highlights))
	opts.Highlights = make([]time.Duration, len(highlights))
	for i, h := range highlights {
		opts.Highlights[i] = time.Duration(h)
	}
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
