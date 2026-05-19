// Package pipeline orchestrates transcription followed by summarization.
package pipeline

import (
	"context"
	"fmt"

	"audio2note/internal/summarizer"
	"audio2note/internal/transcriber"
)

type Options struct {
	AudioFile      string
	Transcriber    transcriber.Transcriber
	Summarizer     summarizer.Summarizer
	TranscriptFile string // absolute path
	OutputFile     string
	ObsidianVault  string // empty when no vault
}

func Run(ctx context.Context, opts Options) error {
	fmt.Printf("→ transcribing %s with %s → %s\n", opts.AudioFile, opts.Transcriber.Name(), opts.TranscriptFile)
	if err := opts.Transcriber.Transcribe(ctx, opts.AudioFile, opts.TranscriptFile); err != nil {
		return fmt.Errorf("transcribe: %w", err)
	}

	fmt.Printf("→ summarizing %s with %s\n", opts.TranscriptFile, opts.Summarizer.Name())
	in := summarizer.Input{
		TranscriptFile: opts.TranscriptFile,
		OutputFile:     opts.OutputFile,
		ObsidianVault:  opts.ObsidianVault,
	}
	if err := opts.Summarizer.Summarize(ctx, in); err != nil {
		return fmt.Errorf("summarize: %w", err)
	}
	return nil
}
