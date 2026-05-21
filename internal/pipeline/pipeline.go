// Package pipeline orchestrates transcription followed by summarization.
package pipeline

import (
	"context"
	"fmt"
	"os"

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
}

func Run(ctx context.Context, opts Options) error {
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
		fmt.Println("→ speaker edits discarded")
		return nil
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
