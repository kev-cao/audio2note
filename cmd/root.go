// Package cmd wires the cobra root command, parses flags, and hands off to the
// pipeline.
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/shlex"
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
	transcriber           string
	summarizer            string
	transcriptFile        string
	obsidian              bool
	obsidianVault         string
	outputFile            string
	transcribeOnly        bool
	summarizeOnly         bool
	transcriberArgs       string
	transcriberNoDefaults bool
	summarizerArgs        string
	summarizerNoDefaults  bool
	prompt                string
}

func newRootCmd() *cobra.Command {
	var f flags
	cmd := &cobra.Command{
		Use:           "audio2note <audio-file>",
		Short:         "Transcribe an audio file and summarize it into meeting notes.",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			audioArg := ""
			if len(args) > 0 {
				audioArg = args[0]
			}
			return run(cmd.Context(), audioArg, f, cmd.Flags().Changed("obsidian-vault"))
		},
	}

	cmd.Flags().StringVar(&f.transcriber, "transcriber", "macwhisper", "Transcriber adapter to use")
	cmd.Flags().StringVar(&f.summarizer, "summarizer", "claudecode", "Summarizer adapter to use")
	cmd.Flags().StringVar(&f.transcriptFile, "transcript-file", "", "Path to the transcript file (defaults to the transcriber's default filename in cwd)")
	cmd.Flags().BoolVar(&f.obsidian, "obsidian", false, "Write notes to the Obsidian vault at $"+obsidianVaultEnv)
	cmd.Flags().StringVar(&f.obsidianVault, "obsidian-vault", "", "Explicit Obsidian vault path (implies --obsidian)")
	cmd.Flags().StringVar(&f.outputFile, "output-file", "transcript-notes.md", "File name for the summarizer's notes output")
	cmd.Flags().BoolVar(&f.transcribeOnly, "transcribe-only", false, "Run only the transcriber (skip summarization)")
	cmd.Flags().BoolVar(&f.summarizeOnly, "summarize-only", false, "Run only the summarizer on an existing transcript file (skip transcription)")
	cmd.Flags().StringVar(&f.transcriberArgs, "transcriber-args", "", "Extra args to forward to the transcriber CLI (shell-quoted, e.g. \"--model whisper-large --stream\")")
	cmd.Flags().BoolVar(&f.transcriberNoDefaults, "transcriber-no-defaults", false, "Skip the transcriber adapter's hardcoded extra flags")
	cmd.Flags().StringVar(&f.summarizerArgs, "summarizer-args", "", "Extra args to forward to the summarizer CLI (shell-quoted, e.g. \"--model claude-sonnet-4-6\")")
	cmd.Flags().BoolVar(&f.summarizerNoDefaults, "summarizer-no-defaults", false, "Skip the summarizer adapter's hardcoded extra flags")
	cmd.Flags().StringVar(&f.prompt, "prompt", "", "Extra text appended to the summarizer's prompt (e.g. \"Use bullet points and keep it under 200 words.\")")

	cmd.MarkFlagsMutuallyExclusive("transcribe-only", "summarize-only")

	return cmd
}

func run(ctx context.Context, audioArg string, f flags, vaultFlagSet bool) error {
	tr, err := transcriber.Get(f.transcriber)
	if err != nil {
		return fmt.Errorf("transcriber: %w", err)
	}
	sum, err := summarizer.Get(f.summarizer)
	if err != nil {
		return fmt.Errorf("summarizer: %w", err)
	}

	// Audio file is only required when we're going to transcribe.
	var audioFile string
	if !f.summarizeOnly {
		if audioArg == "" {
			return fmt.Errorf("audio file is required (unless --summarize-only is set)")
		}
		audioFile, err = filepath.Abs(audioArg)
		if err != nil {
			return fmt.Errorf("resolve audio file: %w", err)
		}
		if _, err := os.Stat(audioFile); err != nil {
			return fmt.Errorf("audio file: %w", err)
		}
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

	transcriberArgs, err := splitArgs(f.transcriberArgs)
	if err != nil {
		return fmt.Errorf("--transcriber-args: %w", err)
	}
	summarizerArgs, err := splitArgs(f.summarizerArgs)
	if err != nil {
		return fmt.Errorf("--summarizer-args: %w", err)
	}

	return pipeline.Run(ctx, pipeline.Options{
		AudioFile:             audioFile,
		Transcriber:           tr,
		Summarizer:            sum,
		TranscriptFile:        transcriptPath,
		OutputFile:            f.outputFile,
		ObsidianVault:         vault,
		SkipTranscribe:        f.summarizeOnly,
		SkipSummarize:         f.transcribeOnly,
		TranscriberArgs:       transcriberArgs,
		TranscriberNoDefaults: f.transcriberNoDefaults,
		SummarizerArgs:        summarizerArgs,
		SummarizerNoDefaults:  f.summarizerNoDefaults,
		ExtraPrompt:           f.prompt,
	})
}

// splitArgs runs a shell-style split over s. Empty input returns nil.
func splitArgs(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	return shlex.Split(s)
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
