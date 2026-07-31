// Package cmd wires the cobra root command, parses flags, and hands off to the
// pipeline.
package cmd

import (
	// Blank-import adapters so their init() registers them. Adding a new
	// adapter = new package + one line here.
	_ "audio2note/internal/adapters/claudecode"
	_ "audio2note/internal/adapters/macwhisper"
	_ "audio2note/internal/adapters/opencode"
	_ "audio2note/internal/adapters/resembleenhance"
	_ "audio2note/internal/adapters/whisperx"
	"audio2note/internal/audiostitch"
	"audio2note/internal/denoiser"
	"audio2note/internal/enhancer"
	"audio2note/internal/pipeline"
	"audio2note/internal/summarizer"
	"audio2note/internal/transcriber"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/shlex"
	"github.com/spf13/cobra"
)

const obsidianVaultEnv = "OBSIDIAN_VAULT_PATH"

// defaultDenoiser / defaultEnhancer are resolved from their registries when
// --denoise / --enhance-audio are set. There's a single adapter today (it
// satisfies both); future --denoiser / --enhancer selection flags would slot in
// alongside these the way --transcriber does.
const (
	defaultDenoiser = "resemble-enhance"
	defaultEnhancer = "resemble-enhance"
)

// defaultTranscriptPath is where the pipeline writes the transcript when
// --transcript-file isn't given. Stable across runs (each invocation
// overwrites the same file) and lives in the OS temp dir so it doesn't
// clutter cwd; the OS reaps it eventually, so there's no explicit cleanup.
var defaultTranscriptPath = filepath.Join(os.TempDir(), "audio2note-transcript.json")

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
	editSpeakerLabels     bool
	denoise               bool
	enhanceAudio          bool
	// noRename is set only by the `plaud run` subcommand; the root command
	// never reads it. Lives here so addPipelineFlags can share the struct.
	noRename bool
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audio2note",
		Short: "Transcribe one or more audio files and summarize them into meeting notes.",
	}
	cmd.AddCommand(newRunCmd(), newPlaudCmd())
	return cmd
}

// newRunCmd builds the `run` subcommand: the local audio-file flow. It mirrors
// `plaud run` so the two sources have the same `<source> run` shape.
func newRunCmd() *cobra.Command {
	var f flags
	cmd := &cobra.Command{
		Use:           "run <audio-file>...",
		Short:         "Transcribe one or more local audio files and summarize them into meeting notes. Multiple files are stitched in the order given.",
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), args, f, cmd.Flags().Changed("obsidian-vault"))
		},
	}
	addPipelineFlags(cmd, &f)
	return cmd
}

// addPipelineFlags registers the shared pipeline flags on cmd. Used by the
// root command (local audio file flow) and by `plaud run` (Plaud source flow).
func addPipelineFlags(cmd *cobra.Command, f *flags) {
	addRegistryFlag(cmd, &f.transcriber, "transcriber", "macwhisper", "Transcriber adapter to use", transcriber.Names)
	addRegistryFlag(cmd, &f.summarizer, "summarizer", "claudecode", "Summarizer adapter to use", summarizer.Names)
	cmd.Flags().StringVar(&f.transcriptFile, "transcript-file", "", "Path to the transcript file (defaults to "+defaultTranscriptPath+")")
	cmd.Flags().BoolVar(&f.obsidian, "obsidian", false, "Write notes to the Obsidian vault at $"+obsidianVaultEnv)
	cmd.Flags().StringVar(&f.obsidianVault, "obsidian-vault", "", "Explicit Obsidian vault path (implies --obsidian)")
	cmd.Flags().StringVar(&f.outputFile, "output-file", "", "If set, instructs the summarizer to use this exact file name; otherwise the summarizer chooses")
	cmd.Flags().BoolVar(&f.transcribeOnly, "transcribe-only", false, "Run only the transcriber (skip summarization)")
	cmd.Flags().BoolVar(&f.summarizeOnly, "summarize-only", false, "Run only the summarizer on an existing transcript file (skip transcription)")
	cmd.Flags().StringVar(&f.transcriberArgs, "transcriber-args", "", "Extra args to forward to the transcriber CLI (shell-quoted, e.g. \"--model whisper-large --stream\")")
	cmd.Flags().BoolVar(&f.transcriberNoDefaults, "transcriber-no-defaults", false, "Skip the transcriber adapter's hardcoded extra flags")
	cmd.Flags().StringVar(&f.summarizerArgs, "summarizer-args", "", "Extra args to forward to the summarizer CLI (shell-quoted, e.g. \"--model claude-sonnet-4-6\")")
	cmd.Flags().BoolVar(&f.summarizerNoDefaults, "summarizer-no-defaults", false, "Skip the summarizer adapter's hardcoded extra flags")
	cmd.Flags().StringVar(&f.prompt, "prompt", "", "Extra text appended to the summarizer's prompt (e.g. \"Use bullet points and keep it under 200 words.\")")
	cmd.Flags().BoolVar(&f.editSpeakerLabels, "edit-speaker-labels", false, "Open an interactive TUI between transcribe and summarize to rename speaker labels in the transcript")
	cmd.Flags().BoolVar(&f.denoise, "denoise", false, "Denoise speech (via the `"+defaultDenoiser+"` CLI, denoiser only) before transcription")
	cmd.Flags().BoolVar(&f.enhanceAudio, "enhance-audio", false, "Enhance speech (via the `"+defaultEnhancer+"` CLI, full denoise + restore — slow) before transcription")

	cmd.MarkFlagsMutuallyExclusive("transcribe-only", "summarize-only")
	// At most one of these: --denoise and --enhance-audio both preprocess audio
	// (full enhance already denoises), and neither makes sense with --summarize-only.
	cmd.MarkFlagsMutuallyExclusive("denoise", "enhance-audio", "summarize-only")
}

func run(ctx context.Context, audioArgs []string, f flags, vaultFlagSet bool) error {
	// Fast-fail before stitching if --denoise/--enhance-audio's tools are missing.
	if err := validateAudioProcessing(f); err != nil {
		return err
	}
	audioPath, err := resolveAudioFiles(ctx, audioArgs, f)
	if err != nil {
		return err
	}
	transcriptPath, err := resolveTranscriptPath(f)
	if err != nil {
		return err
	}
	opts, err := buildOptions(f, audioPath, transcriptPath, vaultFlagSet)
	if err != nil {
		return err
	}
	// The local-file flow never renames anything on Plaud, so we ignore the
	// returned Result (RequestTitle stays false here).
	_, err = pipeline.Run(ctx, opts)
	return err
}

// resolveAudioFiles reads the positional audio file arguments. Returns empty
// when the caller is in `--summarize-only` mode (no audio needed). For a
// single argument, returns its absolute path. For multiple arguments, stitches
// them in order via ffmpeg and returns the stitched file's path.
func resolveAudioFiles(ctx context.Context, audioArgs []string, f flags) (string, error) {
	if f.summarizeOnly {
		if len(audioArgs) > 0 {
			return "", fmt.Errorf("--summarize-only does not accept audio file arguments (got %d)", len(audioArgs))
		}
		return "", nil
	}
	if len(audioArgs) == 0 {
		return "", fmt.Errorf("audio file is required (unless --summarize-only is set)")
	}

	abs := make([]string, len(audioArgs))
	for i, a := range audioArgs {
		p, err := filepath.Abs(a)
		if err != nil {
			return "", fmt.Errorf("resolve audio file %q: %w", a, err)
		}
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("audio file %q: %w", a, err)
		}
		abs[i] = p
	}

	if len(abs) == 1 {
		return abs[0], nil
	}

	ext := strings.ToLower(filepath.Ext(abs[0]))
	stitched := filepath.Join(os.TempDir(), fmt.Sprintf("audio2note-stitched-%d%s", time.Now().UnixNano(), ext))
	fmt.Printf("→ stitching %d audio files → %s\n", len(abs), stitched)
	if err := audiostitch.Stitch(ctx, abs, stitched); err != nil {
		return "", err
	}
	return stitched, nil
}

// resolveTranscriptPath picks the transcript file path: --transcript-file if
// set, otherwise the shared defaultTranscriptPath. Always returned as an
// absolute path.
func resolveTranscriptPath(f flags) (string, error) {
	p := f.transcriptFile
	if p == "" {
		p = defaultTranscriptPath
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("resolve transcript file: %w", err)
	}
	return abs, nil
}

// buildOptions resolves the registry-looked-up transcriber / summarizer, the
// vault, the shlex-split args, and assembles a fully-populated
// pipeline.Options. The caller is responsible for setting source-specific
// fields like Highlights afterward.
func buildOptions(
	f flags, audioPath, transcriptPath string, vaultFlagSet bool,
) (pipeline.Options, error) {
	tr, err := transcriber.Get(f.transcriber)
	if err != nil {
		return pipeline.Options{}, fmt.Errorf("transcriber: %w", err)
	}
	sum, err := summarizer.Get(f.summarizer)
	if err != nil {
		return pipeline.Options{}, fmt.Errorf("summarizer: %w", err)
	}
	var den denoiser.Denoiser
	if f.denoise {
		den, err = denoiser.Get(defaultDenoiser)
		if err != nil {
			return pipeline.Options{}, fmt.Errorf("denoiser: %w", err)
		}
	}
	var enh enhancer.Enhancer
	if f.enhanceAudio {
		enh, err = enhancer.Get(defaultEnhancer)
		if err != nil {
			return pipeline.Options{}, fmt.Errorf("enhancer: %w", err)
		}
	}
	vault, err := resolveVault(f, vaultFlagSet)
	if err != nil {
		return pipeline.Options{}, err
	}
	transcriberArgs, err := splitArgs(f.transcriberArgs)
	if err != nil {
		return pipeline.Options{}, fmt.Errorf("--transcriber-args: %w", err)
	}
	summarizerArgs, err := splitArgs(f.summarizerArgs)
	if err != nil {
		return pipeline.Options{}, fmt.Errorf("--summarizer-args: %w", err)
	}
	return pipeline.Options{
		AudioFile:             audioPath,
		Denoiser:              den,
		Enhancer:              enh,
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
		EditSpeakerLabels:     f.editSpeakerLabels,
	}, nil
}

// validateAudioProcessing resolves the denoiser/enhancer and checks its
// external tools are available when --denoise/--enhance-audio is set, so callers
// can fast-fail before doing expensive work (downloading, stitching). The two
// flags are mutually exclusive, so at most one branch runs. A no-op when neither
// flag is set.
func validateAudioProcessing(f flags) error {
	if f.denoise {
		d, err := denoiser.Get(defaultDenoiser)
		if err != nil {
			return fmt.Errorf("denoiser: %w", err)
		}
		return d.Validate()
	}
	if f.enhanceAudio {
		e, err := enhancer.Get(defaultEnhancer)
		if err != nil {
			return fmt.Errorf("enhancer: %w", err)
		}
		return e.Validate()
	}
	return nil
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
