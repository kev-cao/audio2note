// Package whisperx adapts the whisperx CLI to the Transcriber interface.
//
// Defaults: --model small --device cuda --output_format json --diarize.
// --diarize requires HuggingFace authentication (HF_TOKEN env var or
// `hf auth login`); the adapter pre-checks this and surfaces a
// clear error instead of letting whisperx fail downstream.
//
// whisperx writes its output as <output_dir>/<audio-stem>.<ext> — there's
// no flag to control the filename, only the directory. The adapter passes
// --output_dir <dir> and renames the stem-derived file to the caller's
// requested OutputFile when they differ.
package whisperx

import (
	"audio2note/internal/transcriber"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type WhisperX struct{}

func (WhisperX) Name() string { return "whisperx" }

func (WhisperX) Transcribe(ctx context.Context, in transcriber.Input) error {
	// --diarize fires only when defaults are applied. Pre-check HF auth so
	// the user gets a clear message instead of a downstream whisperx error.
	if !in.NoDefaults {
		if err := checkHuggingFaceAuth(); err != nil {
			return err
		}
	}

	outDir := filepath.Dir(in.OutputFile)
	if outDir == "" {
		outDir = "."
	}
	args := buildArgs(in, outDir)

	cmd := exec.CommandContext(ctx, "whisperx", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("whisperx: %w", err)
	}
	return renameStemFile(in.AudioFile, in.OutputFile, outDir)
}

// buildArgs returns the whisperx command-line arguments for in. Pure;
// extracted so it can be exercised by tests without invoking whisperx.
func buildArgs(in transcriber.Input, outDir string) []string {
	args := []string{in.AudioFile}
	if !in.NoDefaults {
		args = append(args,
			"--model", "small",
			"--device", "cuda",
			"--output_format", "json",
			"--diarize",
			"--no_align",
			"--print_progress", "True",
			"--verbose", "False",
		)
	}
	// Always set --output_dir so the adapter can find the result regardless
	// of the cwd at invocation time. In NoDefaults mode the user can still
	// override it with their own --transcriber-args (whisperx accepts the
	// last value for repeated flags).
	args = append(args, "--output_dir", outDir)
	args = append(args, in.ExtraArgs...)
	return args
}

// renameStemFile moves <outDir>/<audio-stem>.json to requested when they
// differ. No-ops if they match or if the expected file doesn't exist (which
// can happen when the user supplied --output_format with NoDefaults).
func renameStemFile(audioFile, requested, outDir string) error {
	stem := strings.TrimSuffix(filepath.Base(audioFile), filepath.Ext(audioFile))
	expected := filepath.Join(outDir, stem+".json")
	if expected == requested {
		return nil
	}
	if _, err := os.Stat(expected); err != nil {
		return nil
	}
	if err := os.Rename(expected, requested); err != nil {
		return fmt.Errorf("rename whisperx output %s → %s: %w", expected, requested, err)
	}
	return nil
}

// checkHuggingFaceAuth returns nil if HF_TOKEN is set or a HuggingFace
// token file is present on disk; otherwise it returns an instruction-style
// error. The token file path is the standard location written by
// `hf auth login`.
func checkHuggingFaceAuth() error {
	if os.Getenv("HF_TOKEN") != "" {
		return nil
	}
	home, err := os.UserHomeDir()
	if err == nil {
		info, statErr := os.Stat(filepath.Join(home, ".cache", "huggingface", "token"))
		if statErr == nil && info.Size() > 0 {
			return nil
		}
	}
	return errors.New("whisperx --diarize requires HuggingFace authentication; please run `hf auth login` or set $HF_TOKEN")
}

func init() { transcriber.Register(WhisperX{}) }
