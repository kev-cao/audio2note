package resembleenhance

import (
	"audio2note/internal/denoiser"
	"audio2note/internal/enhancer"
	"context"
	"os/exec"
	"testing"
)

// TestRegistered verifies the adapter registers itself under "resemble-enhance"
// in both the denoiser and enhancer registries.
func TestRegistered(t *testing.T) {
	d, err := denoiser.Get("resemble-enhance")
	if err != nil {
		t.Fatalf("denoiser.Get(resemble-enhance): %v", err)
	}
	if d.Name() != "resemble-enhance" {
		t.Errorf("denoiser Name() = %q, want %q", d.Name(), "resemble-enhance")
	}
	e, err := enhancer.Get("resemble-enhance")
	if err != nil {
		t.Fatalf("enhancer.Get(resemble-enhance): %v", err)
	}
	if e.Name() != "resemble-enhance" {
		t.Errorf("enhancer Name() = %q, want %q", e.Name(), "resemble-enhance")
	}
}

// TestMissingBinary checks the friendly install error when resemble-enhance
// isn't on PATH, via both entry points. Skipped if it happens to be installed.
func TestMissingBinary(t *testing.T) {
	if _, err := exec.LookPath(binary); err == nil {
		t.Skip("resemble-enhance is installed; can't test the missing-binary path")
	}
	if _, err := (ResembleEnhance{}).Denoise(context.Background(), denoiser.Input{
		AudioFile: "/tmp/whatever.wav", OutputDir: t.TempDir(),
	}); err == nil {
		t.Errorf("Denoise: want error when binary missing; got nil")
	}
	if _, err := (ResembleEnhance{}).Enhance(context.Background(), enhancer.Input{
		AudioFile: "/tmp/whatever.wav", OutputDir: t.TempDir(),
	}); err == nil {
		t.Errorf("Enhance: want error when binary missing; got nil")
	}
}

// TestDetectDevice checks device detection returns one of the supported
// devices (CUDA preferred, then MPS on Apple Silicon, then CPU).
func TestDetectDevice(t *testing.T) {
	switch got := detectDevice(context.Background()); got {
	case "cuda", "mps", "cpu":
		// ok
	default:
		t.Errorf("detectDevice() = %q, want cuda, mps, or cpu", got)
	}
}
