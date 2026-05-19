// Package cmd wires the cobra root command, parses flags, and hands off to the
// pipeline.
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"audio2note/internal/pipeline"
	"audio2note/internal/summarizer"
	"audio2note/internal/transcriber"

	// Blank-import adapters so their init() registers them. Adding a new
	// adapter = new package + one line here.
	_ "audio2note/internal/adapters/claudecode"
	_ "audio2note/internal/adapters/macwhisper"
)

const obsidianVaultEnv = "OBSIDIAN_VAULT_PATH"

type flags struct {
	transcriber    string
	summarizer     string
	transcriptFile string
	obsidian       bool
	obsidianVault  string
	outputFile     string
}

func newRootCmd() *cobra.Command {
	var f flags
	cmd := &cobra.Command{
		Use:           "audio2note <audio-file>",
		Short:         "Transcribe an audio file and summarize it into meeting notes.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), args[0], f, cmd.Flags().Changed("obsidian-vault"))
		},
	}

	cmd.Flags().StringVar(&f.transcriber, "transcriber", "macwhisper", "Transcriber adapter to use")
	cmd.Flags().StringVar(&f.summarizer, "summarizer", "claudecode", "Summarizer adapter to use")
	cmd.Flags().StringVar(&f.transcriptFile, "transcript-file", "", "Path to the transcript file (defaults to the transcriber's default filename in cwd)")
	cmd.Flags().BoolVar(&f.obsidian, "obsidian", false, "Write notes to the Obsidian vault at $"+obsidianVaultEnv)
	cmd.Flags().StringVar(&f.obsidianVault, "obsidian-vault", "", "Explicit Obsidian vault path (implies --obsidian)")
	cmd.Flags().StringVar(&f.outputFile, "output-file", "transcript-notes.md", "File name for the summarizer's notes output")

	return cmd
}

func run(ctx context.Context, audioArg string, f flags, vaultFlagSet bool) error {
	audioFile, err := filepath.Abs(audioArg)
	if err != nil {
		return fmt.Errorf("resolve audio file: %w", err)
	}
	if _, err := os.Stat(audioFile); err != nil {
		return fmt.Errorf("audio file: %w", err)
	}

	tr, err := transcriber.Get(f.transcriber)
	if err != nil {
		return fmt.Errorf("transcriber: %w", err)
	}
	sum, err := summarizer.Get(f.summarizer)
	if err != nil {
		return fmt.Errorf("summarizer: %w", err)
	}

	transcriptPath := f.transcriptFile
	if transcriptPath == "" {
		transcriptPath = tr.DefaultTranscriptFile()
	}
	transcriptPath, err = filepath.Abs(transcriptPath)
	if err != nil {
		return fmt.Errorf("resolve transcript file: %w", err)
	}

	vault, err := resolveVault(f, vaultFlagSet)
	if err != nil {
		return err
	}

	return pipeline.Run(ctx, pipeline.Options{
		AudioFile:      audioFile,
		Transcriber:    tr,
		Summarizer:     sum,
		TranscriptFile: transcriptPath,
		OutputFile:     f.outputFile,
		ObsidianVault:  vault,
	})
}

// resolveVault picks the Obsidian vault path according to the rules:
//  1. --obsidian-vault wins outright (and implies --obsidian).
//  2. --obsidian alone reads $OBSIDIAN_VAULT_PATH; empty env is an error.
//  3. Neither flag set: empty string (no vault output).
func resolveVault(f flags, vaultFlagSet bool) (string, error) {
	if vaultFlagSet {
		if f.obsidianVault == "" {
			return "", fmt.Errorf("--obsidian-vault requires a non-empty path")
		}
		return f.obsidianVault, nil
	}
	if f.obsidian {
		v := os.Getenv(obsidianVaultEnv)
		if v == "" {
			return "", fmt.Errorf("--obsidian was set but $%s is empty", obsidianVaultEnv)
		}
		return v, nil
	}
	return "", nil
}

func Execute() {
	if err := newRootCmd().ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
