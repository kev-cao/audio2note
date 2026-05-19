// Package macwhisper adapts the MacWhisper CLI (`mw`) to the Transcriber
// interface.
//
// `mw transcribe <file>` prints the transcript to stdout, so this adapter
// redirects stdout to the caller-chosen output file. stderr passes through so
// any progress messages or warnings from mw remain visible.
package macwhisper

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"audio2note/internal/transcriber"
)

type MacWhisper struct{}

func (MacWhisper) Name() string                  { return "macwhisper" }
func (MacWhisper) DefaultTranscriptFile() string { return "Transcript.txt" }

func (MacWhisper) Transcribe(ctx context.Context, audioFile, outputFile string) error {
	f, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("create transcript file: %w", err)
	}
	defer f.Close()

	cmd := exec.CommandContext(ctx, "mw", "transcribe", audioFile)
	cmd.Stdout = f
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func init() {
	transcriber.Register(MacWhisper{})
}
