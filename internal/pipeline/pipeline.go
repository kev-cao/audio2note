// Package pipeline orchestrates transcription followed by summarization.
//
// The pipeline is intentionally source-agnostic: callers populate the path
// fields on Options however they need to (local file, downloaded from a cloud
// service, etc.) and Run takes it from there.
package pipeline

import (
	"audio2note/internal/speakeredit"
	"audio2note/internal/summarizer"
	"audio2note/internal/transcriber"
	"context"
	"fmt"
	"os"
	"time"
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
	Highlights            []time.Duration
	// RequestTitle asks the summarizer to produce a meeting title alongside
	// its notes; the title shows up on the returned Result. Callers that
	// don't want a title (root command) leave this false.
	RequestTitle bool
}

// Result reports values produced during the pipeline that callers may need
// after the run (e.g. cmd/plaud uses Title to rename the recording).
type Result struct {
	// Title is the summarizer-produced meeting title when Options.RequestTitle
	// was true and the adapter successfully captured one. Empty otherwise.
	Title string
}

func Run(ctx context.Context, opts Options) (Result, error) {
	if !opts.SkipTranscribe {
		fmt.Printf("→ transcribing %s with %s → %s\n", opts.AudioFile, opts.Transcriber.Name(), opts.TranscriptFile)
		in := transcriber.Input{
			AudioFile:  opts.AudioFile,
			OutputFile: opts.TranscriptFile,
			ExtraArgs:  opts.TranscriberArgs,
			NoDefaults: opts.TranscriberNoDefaults,
		}
		if err := opts.Transcriber.Transcribe(ctx, in); err != nil {
			return Result{}, fmt.Errorf("transcribe: %w", err)
		}
	}

	if opts.EditSpeakerLabels {
		if err := runSpeakerEdit(opts); err != nil {
			return Result{}, err
		}
	}

	var result Result
	if !opts.SkipSummarize {
		if _, err := os.Stat(opts.TranscriptFile); err != nil {
			return Result{}, fmt.Errorf("transcript file: %w", err)
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
			RequestTitle:   opts.RequestTitle,
		}
		sumRes, err := opts.Summarizer.Summarize(ctx, in)
		if err != nil {
			return Result{}, fmt.Errorf("summarize: %w", err)
		}
		result.Title = sumRes.Title
	}
	return result, nil
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
