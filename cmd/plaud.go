package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"audio2note/internal/pipeline"
	"audio2note/internal/plaud"
	"audio2note/internal/plaudpicker"
)

const plaudSetupInstructions = `To get your Plaud bearer token:
  1. Open https://web.plaud.ai in your browser and sign in.
  2. Open the browser dev tools and inspect localStorage for the site.
  3. Copy the value of the key "pid_tokenstr".

`

func newPlaudCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plaud",
		Short: "Plaud-related commands",
	}
	cmd.AddCommand(newPlaudAuthCmd(), newPlaudRunCmd())
	return cmd
}

func newPlaudRunCmd() *cobra.Command {
	var f flags
	cmd := &cobra.Command{
		Use:           "run",
		Short:         "Pick a Plaud recording and run the transcribe + summarize pipeline",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := plaud.NewClient()
			if err != nil {
				if errors.Is(err, plaud.ErrMissing) {
					return errors.New("missing bearer token, please run `audio2note plaud auth setup`")
				}
				return err
			}
			return runPlaud(cmd.Context(), client, f, cmd.Flags().Changed("obsidian-vault"))
		},
	}
	addPipelineFlags(cmd, &f)
	return cmd
}

// runPlaud is the testable post-construction handler: lists, opens the
// picker, and (on a successful pick) hands off to runPlaudAfterPick. The
// picker itself isn't covered by tests (TUI); runPlaudAfterPick is.
func runPlaud(ctx context.Context, client plaud.Client, f flags, vaultFlagSet bool) error {
	fmt.Println("→ fetching Plaud library…")
	files, err := client.ListFiles(ctx)
	if err != nil {
		return fmt.Errorf("plaud list files: %w", err)
	}
	if len(files) == 0 {
		return errors.New("plaud library is empty")
	}
	selected, ok, err := plaudpicker.Pick(files)
	if err != nil {
		return fmt.Errorf("plaud picker: %w", err)
	}
	if !ok {
		return errors.New("plaud selection cancelled")
	}
	opts, err := runPlaudAfterPick(ctx, client, selected, f, vaultFlagSet)
	if err != nil {
		return err
	}
	return pipeline.Run(ctx, opts)
}

// runPlaudAfterPick downloads what's needed based on the flag combination,
// fetches highlights, and returns the fully populated pipeline.Options. The
// caller invokes pipeline.Run — separating the side-effecting Plaud work
// from the pipeline invocation makes this function testable in isolation.
func runPlaudAfterPick(ctx context.Context, client plaud.Client, selected plaud.File, f flags, vaultFlagSet bool) (pipeline.Options, error) {
	transcriptPath, err := resolveTranscriptPath(f)
	if err != nil {
		return pipeline.Options{}, err
	}

	var audioPath string
	if f.summarizeOnly {
		// TODO(plaud-mw-export): once the MacWhisper CLI supports
		// exporting transcripts with timestamps, this file-existence
		// branch can be deleted — the user can always download the
		// transcript fresh from Plaud. The branch exists today to
		// support the workflow: `plaud run --transcribe-only` →
		// manually export a timestamped JSON from the MacWhisper
		// desktop app → `plaud run --summarize-only --transcript-file
		// exported.json` (uses the local file, only fetches
		// highlights from Plaud).
		switch _, statErr := os.Stat(transcriptPath); {
		case statErr == nil:
			fmt.Printf("→ using existing transcript file %s; fetching highlights only\n", transcriptPath)
		case errors.Is(statErr, os.ErrNotExist):
			fmt.Printf("→ downloading transcript for %s → %s\n", selected.Filename, transcriptPath)
			if _, err := client.DownloadTranscript(ctx, selected.ID, transcriptPath); err != nil {
				return pipeline.Options{}, fmt.Errorf("download transcript: %w", err)
			}
		default:
			return pipeline.Options{}, fmt.Errorf("transcript file: %w", statErr)
		}
	} else {
		tmpPath := filepath.Join(os.TempDir(), "audio2note-plaud-"+string(selected.ID)+selected.Extension)
		fmt.Printf("→ downloading recording for %s → %s\n", selected.Filename, tmpPath)
		path, err := client.DownloadRecording(ctx, selected.ID, tmpPath)
		if err != nil {
			return pipeline.Options{}, fmt.Errorf("download recording: %w", err)
		}
		audioPath = path
	}

	highlights, err := fetchPlaudHighlights(ctx, client, selected.ID)
	if err != nil {
		return pipeline.Options{}, err
	}

	opts, err := buildOptions(f, audioPath, transcriptPath, vaultFlagSet)
	if err != nil {
		return pipeline.Options{}, err
	}
	opts.Highlights = highlights
	return opts, nil
}

// fetchPlaudHighlights gets the user-marked highlight timestamps for the
// recording. Failures are non-fatal: a warning is printed and the pipeline
// proceeds without highlights.
func fetchPlaudHighlights(ctx context.Context, client plaud.Client, id plaud.ID) ([]time.Duration, error) {
	hl, err := client.GetHighlights(ctx, id)
	if err != nil {
		fmt.Printf("→ couldn't fetch highlights: %v (proceeding without)\n", err)
		return nil, nil
	}
	if len(hl) == 0 {
		return nil, nil
	}
	fmt.Printf("→ got %d highlight(s)\n", len(hl))
	out := make([]time.Duration, len(hl))
	for i, h := range hl {
		out[i] = time.Duration(h)
	}
	return out, nil
}

func newPlaudAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage Plaud authentication",
	}
	cmd.AddCommand(newPlaudAuthSetupCmd(), newPlaudAuthValidateCmd())
	return cmd
}

func newPlaudAuthSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "setup",
		Short:         "Prompt for a Plaud bearer token and save it to ~/.plaud/token",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Print(plaudSetupInstructions)
			fmt.Print("Bearer token: ")
			token, err := readLine(os.Stdin)
			if err != nil {
				return fmt.Errorf("read token: %w", err)
			}
			if err := plaud.SaveToken(token); err != nil {
				return err
			}
			path, _ := plaud.TokenPath()
			fmt.Printf("✓ saved token to %s\n", path)
			return nil
		},
	}
}

func newPlaudAuthValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "validate",
		Short:         "Check that the saved Plaud bearer token is still valid",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := plaud.NewClient()
			if err != nil {
				if errors.Is(err, plaud.ErrMissing) {
					return errors.New("missing bearer token, please run `audio2note plaud auth setup`")
				}
				return err
			}
			if err := c.Validate(cmd.Context()); err != nil {
				return errors.New("bearer token expired, please run `audio2note plaud auth setup`")
			}
			fmt.Println("token verified!")
			return nil
		},
	}
}

func readLine(r io.Reader) (string, error) {
	sc := bufio.NewScanner(r)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return "", err
		}
		return "", errors.New("no input")
	}
	return strings.TrimSpace(sc.Text()), nil
}
