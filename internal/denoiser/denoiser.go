// Package denoiser defines the Denoiser interface and a global registry
// adapters register into via init(). A denoiser cleans up an audio file by
// removing background noise (without generative restoration) before
// transcription.
package denoiser

import (
	"audio2note/internal/registry"
	"context"
)

// Input is everything a denoiser needs to produce a cleaned-up audio file.
type Input struct {
	// AudioFile is the absolute path to the source audio.
	AudioFile string
	// OutputDir is a directory the denoiser may write its output (and any
	// intermediate files) into. The denoiser owns the layout beneath it and
	// returns the absolute path of the denoised file.
	OutputDir string
}

// Denoiser removes background noise from an audio file to improve downstream
// transcription and returns the absolute path to the denoised audio.
type Denoiser interface {
	Name() string
	// Validate checks the adapter's external dependencies (e.g. required CLI
	// binaries) are available, so callers can fast-fail before doing expensive
	// work like downloading or stitching audio.
	Validate() error
	Denoise(ctx context.Context, in Input) (string, error)
}

var reg = registry.New[Denoiser]()

func Register(d Denoiser)               { reg.Register(d) }
func Get(name string) (Denoiser, error) { return reg.Get(name) }
func Names() []string                   { return reg.Names() }
