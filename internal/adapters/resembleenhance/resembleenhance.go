// Package resembleenhance adapts the resemble-enhance CLI to both the Denoiser
// and Enhancer interfaces. resemble-enhance is a speech denoiser + restorer
// purpose-built for cleaning up recorded speech (podcasts, interviews,
// meetings), which makes it a good preprocessing step before transcription.
//
//   - Denoise runs it in --denoise_only mode: just the denoiser. Fast(er) and
//     non-generative, so it won't alter speech — the safe default for ASR.
//   - Enhance runs the full pipeline (denoise + a generative flow-matching
//     restorer). Higher quality but much slower on CPU, and generative, so it
//     can subtly change speech.
package resembleenhance

import (
	"audio2note/internal/denoiser"
	"audio2note/internal/enhancer"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// binary is the resemble-enhance CLI we shell out to.
const binary = "resemble-enhance"

// installURL is surfaced when the binary isn't on PATH.
const installURL = "https://github.com/resemble-ai/resemble-enhance"

// subdir groups this adapter's working files under the output dir. The CLI
// works on folders, so we stage the audio in in/ and read results from out/.
const subdir = "audio2note-enhance"

// stagedName is the fixed WAV filename we stage the input under; the CLI mirrors
// the name into the output folder, so the result lands at out/<stagedName>.
const stagedName = "audio.wav"

type ResembleEnhance struct{}

func (ResembleEnhance) Name() string { return "resemble-enhance" }

// Validate checks the tools this adapter needs are on PATH: resemble-enhance,
// ffmpeg (used to stage the input as a WAV), and git + git-lfs (resemble-enhance
// clones its model from a git-lfs repo on first use). We require git-lfs
// unconditionally rather than only when the model is uncached: it's a cheap
// check, and failing here up front beats a cryptic clone error mid-run.
func (ResembleEnhance) Validate() error {
	if _, err := exec.LookPath(binary); err != nil {
		return fmt.Errorf("%s not found on PATH; install resemble-enhance (see %s)", binary, installURL)
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found on PATH")
	}
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found on PATH (required for the resemble-enhance model download)")
	}
	if _, err := exec.LookPath("git-lfs"); err != nil {
		return fmt.Errorf("git-lfs not found on PATH (required for the resemble-enhance model download)")
	}
	// git-lfs can be installed but never initialized (`git lfs install` sets up
	// the smudge filter in global git config, a one-time step). Without it the
	// model repo clones as pointer files and inference fails with a missing
	// .pt. Verify the filter is configured.
	if out, _ := exec.Command("git", "config", "--get", "filter.lfs.smudge").Output(); strings.TrimSpace(string(out)) == "" {
		return fmt.Errorf("git-lfs is installed but not initialized; run `git lfs install` once")
	}
	return nil
}

// Denoise cleans up in.AudioFile with resemble-enhance's denoiser only (no
// generative restoration) and returns the path to the denoised audio.
func (r ResembleEnhance) Denoise(ctx context.Context, in denoiser.Input) (string, error) {
	return r.run(ctx, in.AudioFile, in.OutputDir, true /* denoiseOnly */)
}

// Enhance cleans up in.AudioFile with resemble-enhance's full pipeline (denoise
// + generative restoration) and returns the path to the enhanced audio.
func (r ResembleEnhance) Enhance(ctx context.Context, in enhancer.Input) (string, error) {
	return r.run(ctx, in.AudioFile, in.OutputDir, false /* denoiseOnly */)
}

// run stages the source as a WAV, runs resemble-enhance over it, and returns
// the resulting file. The CLI processes a folder of .wav files (in_dir) and
// writes each result to out_dir under the same name, so we stage a single WAV
// in a fresh input folder and read the mirrored output back.
func (r ResembleEnhance) run(ctx context.Context, audioFile, outputDir string, denoiseOnly bool) (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}

	base := filepath.Join(outputDir, subdir)
	inDir := filepath.Join(base, "in")
	outDir := filepath.Join(base, "out")
	// Start clean so a stale input from a previous run isn't reprocessed (the
	// CLI globs every .wav under in_dir).
	if err := os.RemoveAll(base); err != nil {
		return "", fmt.Errorf("resemble-enhance: clear work dir: %w", err)
	}
	if err := os.MkdirAll(inDir, 0o755); err != nil {
		return "", fmt.Errorf("resemble-enhance: create input dir: %w", err)
	}

	// Stage the source as a WAV the CLI will pick up (it globs *.wav, and this
	// also normalizes any input container/codec through ffmpeg).
	staged := filepath.Join(inDir, stagedName)
	if err := toWAV(ctx, audioFile, staged); err != nil {
		return "", err
	}

	device := detectDevice(ctx)
	// Full enhance can't use MPS: its weight-norm op isn't implemented there,
	// and the CPU fallback re-applies inside the enhancer's per-step
	// parametrization, thrashing host<->device copies badly enough to exhaust
	// memory. Fall back to CPU for enhance. Denoise has no such op and runs fine
	// (and ~2x+ faster than CPU) on MPS.
	if !denoiseOnly && device == "mps" {
		fmt.Println("→ full enhance isn't supported on MPS; falling back to CPU (slow) — consider --denoise")
		device = "cpu"
	}

	mode := "enhance"
	args := []string{"--device", device, inDir, outDir}
	if denoiseOnly {
		mode = "denoise"
		// --denoise_only skips the generative enhancer stage (a flow-matching
		// model iterating --nfe times), which is far too slow on CPU and can
		// subtly alter speech.
		args = append([]string{"--denoise_only"}, args...)
	}
	fmt.Printf("→ resemble-enhance (%s) using device %q\n", mode, device)

	cmd := exec.CommandContext(ctx, binary, args...)
	// resemble-enhance prints a progress bar (and Torch warnings) to stderr;
	// pass them through so a long run shows progress. No useful stdout.
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("resemble-enhance: %w", err)
	}

	out := filepath.Join(outDir, stagedName)
	if _, err := os.Stat(out); err != nil {
		return "", fmt.Errorf("resemble-enhance: expected output %s not found: %w", out, err)
	}
	return out, nil
}

// detectDevice picks the compute device for resemble-enhance, preferring CUDA,
// then Apple's MPS, then CPU. The CLI defaults to "cuda" and doesn't fall back
// on its own, so we ask Torch (already a resemble-enhance dependency) what's
// available. CUDA is always chosen when present. On Apple Silicon MPS is used
// for denoise (~2x+ faster than CPU); full enhance can't use MPS and run()
// downgrades it to CPU.
func detectDevice(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, pythonInterpreter(), "-c",
		"import torch; print('cuda' if torch.cuda.is_available() else ('mps' if torch.backends.mps.is_available() else 'cpu'))").Output()
	if err == nil {
		switch d := strings.TrimSpace(string(out)); d {
		case "cuda", "mps", "cpu":
			return d
		}
	}
	return "cpu"
}

// pythonInterpreter returns the Python that resemble-enhance itself runs under
// — its sibling in the same bin dir — so the Torch import in detectDevice sees
// the same environment (the `python3` on PATH may be a different interpreter
// without Torch installed). Falls back to "python3" if it can't be located.
func pythonInterpreter() string {
	p, err := exec.LookPath(binary)
	if err != nil {
		return "python3"
	}
	sibling := filepath.Join(filepath.Dir(p), "python3")
	if _, err := os.Stat(sibling); err == nil {
		return sibling
	}
	return "python3"
}

// toWAV transcodes src to a WAV at dst via ffmpeg.
func toWAV(ctx context.Context, src, dst string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-i", src,
		dst,
	)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("resemble-enhance: ffmpeg transcode %s to WAV: %w", filepath.Base(src), err)
	}
	return nil
}

func init() {
	denoiser.Register(ResembleEnhance{})
	enhancer.Register(ResembleEnhance{})
}
