// Package macwhisper adapts the MacWhisper CLI (`mw`) to the Transcriber
// interface.
//
// `mw transcribe <file>` prints the transcript to stdout, so this adapter
// redirects stdout to the caller-chosen output file. stderr passes through so
// any progress messages or warnings from mw remain visible.
package macwhisper

import (
	"audio2note/internal/transcriber"
	"context"
	"fmt"
	"os"
	"os/exec"
)

type MacWhisper struct{}

func (MacWhisper) Name() string { return "macwhisper" }

func (MacWhisper) Transcribe(ctx context.Context, in transcriber.Input) error {
	f, err := os.Create(in.OutputFile)
	if err != nil {
		return fmt.Errorf("create transcript file: %w", err)
	}
	defer f.Close()

	args := []string{"transcribe", in.AudioFile}
	if !in.NoDefaults {
		// --persist saves the transcription to MacWhisper's history.
		args = append(args, "--persist")
	}
	args = append(args, in.ExtraArgs...)

	cmd := exec.CommandContext(ctx, "mw", args...)
	cmd.Stdout = f
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func init() {
	transcriber.Register(MacWhisper{})
}
