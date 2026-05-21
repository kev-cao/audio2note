// Package pipeline orchestrates transcription followed by summarization.
package pipeline

import (
	"context"
	"fmt"
	"os"

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
