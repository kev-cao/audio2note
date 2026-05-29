// Package audiostitch concatenates audio files into a single output using
// ffmpeg. All inputs must share the same codec/extension because we use
// ffmpeg's concat demuxer with stream copy (-c copy) — no re-encoding.
package audiostitch

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Stitch concatenates inputs in order into outputPath. The output's container
// is inferred from outputPath's extension and must match the inputs' shared
// extension. The caller owns outputPath cleanup.
func Stitch(ctx context.Context, inputs []string, outputPath string) error {
	if len(inputs) < 2 {
		return fmt.Errorf("audiostitch: need at least 2 inputs, got %d", len(inputs))
	}
	ext := strings.ToLower(filepath.Ext(inputs[0]))
	for _, in := range inputs[1:] {
		if got := strings.ToLower(filepath.Ext(in)); got != ext {
			return fmt.Errorf("audiostitch: cannot stitch mixed formats: %q (%s) and %q (%s)", inputs[0], ext, in, got)
		}
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("audiostitch: ffmpeg not found on PATH (install ffmpeg, e.g. `brew install ffmpeg`)")
	}

	listFile, err := writeConcatList(inputs)
	if err != nil {
		return err
	}
	defer os.Remove(listFile)

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listFile,
		"-c", "copy",
		outputPath,
	)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("audiostitch: ffmpeg failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// writeConcatList writes an ffmpeg concat-demuxer list to a temp file and
// returns its path. Each line is `file '<abs path>'` with single quotes inside
// the path escaped per ffmpeg's rules (' → '\”).
func writeConcatList(inputs []string) (string, error) {
	f, err := os.CreateTemp("", "audio2note-concat-*.txt")
	if err != nil {
		return "", fmt.Errorf("audiostitch: create concat list: %w", err)
	}
	defer f.Close()
	for _, in := range inputs {
		abs, err := filepath.Abs(in)
		if err != nil {
			os.Remove(f.Name())
			return "", fmt.Errorf("audiostitch: resolve %q: %w", in, err)
		}
		escaped := strings.ReplaceAll(abs, "'", `'\''`)
		if _, err := fmt.Fprintf(f, "file '%s'\n", escaped); err != nil {
			os.Remove(f.Name())
			return "", fmt.Errorf("audiostitch: write concat list: %w", err)
		}
	}
	return f.Name(), nil
}
