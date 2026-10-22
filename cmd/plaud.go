package cmd

import (
	"audio2note/internal/audiostitch"
	"audio2note/internal/config"
	"audio2note/internal/pipeline"
	"audio2note/internal/plaud"
	"audio2note/internal/plaudpicker"
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// defaultPlaudNameRegex matches Plaud's default `YYYY-MM-DD HH:MM:SS`
// filenames. When the file's name doesn't match, the user already renamed it
// and we leave it alone (see renamePlaudFiles).
var defaultPlaudNameRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`)

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
			// Fast-fail before any Plaud work if --denoise/--enhance-audio's tools are missing.
			if err := validateAudioProcessing(f); err != nil {
				return err
			}
			cfg, err := config.Effective()
			if err != nil {
				return err
			}
			if err := applyConfig(cmd, &f, cfg); err != nil {
				return err
			}
			client, err := plaud.NewClient()
			if err != nil {
				if errors.Is(err, plaud.ErrMissing) {
					return errors.New("missing bearer token, please run `audio2note plaud auth setup`")
				}
				return err
			}
			return runPlaud(cmd.Context(), client, f, cfg, cmd.Flags().Changed("obsidian-vault"))
		},
	}
	addPipelineFlags(cmd, &f)
	// --no-rename is Plaud-specific so it lives on this subcommand only
	// (not in addPipelineFlags, which is shared with the local-file root).
	cmd.Flags().BoolVar(&f.noRename, "no-rename", false, "Disable auto-renaming the Plaud recording to a summarizer-produced title after a successful run")
	return cmd
}

// runPlaud is the testable post-construction handler: lists, opens the
// picker, and (on a successful pick) hands off to runPlaudAfterPick. The
// picker itself isn't covered by tests (TUI); runPlaudAfterPick is.
func runPlaud(
	ctx context.Context, client plaud.Client, f flags, cfg config.Config, vaultFlagSet bool,
) error {
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
	if !ok || len(selected) == 0 {
		return errors.New("plaud selection cancelled")
	}
	opts, err := runPlaudAfterPick(ctx, client, selected, f, cfg, vaultFlagSet)
	if err != nil {
		return err
	}
	result, err := pipeline.Run(ctx, opts)
	if err != nil {
		return err
	}
	if !f.noRename && !f.transcribeOnly {
		renamePlaudFiles(ctx, client, selected, result.Title)
	}
	return nil
}

// renamePlaudFiles updates the filename on each selected Plaud recording to a
// summarizer-produced title. Best-effort: an empty title, a file that's
// already been manually renamed, or a per-file API error each surface as a
// warning line — they never abort the command. Multi-file selections get
// `(part N/M)` suffixes so the stitched recording is still discoverable as a
// group.
func renamePlaudFiles(
	ctx context.Context, client plaud.Client, selected []plaud.File, title string,
) {
	if title == "" {
		fmt.Println("→ summarizer did not produce a title, skipping Plaud rename")
		return
	}
	for i, f := range selected {
		if !defaultPlaudNameRegex.MatchString(f.Filename) {
			fmt.Printf("→ Plaud file %q already renamed, skipping\n", f.Filename)
			continue
		}
		newName := title
		if len(selected) > 1 {
			newName = fmt.Sprintf("%s (part %d/%d)", title, i+1, len(selected))
		}
		if err := client.RenameFile(ctx, f.ID, newName); err != nil {
			fmt.Printf("→ couldn't rename Plaud file %s: %v\n", f.Filename, err)
			continue
		}
		fmt.Printf("→ renamed Plaud file %s → %q\n", f.Filename, newName)
	}
}

// runPlaudAfterPick downloads what's needed based on the flag combination,
// fetches highlights for every selected recording (offsetting timestamps so
// they line up with the stitched timeline), and returns the fully populated
// pipeline.Options. The caller invokes pipeline.Run — separating the
// side-effecting Plaud work from the pipeline invocation makes this function
// testable in isolation.
func runPlaudAfterPick(
	ctx context.Context,
	client plaud.Client,
	selected []plaud.File,
	f flags,
	cfg config.Config,
	vaultFlagSet bool,
) (pipeline.Options, error) {
	if len(selected) == 0 {
		return pipeline.Options{}, errors.New("no recordings selected")
	}
	transcriptPath, err := resolveTranscriptPath(f)
	if err != nil {
		return pipeline.Options{}, err
	}

	var audioPath string
	if f.summarizeOnly {
		if err := resolveSummarizeOnlyTranscript(ctx, client, selected, transcriptPath); err != nil {
			return pipeline.Options{}, err
		}
	} else {
		path, err := downloadAndMaybeStitch(ctx, client, selected)
		if err != nil {
			return pipeline.Options{}, err
		}
		audioPath = path
	}

	highlights, err := fetchAllPlaudHighlights(ctx, client, selected)
	if err != nil {
		return pipeline.Options{}, err
	}

	opts, err := buildOptions(f, cfg, audioPath, transcriptPath, vaultFlagSet)
	if err != nil {
		return pipeline.Options{}, err
	}
	opts.Highlights = highlights
	// Only ask the summarizer for a title when we're actually going to
	// use it for rename; otherwise we'd spend tokens and add a flag-handshake
	// step for no reason.
	opts.RequestTitle = !f.noRename && !f.transcribeOnly
	return opts, nil
}

// resolveSummarizeOnlyTranscript prepares the transcript file for
// --summarize-only mode. With a single recording, falls back to downloading
// from Plaud when no local file exists (preserving the long-standing
// transcribe-only → manual MW export → summarize-only workflow described in
// the TODO below). With multiple recordings, requires the local transcript to
// already exist — we don't stitch JSON transcripts.
func resolveSummarizeOnlyTranscript(
	ctx context.Context, client plaud.Client, selected []plaud.File, transcriptPath string,
) error {
	if len(selected) > 1 {
		if _, err := os.Stat(transcriptPath); err != nil {
			return fmt.Errorf("--summarize-only with multiple recordings requires --transcript-file to point at an existing transcript (typically one you produced earlier with --transcribe-only on the same selection); %w", err)
		}
		fmt.Printf("→ using existing transcript file %s; fetching highlights from %d recordings\n", transcriptPath, len(selected))
		return nil
	}
	// TODO(plaud-mw-export): once the MacWhisper CLI supports
	// exporting transcripts with timestamps, this file-existence
	// branch can be deleted — the user can always download the
	// transcript fresh from Plaud. The branch exists today to
	// support the workflow: `plaud run --transcribe-only` →
	// manually export a timestamped JSON from the MacWhisper
	// desktop app → `plaud run --summarize-only --transcript-file
	// exported.json` (uses the local file, only fetches
	// highlights from Plaud).
	only := selected[0]
	switch _, statErr := os.Stat(transcriptPath); {
	case statErr == nil:
		fmt.Printf("→ using existing transcript file %s; fetching highlights only\n", transcriptPath)
	case errors.Is(statErr, os.ErrNotExist):
		fmt.Printf("→ downloading transcript for %s → %s\n", only.Filename, transcriptPath)
		if _, err := client.DownloadTranscript(ctx, only.ID, transcriptPath); err != nil {
			return fmt.Errorf("download transcript: %w", err)
		}
	default:
		return fmt.Errorf("transcript file: %w", statErr)
	}
	return nil
}

// downloadAndMaybeStitch downloads every selected recording and, if there's
// more than one, stitches them into a single mp3/ogg via ffmpeg. Returns the
// absolute path of the file the transcriber should consume.
func downloadAndMaybeStitch(
	ctx context.Context, client plaud.Client, selected []plaud.File,
) (string, error) {
	if len(selected) == 1 {
		only := selected[0]
		tmpPath := filepath.Join(os.TempDir(), "audio2note-plaud-"+string(only.ID)+only.Extension)
		fmt.Printf("→ downloading recording for %s → %s\n", only.Filename, tmpPath)
		path, err := client.DownloadRecording(ctx, only.ID, tmpPath)
		if err != nil {
			return "", fmt.Errorf("download recording: %w", err)
		}
		return path, nil
	}

	ext := selected[0].Extension
	for _, f := range selected[1:] {
		if f.Extension != ext {
			return "", fmt.Errorf("cannot stitch recordings with mismatched formats: %s (%s) and %s (%s)", selected[0].Filename, ext, f.Filename, f.Extension)
		}
	}

	paths := make([]string, len(selected))
	for i, f := range selected {
		tmpPath := filepath.Join(os.TempDir(), "audio2note-plaud-"+string(f.ID)+f.Extension)
		fmt.Printf("→ downloading recording %d/%d: %s → %s\n", i+1, len(selected), f.Filename, tmpPath)
		p, err := client.DownloadRecording(ctx, f.ID, tmpPath)
		if err != nil {
			return "", fmt.Errorf("download recording %s: %w", f.Filename, err)
		}
		paths[i] = p
	}

	stitched := filepath.Join(os.TempDir(), fmt.Sprintf("audio2note-plaud-stitched-%d%s", time.Now().UnixNano(), ext))
	fmt.Printf("→ stitching %d recordings → %s\n", len(paths), stitched)
	if err := audiostitch.Stitch(ctx, paths, stitched); err != nil {
		return "", err
	}
	return stitched, nil
}

// fetchAllPlaudHighlights pulls highlights for every selected recording in
// order and merges them into a single timeline, adding the cumulative
// duration of earlier recordings to each highlight so the offsets line up
// with the stitched audio.
func fetchAllPlaudHighlights(
	ctx context.Context, client plaud.Client, selected []plaud.File,
) ([]time.Duration, error) {
	perFile := make([][]time.Duration, len(selected))
	for i, f := range selected {
		hl, err := fetchPlaudHighlights(ctx, client, f)
		if err != nil {
			return nil, err
		}
		perFile[i] = hl
	}
	return offsetHighlights(selected, perFile), nil
}

// offsetHighlights merges per-file highlight slices into one timeline by
// adding the cumulative duration of earlier files. Pure function — easy to
// test independently of the Plaud client.
func offsetHighlights(selected []plaud.File, perFile [][]time.Duration) []time.Duration {
	if len(selected) == 0 {
		return nil
	}
	var total int
	for _, hl := range perFile {
		total += len(hl)
	}
	if total == 0 {
		return nil
	}
	out := make([]time.Duration, 0, total)
	var offset time.Duration
	for i, f := range selected {
		for _, h := range perFile[i] {
			out = append(out, offset+h)
		}
		offset += f.Duration
	}
	return out
}

// fetchPlaudHighlights gets the user-marked highlight timestamps for one
// recording. Failures are non-fatal: a warning is printed and the caller
// proceeds without highlights for that recording.
func fetchPlaudHighlights(
	ctx context.Context, client plaud.Client, f plaud.File,
) ([]time.Duration, error) {
	hl, err := client.GetHighlights(ctx, f.ID)
	if err != nil {
		fmt.Printf("→ couldn't fetch highlights for %s: %v (proceeding without)\n", f.Filename, err)
		return nil, nil
	}
	if len(hl) == 0 {
		return nil, nil
	}
	fmt.Printf("→ got %d highlight(s) for %s\n", len(hl), f.Filename)
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
